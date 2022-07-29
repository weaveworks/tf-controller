package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	. "github.com/onsi/gomega"
	gs "github.com/onsi/gomega/gstruct"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000241_auto_approve_with_health_checks_test(t *testing.T) {
	Spec("This spec describes behaviour when health checks are specified")

	const (
		sourceName    = "health-check-auto-approve"
		terraformName = "tf-health-check-auto-approve"
	)

	g := NewWithT(t)
	ctx := context.Background()
	updatedTime := time.Now()

	It("should do health checks")
	By("creating a new Git repository object")
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
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())

	By("setting the git repo status object, the URL, and the correct checksum")
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
		URL: server.URL() + "/tf-health-check.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tf-health-check.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "84b1410341b0e87d811bb1b812741e84a74ea00db851f88fd0855589b5093d14", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

	By("creating a new TF and attaching to the repo, with approve plan set to auto and health checks defined")
	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

	inClusterConfig := false
	healthCheckTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			Interval:    metav1.Duration{Duration: time.Second * 5},
			ApprovePlan: "auto",
			Path:        "./tf-health-check-example",
			BackendConfig: &infrav1.BackendConfigSpec{
				SecretSuffix:    terraformName,
				InClusterConfig: &inClusterConfig,
				ConfigPath:      testEnvKubeConfigPath,
			},
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			HealthChecks: []infrav1.HealthCheck{
				{
					Name:    "tcpTest",
					Address: "{{.foo}}:{{.port}}",
					Type:    "tcp",
				},
				{
					Name:    "httpTest",
					URL:     server.URL() + "{{.bar}}",
					Type:    "http",
					Timeout: &metav1.Duration{Duration: time.Second * 5},
				},
			},
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: "tf-output-" + terraformName,
			},
		},
	}
	g.Expect(k8sClient.Create(ctx, &healthCheckTF)).Should(Succeed())

	By("checking that the health check example TF gets created")
	healthCheckTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdhealthCheckTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, healthCheckTFKey, &createdhealthCheckTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	By("checking that the plan was applied successfully")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, healthCheckTFKey, &createdhealthCheckTF)
		if err != nil {
			return nil
		}
		for _, c := range createdhealthCheckTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": createdhealthCheckTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            "Apply",
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-master-b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
	}))

	By("checking that we have outputs available in the TF object")
	idFn := func(element interface{}) string {
		return fmt.Sprintf("%v", element)
	}
	g.Eventually(func() []string {
		err := k8sClient.Get(ctx, healthCheckTFKey, &createdhealthCheckTF)
		if err != nil {
			return nil
		}
		return createdhealthCheckTF.Status.AvailableOutputs
	}, timeout, interval).Should(gs.MatchAllElements(idFn, gs.Elements{
		"foo":  Equal("foo"),
		"bar":  Equal("bar"),
		"port": Equal("port"),
	}))

	It("should be reconciled and produce the correct output secret.")
	By("checking that the named output secret contains all outputs.")
	outputKey := types.NamespacedName{Namespace: "flux-system", Name: "tf-output-" + terraformName}
	outputSecret := corev1.Secret{}
	g.Eventually(func() (int, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		if err != nil {
			return -1, err
		}
		return len(outputSecret.Data), nil
	}, timeout, interval).Should(Equal(3))

	By("checking that the output secret contains the correct output data, provisioned by the TF resource.")
	expectedOutputValue := map[string]string{
		"Name":        "tf-output-" + terraformName,
		"Namespace":   "flux-system",
		"FooValue":    "weave.works",
		"BarValue":    "/get",
		"PortValue":   "80",
		"OwnerRef[0]": string(createdhealthCheckTF.UID),
	}
	g.Eventually(func() (map[string]string, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		fooValue := string(outputSecret.Data["foo"])
		barValue := string(outputSecret.Data["bar"])
		portValue := string(outputSecret.Data["port"])
		return map[string]string{
			"Name":        outputSecret.Name,
			"Namespace":   outputSecret.Namespace,
			"FooValue":    fooValue,
			"BarValue":    barValue,
			"PortValue":   portValue,
			"OwnerRef[0]": string(outputSecret.OwnerReferences[0].UID),
		}, err
	}, timeout, interval).Should(Equal(expectedOutputValue), "expected output %v", expectedOutputValue)

	By("checking that the health checks are performed successfully")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, healthCheckTFKey, &createdhealthCheckTF)
		if err != nil {
			return nil
		}
		for _, c := range createdhealthCheckTF.Status.Conditions {
			if c.Type == "HealthCheck" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    "HealthCheck",
		"Reason":  "HealthChecksSucceed",
		"Message": "Health checks succeeded",
	}))
}
