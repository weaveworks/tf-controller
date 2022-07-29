package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/weaveworks/tf-controller/utils"

	. "github.com/onsi/gomega"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000160_auto_applied_should_tx_to_plan_when_unrelated_source_changed_test(t *testing.T) {

	Spec("This spec describes the behaviour of a Terraform resource with auto approve, when an unrelated non-TF file triggers a source change.")
	It("should transition to planned, with no change.")
	It("should contain the *new* revision in .status.lastAttemptedRevision.")
	It("should contain the *old* revision in .status.lastAppliedRevision.")
	It("should contain the *new* revision in .status.lastPlannedRevision.")

	const (
		sourceName    = "gr-unrelated-source-changed-auto-approve-no-output"
		terraformName = "tf-unrelated-changed-auto-approve-no-output"
	)
	ctx := context.Background()
	g := NewWithT(t)

	Given("a GitRepository")
	By("defining a new GitRepository resource.")
	updatedTime := time.Now()
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

	By("creating the GitRepository resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testRepo)).Should(Succeed()) }()

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
		URL: server.URL() + "/tf-k8s-configmap.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/tf-k8s-configmap.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "c3bf30bad9621b5110a3761a70754170d1dae6c525a63098b6ec9919efac3555", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

	Given("a Terraform resource with auto approve, attached to the given GitRepository resource.")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	inClusterConfig := false
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./tf-k8s-configmap",
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
			Interval: metav1.Duration{Duration: time.Second * 10},
			Vars: []infrav1.Variable{
				{
					Name:  "kubeconfig",
					Value: utils.MustJSONEncodeBytes(t, []byte(testEnvKubeConfigPath)),
				},
				{
					Name:  "context",
					Value: utils.MustJSONEncodeBytes(t, []byte("envtest")),
				},
				{
					Name:  "config_name",
					Value: utils.MustJSONEncodeBytes(t, []byte("cm-"+terraformName)),
				},
			},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	By("checking that the hello world TF get created")
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	By("checking that the TF condition got created")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	By("checking that the applied status of the TF program Successfully, and plan-master-b8e362c206e is applied")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            "Apply",
		"Reason":          infrav1.TFExecApplySucceedReason,
		"LastAppliedPlan": "plan-master-b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
	}))

	By("checking that the config map payload got created.")
	cmPayloadKey := types.NamespacedName{Namespace: "default", Name: "cm-" + terraformName}
	var cmPayload corev1.ConfigMap
	g.Eventually(func() string {
		err := k8sClient.Get(ctx, cmPayloadKey, &cmPayload)
		if err != nil {
			return ""
		}
		return cmPayload.Name
	}, timeout, interval).Should(Equal("cm-" + terraformName))

	Given("a new revision of resource, triggered by a non-TF file change.")
	By("changing source to a new revision.")
	updatedTime = time.Now()
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(2),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/ed22ced771a0056455a2fbb8e362c206e3d0cbb7",
			},
		},
		URL: server.URL() + "/tf-k8s-configmap-unrelated-change.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/ed22ced771a0056455a2fbb8e362c206e3d0cbb7.tar.gz",
			URL:            server.URL() + "/tf-k8s-configmap-unrelated-change.tar.gz",
			Revision:       "master/ed22ced771a0056455a2fbb8e362c206e3d0cbb7",
			Checksum:       "31edb23a8227e8bcd2034cc2223919eee83e9c20f27166535503dc3c1f4326dc", // must be the real checksum value
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the planned secret should be updated, even with no change - because we did plan.")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	tfplanSecret := corev1.Secret{}
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"SavedPlan":             tfplanSecret.Annotations["savedPlan"],
			"TFPlanEmpty":           string(tfplanSecret.Data["tfplan"]) == "",
			"HasEncodingAnnotation": tfplanSecret.Annotations["encoding"] != "" && tfplanSecret.Annotations["encoding"] == "gzip",
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"SavedPlan":             "plan-master-ed22ced771a0056455a2fbb8e362c206e3d0cbb7",
		"TFPlanEmpty":           false,
		"HasEncodingAnnotation": true,
	}))

	By("checking that the status of the TF resource must be transitioned to planned, no change.")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Plan" {
				return map[string]interface{}{
					"Type":                  c.Type,
					"Reason":                c.Reason,
					"Pending":               createdHelloWorldTF.Status.Plan.Pending,
					"LastAppliedPlan":       createdHelloWorldTF.Status.Plan.LastApplied,
					"Message":               c.Message,
					"LastAppliedRevision":   createdHelloWorldTF.Status.LastAppliedRevision,
					"LastAttemptedRevision": createdHelloWorldTF.Status.LastAttemptedRevision,
					"LastPlannedRevision":   createdHelloWorldTF.Status.LastPlannedRevision,
				}
			}
		}
		return nil // plan-master-b8e362c206e3d0cbb7ed22ced771a0056455a2fb
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":                  "Plan",
		"Reason":                "TerraformPlannedNoChanges",
		"LastAppliedRevision":   "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
		"LastAttemptedRevision": "master/ed22ced771a0056455a2fbb8e362c206e3d0cbb7",
		"LastPlannedRevision":   "master/ed22ced771a0056455a2fbb8e362c206e3d0cbb7",
		"LastAppliedPlan":       "plan-master-b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
		"Pending":               "",
		"Message":               "Plan no changes",
	}))

}
