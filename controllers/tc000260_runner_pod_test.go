package controllers

import (
	"context"
	"os"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000260_runner_pod_test(t *testing.T) {
	Spec("This spec describes a runner pod creation process")

	const (
		terraformName      = "runner-pod-test"
		sourceName         = "runner-pod-test"
		serviceAccountName = "helloworld-tf-runner"
		runnerPodImage     = "ghcr.io/weaveworks/tf-runner:test"
	)

	var stringMap = map[string]string{
		"company.com/abc": "xyz",
		"company.com/xyz": "abc",
	}

	g := NewWithT(t)

	It("generate a runner pod template")
	By("passing a terraform object, the runner pod template should be accurate")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			ServiceAccountName: serviceAccountName,
			RunnerPodTemplate: infrav1.RunnerPodTemplate{
				Metadata: infrav1.RunnerPodMetadata{
					Labels:      stringMap,
					Annotations: stringMap,
				},
				Spec: infrav1.RunnerPodSpec{
					Image: runnerPodImage,
				},
			},
		},
	}

	spec := reconciler.runnerPodSpec(helloWorldTF)
	g.Expect(spec.ServiceAccountName == serviceAccountName)
	g.Expect(spec.Containers[0].Image == runnerPodImage)

	podTemplate := runnerPodTemplate(helloWorldTF)
	g.Expect(func() bool {
		for k, v := range stringMap {
			if v != podTemplate.ObjectMeta.Labels[k] {
				return false
			}
		}
		for k, v := range stringMap {
			if v != podTemplate.ObjectMeta.Annotations[k] {
				return false
			}
		}
		return true
	})
}

func Test_000260_runner_pod_test_env_vars(t *testing.T) {
	Spec("This spec describes a runner pod creation process")

	const (
		terraformName      = "runner-pod-test"
		sourceName         = "runner-pod-test"
		serviceAccountName = "helloworld-tf-runner"
		runnerPodImage     = "ghcr.io/weaveworks/tf-runner:test"
	)

	var stringMap = map[string]string{
		"company.com/abc": "xyz",
		"company.com/xyz": "abc",
	}

	g := NewWithT(t)

	It("generate a runner pod template")
	By("passing a terraform object, the runner pod template should be accurate")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			ServiceAccountName: serviceAccountName,
			RunnerPodTemplate: infrav1.RunnerPodTemplate{
				Metadata: infrav1.RunnerPodMetadata{
					Labels:      stringMap,
					Annotations: stringMap,
				},
				Spec: infrav1.RunnerPodSpec{
					Image: runnerPodImage,
					Env: []corev1.EnvVar{
						{
							Name:  "TEST_NAME",
							Value: "TEST_VALUE",
						},
						{
							Name:  "TEST_NAME_2",
							Value: "TEST_VALUE_2",
						},
					},
				},
			},
		},
	}

	spec := reconciler.runnerPodSpec(helloWorldTF)
	g.Expect(spec.ServiceAccountName == serviceAccountName)
	g.Expect(spec.Containers[0].Image == runnerPodImage)
	g.Expect(len(spec.Containers[0].Env) == 4)
	g.Expect(spec.Containers[0].Env[2].Name == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[0].Name)
	g.Expect(spec.Containers[0].Env[2].Value == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[0].Value)
	g.Expect(spec.Containers[0].Env[3].Name == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[1].Name)
	g.Expect(spec.Containers[0].Env[3].Value == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[1].Value)

	podTemplate := runnerPodTemplate(helloWorldTF)
	g.Expect(func() bool {
		for k, v := range stringMap {
			if v != podTemplate.ObjectMeta.Labels[k] {
				return false
			}
		}
		for k, v := range stringMap {
			if v != podTemplate.ObjectMeta.Annotations[k] {
				return false
			}
		}
		return true
	})
}

func Test_000260_runner_pod_test_env_vars_proxy(t *testing.T) {
	Spec("This spec describes a runner pod creation process")

	const (
		terraformName      = "runner-pod-test"
		sourceName         = "runner-pod-test"
		serviceAccountName = "helloworld-tf-runner"
		runnerPodImage     = "ghcr.io/weaveworks/tf-runner:test"
	)

	var stringMap = map[string]string{
		"company.com/abc": "xyz",
		"company.com/xyz": "abc",
	}

	os.Setenv("HTTP_PROXY", "http://runner_pod_test_env_vars_proxy:1234")
	os.Setenv("HTTPS_PROXY", "http://runner_pod_test_env_vars_proxy:1234")
	os.Setenv("NO_PROXY", "runner.pod.test.env.vars.proxy")
	defer func() {
		os.Setenv("HTTP_PROXY", "")
		os.Setenv("HTTPS_PROXY", "")
		os.Setenv("NO_PROXY", "")
	}()

	g := NewWithT(t)

	It("generate a runner pod template")
	By("passing a terraform object, the runner pod template should be accurate")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			ServiceAccountName: serviceAccountName,
			RunnerPodTemplate: infrav1.RunnerPodTemplate{
				Metadata: infrav1.RunnerPodMetadata{
					Labels:      stringMap,
					Annotations: stringMap,
				},
				Spec: infrav1.RunnerPodSpec{
					Image: runnerPodImage,
					Env: []corev1.EnvVar{
						{
							Name:  "TEST_NAME",
							Value: "TEST_VALUE",
						},
						{
							Name:  "TEST_NAME_2",
							Value: "TEST_VALUE_2",
						},
					},
				},
			},
		},
	}

	spec := reconciler.runnerPodSpec(helloWorldTF)
	g.Expect(spec.ServiceAccountName == serviceAccountName)
	g.Expect(spec.Containers[0].Image == runnerPodImage)
	g.Expect(len(spec.Containers[0].Env) == 7)
	g.Expect(spec.Containers[0].Env[5].Name == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[0].Name)
	g.Expect(spec.Containers[0].Env[5].Value == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[0].Value)
	g.Expect(spec.Containers[0].Env[6].Name == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[1].Name)
	g.Expect(spec.Containers[0].Env[6].Value == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[1].Value)

	podTemplate := runnerPodTemplate(helloWorldTF)
	g.Expect(func() bool {
		for k, v := range stringMap {
			if v != podTemplate.ObjectMeta.Labels[k] {
				return false
			}
		}
		for k, v := range stringMap {
			if v != podTemplate.ObjectMeta.Annotations[k] {
				return false
			}
		}
		return true
	})
}

func Test_000260_runner_pod_test_env_vars_proxy_overwrite(t *testing.T) {
	Spec("This spec describes a runner pod creation process")

	const (
		terraformName      = "runner-pod-test"
		sourceName         = "runner-pod-test"
		serviceAccountName = "helloworld-tf-runner"
		runnerPodImage     = "ghcr.io/weaveworks/tf-runner:test"
	)

	var stringMap = map[string]string{
		"company.com/abc": "xyz",
		"company.com/xyz": "abc",
	}

	os.Setenv("HTTP_PROXY", "http://runner_pod_test_env_vars_proxy_overwrite:1234")
	os.Setenv("HTTPS_PROXY", "http://runner_pod_test_env_vars_proxy_overwrite:1234")
	os.Setenv("NO_PROXY", "runner.pod.test.env.vars.proxy.overwrite")
	defer func() {
		os.Setenv("HTTP_PROXY", "")
		os.Setenv("HTTPS_PROXY", "")
		os.Setenv("NO_PROXY", "")
	}()

	g := NewWithT(t)

	It("generate a runner pod template")
	By("passing a terraform object, the runner pod template should be accurate")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			ServiceAccountName: serviceAccountName,
			RunnerPodTemplate: infrav1.RunnerPodTemplate{
				Metadata: infrav1.RunnerPodMetadata{
					Labels:      stringMap,
					Annotations: stringMap,
				},
				Spec: infrav1.RunnerPodSpec{
					Image: runnerPodImage,
					Env: []corev1.EnvVar{
						{
							Name:  "TEST_NAME",
							Value: "TEST_VALUE",
						},
						{
							Name:  "TEST_NAME_2",
							Value: "TEST_VALUE_2",
						},
						{
							Name:  "HTTP_PROXY",
							Value: "http://runner_pod_test_env_vars_proxy_overwrite:1235",
						},
					},
				},
			},
		},
	}

	spec := reconciler.runnerPodSpec(helloWorldTF)
	g.Expect(spec.ServiceAccountName == serviceAccountName)
	g.Expect(spec.Containers[0].Image == runnerPodImage)
	g.Expect(len(spec.Containers[0].Env) == 7)
	g.Expect(spec.Containers[0].Env[5].Name == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[0].Name)
	g.Expect(spec.Containers[0].Env[5].Value == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[0].Value)
	g.Expect(spec.Containers[0].Env[6].Name == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[1].Name)
	g.Expect(spec.Containers[0].Env[6].Value == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[1].Value)

	g.Expect(spec.Containers[0].Env[2].Name == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[2].Name)
	g.Expect(spec.Containers[0].Env[2].Value == helloWorldTF.Spec.RunnerPodTemplate.Spec.Env[2].Value)

	podTemplate := runnerPodTemplate(helloWorldTF)
	g.Expect(func() bool {
		for k, v := range stringMap {
			if v != podTemplate.ObjectMeta.Labels[k] {
				return false
			}
		}
		for k, v := range stringMap {
			if v != podTemplate.ObjectMeta.Annotations[k] {
				return false
			}
		}
		return true
	})
}

func Test_000260_runner_pod_test_env_vars_proxy_output(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource when variables are provided via EnvVars.")
	It("should be reconciled and output the variable in an output.")

	const (
		sourceName          = "gr-envvar-variable-output"
		terraformName       = "tf-envvar-variable-output"
		terraformNameSecret = "tf-envvar-variable-output-secret"
	)
	g := NewWithT(t)
	ctx := context.Background()

	testEnvKubeConfigPath, err := findKubeConfig(testEnv)
	g.Expect(err).Should(BeNil())

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

	Given("the GitRepository's reconciled status")
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
		URL: server.URL() + "/terraform-envvar-variable-output.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/terraform-envvar-variable-output.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "72637c4e56394f5e728c7eaf85f959497189cc35d1441957840a96812026a5d6",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled.")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := &sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, createdRepo)).Should(Succeed())

	Given("a Terraform resource with auto approve, attached to the given GitRepository resource")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	By("specifying the .spec.writeOutputsToSecret.")

	inClusterConfig := false
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			BackendConfig: &infrav1.BackendConfigSpec{
				SecretSuffix:    terraformName,
				InClusterConfig: &inClusterConfig,
				ConfigPath:      testEnvKubeConfigPath,
			},
			Path: "./terraform-envvar-variable-output",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			RunnerPodTemplate: infrav1.RunnerPodTemplate{
				Spec: infrav1.RunnerPodSpec{
					Env: []corev1.EnvVar{
						{
							Name:  "HTTP_PROXY",
							Value: "http://runner_pod_test_env_vars_proxy_output:1234",
						},
						{
							Name:  "HTTPS_PROXY",
							Value: "http://runner_pod_test_env_vars_proxy_output:1234",
						},
						{
							Name:  "NO_PROXY",
							Value: "cluster.local,terraform.io,registry.terraform.io,releases.hashicorp.com",
						},
					},
				},
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: terraformNameSecret,
			},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

	By("checking that the TF resource existed inside the cluster.")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should apply successfully.")
	By("checking that the status of the TF resource is `TerraformAppliedSucceed`.")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":   c.Type,
					"Reason": c.Reason,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":   "Apply",
		"Reason": infrav1.TFExecApplySucceedReason,
	}))

	It("should be reconciled and produce the correct output secret.")
	By("checking that the named output secret contains 3 data fields.")
	outputKey := types.NamespacedName{Namespace: "flux-system", Name: terraformNameSecret}
	outputSecret := corev1.Secret{}
	g.Eventually(func() (int, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		if err != nil {
			return -1, err
		}
		return len(outputSecret.Data), nil
	}, timeout*2, interval).Should(Equal(3))

	By("checking that the output secret contains the correct output data, provisioned by the TF resource.")
	expectedOutputValue := map[string]string{
		"Name":              terraformNameSecret,
		"Namespace":         "flux-system",
		"Value HTTPS_PROXY": "http://runner_pod_test_env_vars_proxy_output:1234",
		"Value HTTP_PROXY":  "http://runner_pod_test_env_vars_proxy_output:1234",
		"Value NO_PROXY":    "cluster.local,terraform.io,registry.terraform.io,releases.hashicorp.com",
		"OwnerRef[0]":       string(createdHelloWorldTF.UID),
	}
	g.Eventually(func() (map[string]string, error) {
		err := k8sClient.Get(ctx, outputKey, &outputSecret)
		return map[string]string{
			"Name":              outputSecret.Name,
			"Namespace":         outputSecret.Namespace,
			"Value HTTPS_PROXY": string(outputSecret.Data["https_proxy"]),
			"Value HTTP_PROXY":  string(outputSecret.Data["http_proxy"]),
			"Value NO_PROXY":    string(outputSecret.Data["no_proxy"]),
			"OwnerRef[0]":       string(outputSecret.OwnerReferences[0].UID),
		}, err
	}, timeout, interval).Should(Equal(expectedOutputValue), "expected output %v", expectedOutputValue)

}
