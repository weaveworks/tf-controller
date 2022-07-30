package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"

	"github.com/weaveworks/tf-controller/runner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

// StartGRPCServerForTesting should be used only for testing
func StartGRPCServerForTesting(server *runner.TerraformRunnerServer, namespace string, addr string, mgr controllerruntime.Manager, rotator *CertRotator) error {
	// wait for the certs to be available and the manager to be ready
	<-rotator.Ready
	<-mgr.Elected()

	trigger := Trigger{
		Namespace: namespace,
		Ready:     make(chan *TriggerResult),
	}

	rotator.TriggerNamespaceTLSGeneration <- trigger
	result := <-trigger.Ready
	if result.Err != nil {
		return result.Err
	}

	creds, err := GetGRPCServerCredentials(result.Secret)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer(grpc.Creds(creds))

	// local runner, use the same client as the manager
	runner.RegisterRunnerServer(grpcServer, server)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil
	}
	if err := grpcServer.Serve(listener); err != nil {
		return err
	}

	return nil
}

// GetGRPCClientCredentials returns transport credentials for a client connection
func GetGRPCClientCredentials(secret *corev1.Secret) (credentials.TransportCredentials, error) {
	ca, cert, err := buildArtifactsFromSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to build artifacts from Secret: %v", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(ca.CertPEM) {
		return nil, fmt.Errorf("failed to add client CA")
	}

	runnerCert, err := tls.X509KeyPair(cert.CertPEM, cert.KeyPEM)
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{runnerCert},
		RootCAs:      certPool,
	}

	return credentials.NewTLS(config), nil
}

// GetGRPCServerCredentials returns transport credentials for a server
func GetGRPCServerCredentials(secret *corev1.Secret) (credentials.TransportCredentials, error) {
	ca, cert, err := buildArtifactsFromSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to build artifacts from Secret: %v", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(ca.CertPEM) {
		return nil, fmt.Errorf("failed to add client CA")
	}

	serverCert, err := tls.X509KeyPair(cert.CertPEM, cert.KeyPEM)
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    certPool,
	}

	return credentials.NewTLS(config), nil
}
