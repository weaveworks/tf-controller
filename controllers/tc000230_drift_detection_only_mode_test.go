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

func Test_000230_drift_detection_only_mode(t *testing.T) {
	Spec("This spec describes setting drift detection only mode which will skip the plan and apply stages")

	const (
		sourceName    = "gr-drift-detection-only-mode"
		terraformName = "tr-drift-detection-only-mode"
	)

	g := NewWithT(t)
	ctx := context.Background()

	Given("a GitRepository")
	By("defining a new GitRepository resource.")
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
	updatedTime := time.Now()
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

	Given("a Terraform resource with approvePlan set to disable, attached to the given GitRepository resource.")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

	inClusterConfig := false
	testTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			BackendConfig: &infrav1.BackendConfigSpec{
				SecretSuffix:    terraformName,
				InClusterConfig: &inClusterConfig,
				ConfigPath:      testEnvKubeConfigPath,
			},
			ApprovePlan: infrav1.ApprovePlanAutoValue,
			Interval:    metav1.Duration{Duration: 5 * time.Second},
			Path:        "./tf-k8s-configmap",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
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
	g.Expect(k8sClient.Create(ctx, &testTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testTF)).Should(Succeed()) }()

	By("checking that the TF resource existed inside the cluster.")
	testTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdTFResource := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, testTFKey, &createdTFResource)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should be reconciled and contain some status conditions.")
	By("checking that the TF resource's status conditions has some elements.")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, testTFKey, &createdTFResource)
		if err != nil {
			return -1
		}
		return len(createdTFResource.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	It("should create and apply the plan successfully")
	By("checking that the Ready condition is True with reason TerraformAppliedSucceed")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, testTFKey, &createdTFResource)
		if err != nil {
			return nil
		}
		for _, c := range createdTFResource.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]interface{}{
					"Type":   c.Type,
					"Reason": c.Reason,
				}
			}
		}
		return createdTFResource.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":   "Ready",
		"Reason": infrav1.TFExecApplySucceedReason,
	}))

	It("should be in drift detection only mode")
	By("setting approvePlan to disable")
	createdTFResource.Spec.ApprovePlan = infrav1.ApprovePlanDisableValue
	g.Expect(k8sClient.Update(ctx, &createdTFResource)).Should(Succeed())

	By("checking that the Ready condition is true with reason NoDrift")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, testTFKey, &createdTFResource)
		if err != nil {
			return nil
		}
		for _, c := range createdTFResource.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]interface{}{
					"Type":   c.Type,
					"Status": c.Status,
					"Reason": c.Reason,
				}
			}
		}
		return createdTFResource.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":   "Ready",
		"Status": metav1.ConditionTrue,
		"Reason": infrav1.NoDriftReason,
	}))

	It("should ensure that drift detection only mode is not impacted by spec.force=true")
	By("setting force to true")
	createdTFResource.Spec.Force = true
	g.Expect(k8sClient.Update(ctx, &createdTFResource)).Should(Succeed())

	By("checking that the Ready condition remains true with reason NoDrift")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, testTFKey, &createdTFResource)
		if err != nil {
			return nil
		}
		for _, c := range createdTFResource.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]interface{}{
					"Type":   c.Type,
					"Status": c.Status,
					"Reason": c.Reason,
				}
			}
		}
		return createdTFResource.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":   "Ready",
		"Status": metav1.ConditionTrue,
		"Reason": infrav1.NoDriftReason,
	}))

	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, testTFKey, &createdTFResource)
		if err != nil {
			return false
		}
		return createdTFResource.Status.LastDriftDetectedAt.IsZero()
	}, timeout, interval).Should(BeTrue())

	It("should continue to detect and report drift")
	By("deleting configmap to create a drift")
	cmPayloadKey := types.NamespacedName{Namespace: "default", Name: "cm-" + terraformName}
	var cmPayload corev1.ConfigMap
	g.Eventually(func() string {
		err := k8sClient.Get(ctx, cmPayloadKey, &cmPayload)
		if err != nil {
			return ""
		}
		return cmPayload.Name
	}, timeout, interval).Should(Equal("cm-" + terraformName))

	g.Expect(k8sClient.Delete(ctx, &cmPayload)).Should(Succeed())

	By("checking that the Ready condition transitioned to False with reason DriftDetected")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, testTFKey, &createdTFResource)
		if err != nil {
			return nil
		}
		for _, c := range createdTFResource.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]interface{}{
					"Type":   c.Type,
					"Status": c.Status,
					"Reason": c.Reason,
				}
			}
		}
		return createdTFResource.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":   "Ready",
		"Status": metav1.ConditionFalse,
		"Reason": infrav1.DriftDetectedReason,
	}))

	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, testTFKey, &createdTFResource)
		if err != nil {
			return false
		}
		if createdTFResource.Status.LastDriftDetectedAt == nil {
			return false
		}
		return !(*createdTFResource.Status.LastDriftDetectedAt).IsZero()
	}, timeout, interval).Should(BeTrue())
}
