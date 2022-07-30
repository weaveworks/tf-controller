package mtls

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	certName   = "tls.crt"
	keyName    = "tls.key"
	caCertName = "ca.crt"
	caKeyName  = "ca.key"
)

var crLog = logf.Log.WithName("cert-rotation")
var _ manager.Runnable = &CertRotator{}

/*
// SyncingReader is a reader that needs syncing prior to being usable.
type SyncingReader interface {
	client.Reader
	WaitForCacheSync(ctx context.Context) bool
}*/

// PartialManager is a subset of the manager.Manager interface that is used by the CertRotator.
type PartialManager interface {
	GetConfig() *rest.Config
	GetScheme() *runtime.Scheme
	GetRESTMapper() meta.RESTMapper
	Elected() <-chan struct{}
}

// KeyPairArtifacts stores cert artifacts.
type KeyPairArtifacts struct {
	Cert       *x509.Certificate
	Key        *rsa.PrivateKey
	CertPEM    []byte
	KeyPEM     []byte
	validUntil time.Time
}

type artifact struct {
	ca         *KeyPairArtifacts
	certSecret *corev1.Secret
}

type TriggerResult struct {
	Secret *corev1.Secret
	Err    error
}

type Trigger struct {
	Namespace string
	Ready     chan *TriggerResult
}

// CertRotator contains cert artifacts and a channel to close when the certs are ready.
type CertRotator struct {
	writer client.Client
	mgr    manager.Manager

	extKeyUsages *[]x509.ExtKeyUsage
	Ready        chan struct{}

	CAName             string
	CAOrganization     string
	DNSName            string
	CAValidityDuration time.Duration
	// CertValidityDuration   time.Duration
	RotationCheckFrequency time.Duration
	LookaheadInterval      time.Duration

	TriggerCARotation             chan Trigger // trigger the CA rotation
	TriggerNamespaceTLSGeneration chan Trigger // trigger namespace TLS generation

	artifactCaches         []*artifact
	knownNamespaceTLSMap   map[string]*TriggerResult
	knownNamespaceTLSMapMu sync.Mutex
}

func (cr *CertRotator) GetTLSGenerationResult(namespace string) (*corev1.Secret, error) {
	cr.knownNamespaceTLSMapMu.Lock()
	defer cr.knownNamespaceTLSMapMu.Unlock()

	result := cr.knownNamespaceTLSMap[namespace]
	if result == nil {
		return nil, errors.New("no TLS generation result")
	}

	return result.Secret, result.Err
}

// AddRotator adds the CertRotator and ReconcileWH to the manager.
func AddRotator(ctx context.Context, mgr manager.Manager, cr *CertRotator) error {
	if mgr == nil || cr == nil {
		return fmt.Errorf("nil arguments")
	}

	cr.writer = mgr.GetClient() // TODO make overrideable
	cr.mgr = mgr
	cr.knownNamespaceTLSMap = make(map[string]*TriggerResult)

	if err := mgr.Add(cr); err != nil {
		return err
	}

	return nil
}

// IsCAValid checks that the CA[n-1] is valid.
func (cr *CertRotator) IsCAValid() (bool, error) {
	n := len(cr.artifactCaches)
	if n == 0 {
		return false, errors.New("no CA in the cache")
	}

	validUntil := cr.artifactCaches[n-1].ca.validUntil
	if validUntil.Before(cr.lookaheadTime()) {
		return true, nil
	}

	return false, nil
}

// Start starts the CertRotator runnable to rotate certs and ensure the certs are ready.
func (cr *CertRotator) Start(ctx context.Context) error {
	// Only the leader do cert rotation.
	// if we're not, we're blocked here and don't do any cert rotation until we becomes the leader.
	<-cr.mgr.Elected()

	cr.extKeyUsages = &[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}

	// explicitly rotate on the first round so that the certificate
	// can be bootstrapped, otherwise mgr exits before a cert can be written
	crLog.Info("starting cert rotator controller")
	defer crLog.Info("stopping cert rotator controller")
	if err := cr.refreshCertsInMemory(); err != nil {
		crLog.Error(err, "could not refresh cert on startup")
		return err
	}

	close(cr.Ready)

	// TODO implement GC for getting rid of old certs
	ticker := time.NewTicker(cr.RotationCheckFrequency)

tickerLoop:
	for {
		select {
		case trigger := <-cr.TriggerCARotation:
			if err := cr.refreshCACertsIfNeeded(); err != nil {
				crLog.Error(err, "could not refresh cert")
			}
			n := len(cr.artifactCaches)
			secret := cr.artifactCaches[n-1].certSecret
			trigger.Ready <- &TriggerResult{Secret: secret, Err: nil}

		case <-ticker.C:
			if err := cr.refreshCACertsIfNeeded(); err != nil {
				crLog.Error(err, "could not refresh cert")
			}

			// GC: garbage collect the old CA artifacts
			for {

				if len(cr.artifactCaches) == 0 {
					break
				}

				validUntil := cr.artifactCaches[0].ca.validUntil
				// we must NOT use cr.lookaheadTime() here
				if validUntil.Before(time.Now()) {
					cr.artifactCaches = cr.artifactCaches[1:]
					for namespace := range cr.knownNamespaceTLSMap {
						err := cr.writer.Delete(context.TODO(), &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: namespace,
								Name:      fmt.Sprintf("%s-%d", infrav1.RunnerTLSSecretName, validUntil.Unix()),
							},
						}, client.PropagationPolicy(metav1.DeletePropagationBackground))
						if err != nil {
							crLog.Error(err, "could not delete old CA artifact")
						}
					}
				} else {
					break
				}
			}

		case trigger := <-cr.TriggerNamespaceTLSGeneration:
			namespace := trigger.Namespace
			if _, ok := cr.knownNamespaceTLSMap[namespace]; !ok {
				secret, err := cr.generateNamespaceTLS(namespace)
				if err != nil {
					crLog.Error(err, "error generating TLS for namespace %s", namespace)
				}
				cr.knownNamespaceTLSMap[namespace] = &TriggerResult{Secret: secret, Err: err}
			} else {
				crLog.Info("TLS for namespace %s already generated", namespace)
			}
			trigger.Ready <- cr.knownNamespaceTLSMap[namespace]

		case <-ctx.Done():
			break tickerLoop
		}
	}

	ticker.Stop()
	//close(cr.TriggerCARotation)
	//close(cr.TriggerNamespaceTLSGeneration)
	return nil
}

func (cr *CertRotator) refreshCACertsIfNeeded() error {
	needRegeneration := false
	// if there is no CA artifact, refresh certs
	n := len(cr.artifactCaches)
	if n == 0 {
		crLog.Info("no CA in the cache")
		if err := cr.refreshCertsInMemory(); err != nil {
			return err
		} else {
			needRegeneration = true
		}
	} else if n > 0 {
		validUntil := cr.artifactCaches[n-1].ca.validUntil
		if validUntil.Before(cr.lookaheadTime()) {
			if err := cr.refreshCertsInMemory(); err != nil {
				return err
			} else {
				needRegeneration = true
			}
		}
	}

	if needRegeneration {
		// generate new certs for all namespaces
		for namespace := range cr.knownNamespaceTLSMap {
			secret, err := cr.generateNamespaceTLS(namespace)
			if err != nil {
				crLog.Error(err, "could not generate TLS for namespace")
			}
			cr.knownNamespaceTLSMap[namespace] = &TriggerResult{Secret: secret, Err: err}
		}
	}

	return nil
}

// GetRunnerTLSSecretName returns the name of the TLS Secret.
// It is used by the controller to tell the runner the name of TLS.
func (cr *CertRotator) GetRunnerTLSSecretName() (string, error) {
	n := len(cr.artifactCaches)
	if n == 0 {
		return "", errors.New("no CA in the cache")
	}

	caArtifacts := cr.artifactCaches[n-1].ca
	return fmt.Sprintf("%s-%d", infrav1.RunnerTLSSecretName, caArtifacts.validUntil.Unix()), nil
}

func (cr *CertRotator) refreshCertsInMemory() error {
	var caArtifacts *KeyPairArtifacts
	now := time.Now()
	begin := now.Add(-1 * time.Hour)
	end := now.Add(cr.CAValidityDuration)

	var err error
	caArtifacts, err = cr.createCACert(begin, end)
	if err != nil {
		return err
	}

	// create controller-side certificate
	cert, key, err := cr.createCertPEM(caArtifacts, cr.DNSName, begin, end)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		Data: map[string][]byte{
			caCertName: caArtifacts.CertPEM,
			caKeyName:  caArtifacts.KeyPEM,
			certName:   cert,
			keyName:    key,
		},
	}

	cr.artifactCaches = append(cr.artifactCaches, &artifact{
		ca:         caArtifacts,
		certSecret: secret,
	})

	return nil
}

func buildArtifactsFromSecret(secret *corev1.Secret) (caArtifacts *KeyPairArtifacts, certArtifacts *KeyPairArtifacts, err error) {
	caArtifacts, err = parseArtifacts(caCertName, caKeyName, secret)
	if err != nil {
		return nil, nil, err
	}

	certArtifacts, err = parseArtifacts(certName, keyName, secret)
	if err != nil {
		return nil, nil, err
	}

	return caArtifacts, certArtifacts, nil
}

func parseArtifacts(certName, keyName string, secret *corev1.Secret) (*KeyPairArtifacts, error) {
	certPem, ok := secret.Data[certName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Cert Secret is not well-formed, missing %s", caCertName))
	}

	keyPem, ok := secret.Data[keyName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Cert Secret is not well-formed, missing %s", caKeyName))
	}

	certDer, _ := pem.Decode(certPem)
	if certDer == nil {
		return nil, errors.New("bad cert")
	}

	cert, err := x509.ParseCertificate(certDer.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "while parsing CA cert")
	}

	keyDer, _ := pem.Decode(keyPem)
	if keyDer == nil {
		return nil, errors.New("bad cert")
	}

	key, err := x509.ParsePKCS1PrivateKey(keyDer.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "while parsing key")
	}

	return &KeyPairArtifacts{
		Cert:    cert,
		CertPEM: certPem,
		KeyPEM:  keyPem,
		Key:     key,
	}, nil

}

// createCACert creates the self-signed CA cert and private key that will
// be used to sign the server certificate
func (cr *CertRotator) createCACert(begin, end time.Time) (*KeyPairArtifacts, error) {
	certTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(0),
		Subject: pkix.Name{
			CommonName:   cr.CAName,
			Organization: []string{cr.CAOrganization},
		},
		DNSNames: []string{
			cr.CAName,
		},
		NotBefore:             begin,
		NotAfter:              end,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, errors.Wrap(err, "generating key")
	}
	der, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, key.Public(), key)
	if err != nil {
		return nil, errors.Wrap(err, "creating certificate")
	}
	certPEM, keyPEM, err := pemEncode(der, key)
	if err != nil {
		return nil, errors.Wrap(err, "encoding PEM")
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, errors.Wrap(err, "parsing certificate")
	}

	return &KeyPairArtifacts{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM, validUntil: end}, nil
}

// createCertPEM takes the results of createCACert and uses it to create the
// PEM-encoded public certificate and private key, respectively.
func (cr *CertRotator) createCertPEM(ca *KeyPairArtifacts, hostname string, begin, end time.Time) ([]byte, []byte, error) {
	dnsNames := []string{hostname}
	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		dnsNames = append(dnsNames, "localhost")
	}

	certTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: cr.DNSName,
		},
		DNSNames:              dnsNames,
		NotBefore:             begin,
		NotAfter:              end,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, errors.Wrap(err, "generating key")
	}
	der, err := x509.CreateCertificate(rand.Reader, certTemplate, ca.Cert, key.Public(), ca.Key)
	if err != nil {
		return nil, nil, errors.Wrap(err, "creating certificate")
	}
	certPEM, keyPEM, err := pemEncode(der, key)
	if err != nil {
		return nil, nil, errors.Wrap(err, "encoding PEM")
	}
	return certPEM, keyPEM, nil
}

// pemEncode takes a certificate and encodes it as PEM
func pemEncode(certificateDER []byte, key *rsa.PrivateKey) ([]byte, []byte, error) {
	certBuf := &bytes.Buffer{}
	if err := pem.Encode(certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}); err != nil {
		return nil, nil, errors.Wrap(err, "encoding cert")
	}
	keyBuf := &bytes.Buffer{}
	if err := pem.Encode(keyBuf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		return nil, nil, errors.Wrap(err, "encoding key")
	}
	return certBuf.Bytes(), keyBuf.Bytes(), nil
}

func (cr *CertRotator) lookaheadTime() time.Time {
	return time.Now().Add(cr.LookaheadInterval)
}

func (cr *CertRotator) validServerCert(caCert, cert, key []byte) bool {
	valid, err := ValidCert(caCert, cert, key, cr.DNSName, cr.extKeyUsages, cr.lookaheadTime())
	if err != nil {
		return false
	}
	return valid
}

func (cr *CertRotator) validCACert(cert, key []byte) bool {
	valid, err := ValidCert(cert, cert, key, cr.CAName, nil, cr.lookaheadTime())
	if err != nil {
		return false
	}
	return valid
}

func (cr *CertRotator) generateNamespaceTLS(namespace string) (*corev1.Secret, error) {
	n := len(cr.artifactCaches)
	// get last artifact cache
	artifactCache := cr.artifactCaches[n-1]
	caArtifacts := artifactCache.ca

	hostname := fmt.Sprintf("*.%s.pod.cluster.local", namespace)
	cert, key, err := cr.createCertPEM(caArtifacts, hostname, time.Now().Add(-1*time.Hour), caArtifacts.validUntil)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s-%d", infrav1.RunnerTLSSecretName, caArtifacts.validUntil.Unix())
	tlsCertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				infrav1.RunnerLabel: "true",
			},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			caCertName: caArtifacts.CertPEM,
			caKeyName:  caArtifacts.KeyPEM,
			certName:   cert,
			keyName:    key,
		},
	}

	if err := cr.writer.Create(context.TODO(), tlsCertSecret); err != nil {
		return nil, err
	}

	return tlsCertSecret, nil
}

func (cr *CertRotator) ResetCACache() {
	cr.artifactCaches = []*artifact{}
}

func ValidCert(caCert, cert, key []byte, dnsName string, keyUsages *[]x509.ExtKeyUsage, at time.Time) (bool, error) {
	if len(caCert) == 0 || len(cert) == 0 || len(key) == 0 {
		return false, errors.New("empty cert")
	}

	pool := x509.NewCertPool()
	caDer, _ := pem.Decode(caCert)
	if caDer == nil {
		return false, errors.New("bad CA cert")
	}
	cac, err := x509.ParseCertificate(caDer.Bytes)
	if err != nil {
		return false, errors.Wrap(err, "parsing CA cert")
	}
	pool.AddCert(cac)

	_, err = tls.X509KeyPair(cert, key)
	if err != nil {
		return false, errors.Wrap(err, "building key pair")
	}

	b, _ := pem.Decode(cert)
	if b == nil {
		return false, errors.New("bad private key")
	}

	crt, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return false, errors.Wrap(err, "parsing cert")
	}

	opt := x509.VerifyOptions{
		DNSName:     dnsName,
		Roots:       pool,
		CurrentTime: at,
	}
	if keyUsages != nil {
		opt.KeyUsages = *keyUsages
	}

	_, err = crt.Verify(opt)
	if err != nil {
		return false, errors.Wrap(err, "verifying cert")
	}
	return true, nil
}
