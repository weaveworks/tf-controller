package controllers

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/mtls"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_009990_mtls_generate_creds_test(t *testing.T) {
	Spec("This spec describes the behaviour when generating tls certs for mTLS")

	const (
		sourceName    = "gs-mtls-creds-generate-test"
		terraformName = "tf-mtls-creds-generate-test"
	)

	g := NewWithT(t)
	ctx := context.Background()
	updatedTime := time.Now()

	rotator.ResetCACache()
	readyCh := make(chan *mtls.TriggerResult)
	rotator.TriggerCARotation <- mtls.Trigger{Namespace: "", Ready: readyCh}
	result := <-readyCh
	g.Expect(result.Err).To(BeNil())

	caSecret := result.Secret
	g.Expect(len(caSecret.Data)).To(Equal(4))

	It("should create the ca and runner certs")
	g.Expect(caSecret.Data).Should(HaveKey("ca.crt"))
	g.Expect(caSecret.Data).Should(HaveKey("ca.key"))
	g.Expect(caSecret.Data).Should(HaveKey("tls.crt"))
	g.Expect(caSecret.Data).Should(HaveKey("tls.key"))

	By("creating the GitRepository resource in the cluster.")
	testRepo := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "master",
			},
			Interval:          metav1.Duration{Duration: time.Second * 30},
			GitImplementation: "go-git",
		},
	}
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())

	Given("the GitRepository's reconciled status.")
	By("setting the GitRepository's status, with the downloadable BLOB's URL, and the correct checksum.")
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},
		URL: server.URL() + "/file.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	Given("a Terraform resource  attached to the given GitRepository resource.")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			Path: "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())

	By("checking that the TF resource exists.")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	By("checking that the runner secret gets created")

	secretName, err := rotator.GetRunnerTLSSecretName()
	g.Expect(err).Should(Succeed())

	runnerSecret := &corev1.Secret{}
	runnerSecretKey := types.NamespacedName{Namespace: "flux-system", Name: secretName}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, runnerSecretKey, runnerSecret)
		if err != nil {
			return false
		}
		return len(runnerSecret.Data) == 4
	}, timeout, interval).Should(BeTrue())

	By("verifying that the certificate authority cert is valid")
	caCert := runnerSecret.Data["ca.crt"]
	caKey := runnerSecret.Data["ca.key"]
	caValid, err := mtls.ValidCert(caCert, caCert, caKey, rotator.CAName, nil, time.Now())
	g.Expect(err).To(BeNil())
	g.Expect(caValid).To(BeTrue())

	By("verifying that the runner cert secret has the appropriate labels")
	labels := runnerSecret.ObjectMeta.GetLabels()
	g.Expect(labels).To(HaveKey(infrav1.RunnerLabel))

	By("verifying that the runner cert is valid for a range of hostnames in a given namespace")
	tlsCert := runnerSecret.Data["tls.crt"]
	tlsKey := runnerSecret.Data["tls.key"]
	for _, ip := range []string{"172-1-0-1", "172-10-0-2", "172-127-0-3"} {
		hostname := fmt.Sprintf("%s.flux-system.pod.cluster.local", ip)
		tlsValid, err := mtls.ValidCert(caCert, tlsCert, tlsKey, hostname, nil, time.Now())
		g.Expect(err).To(BeNil())
		g.Expect(tlsValid).To(BeTrue())
	}

	By("verifying that the runner cert is valid only for the terraform runner namespace")
	hostname := "172-1-0-1.kube-system.pod.cluster.local"
	tlsValid, err := mtls.ValidCert(caCert, tlsCert, tlsKey, hostname, nil, time.Now())
	g.Expect(err).ToNot(BeNil())
	g.Expect(tlsValid).To(BeFalse())

	By("rotating the CA should renew the server cert")
	rotator.ResetCACache()
	renewedReadyCh := make(chan *mtls.TriggerResult)
	rotator.TriggerCARotation <- mtls.Trigger{Namespace: "", Ready: renewedReadyCh}
	renewedResult := <-renewedReadyCh
	renewedCaSecret := renewedResult.Secret

	g.Expect(bytes.Compare(caSecret.Data["ca.crt"], renewedCaSecret.Data["ca.crt"])).ToNot(BeZero())

	By("checking that the runner secret gets updated")

	renewedSecretName, err := rotator.GetRunnerTLSSecretName()
	g.Expect(err).Should(Succeed())
	renewedRunnerSecretKey := types.NamespacedName{Namespace: "flux-system", Name: renewedSecretName}

	updatedRunnerSecret := &corev1.Secret{}
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, renewedRunnerSecretKey, updatedRunnerSecret)
		if err != nil {
			return 1
		}
		return bytes.Compare(runnerSecret.Data["tls.crt"], updatedRunnerSecret.Data["tls.crt"])
	}, timeout, interval).ShouldNot(BeZero())

	g.Expect(bytes.Compare(runnerSecret.Data["tls.crt"], updatedRunnerSecret.Data["tls.crt"])).ToNot(BeZero())

	By("ensuring that the refreshed runner cert is valid")
	hostname = "172-1-0-1.flux-system.pod.cluster.local"
	caCert = renewedCaSecret.Data["ca.crt"]
	tlsCert = updatedRunnerSecret.Data["tls.crt"]
	tlsKey = updatedRunnerSecret.Data["tls.key"]
	tlsValid, err = mtls.ValidCert(caCert, tlsCert, tlsKey, hostname, nil, time.Now())
	g.Expect(err).To(BeNil())
	g.Expect(tlsValid).To(BeTrue())
}
