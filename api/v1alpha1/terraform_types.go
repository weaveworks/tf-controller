/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"fmt"
	"strings"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CACertSecretName = "tf-controller.tls"
	// RunnerTLSSecretName is the name of the secret containing a TLS cert that will be written to
	// the namespace in which a terraform runner is created
	RunnerTLSSecretName   = "terraform-runner.tls"
	RunnerLabel           = "infra.contrib.fluxcd.io/terraform"
	GitRepositoryIndexKey = ".metadata.gitRepository"
	BucketIndexKey        = ".metadata.bucket"
)

// WriteOutputsToSecretSpec defines where to store outputs, and which outputs to be stored.
type WriteOutputsToSecretSpec struct {
	// Name is the name of the Secret to be written
	// +required
	Name string `json:"name"`

	// Outputs contain the selected names of outputs to be written
	// to the secret. Empty array means writing all outputs, which is default.
	// +optional
	Outputs []string `json:"outputs,omitempty"`
}

type Variable struct {
	// Name is the name of the variable
	// +required
	Name string `json:"name"`

	// +optional
	Value *apiextensionsv1.JSON `json:"value,omitempty"`

	// +optional
	ValueFrom *corev1.EnvVarSource `json:"valueFrom,omitempty"`
}

// TerraformSpec defines the desired state of Terraform
type TerraformSpec struct {

	// ApprovePlan specifies name of a plan wanted to approve.
	// If its value is "auto", the controller will automatically approve every plan.
	// +optional
	ApprovePlan string `json:"approvePlan,omitempty"`

	// Destroy produces a destroy plan. Applying the plan will destroy all resources.
	// +optional
	Destroy bool `json:"destroy,omitempty"`

	// +optional
	BackendConfig *BackendConfigSpec `json:"backendConfig,omitempty"`

	// List of input variables to set for the Terraform program.
	// +optional
	Vars []Variable `json:"vars,omitempty"`

	// List of references to a Secret or a ConfigMap to generate variables for
	// Terraform resources based on its data, selectively by varsKey. Values of the later
	// Secret / ConfigMap with the samek keys will override those of the former.
	// +optional
	VarsFrom []VarsReference `json:"varsFrom,omitempty"`

	// The interval at which to reconcile the Terraform.
	// +required
	Interval metav1.Duration `json:"interval"`

	// The interval at which to retry a previously failed reconciliation.
	// When not specified, the controller uses the TerraformSpec.Interval
	// value to retry failures.
	// +optional
	RetryInterval *metav1.Duration `json:"retryInterval,omitempty"`

	// Path to the directory containing Terraform (.tf) files.
	// Defaults to 'None', which translates to the root path of the SourceRef.
	// +optional
	Path string `json:"path,omitempty"`

	// SourceRef is the reference of the source where the Terraform files are stored.
	// +required
	SourceRef CrossNamespaceSourceReference `json:"sourceRef"`

	// Suspend is to tell the controller to suspend subsequent TF executions,
	// it does not apply to already started executions. Defaults to false.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// Force instructs the controller to unconditionally
	// re-plan and re-apply TF resources. Defaults to false.
	// +kubebuilder:default:=false
	// +optional
	Force bool `json:"force,omitempty"`

	// A list of target secrets for the outputs to be written as.
	// +optional
	WriteOutputsToSecret *WriteOutputsToSecretSpec `json:"writeOutputsToSecret,omitempty"`

	// Disable automatic drift detection. Drift detection may be resource intensive in
	// the context of a large cluster or complex Terraform statefile. Defaults to false.
	// +kubebuilder:default:=false
	// +optional
	DisableDriftDetection bool `json:"disableDriftDetection,omitempty"`

	// +optional
	// PushSpec *PushSpec `json:"pushSpec,omitempty"`

	// +optional
	CliConfigSecretRef *corev1.SecretReference `json:"cliConfigSecretRef,omitempty"`

	// List of health checks to be performed.
	// +optional
	HealthChecks []HealthCheck `json:"healthChecks,omitempty"`

	// Create destroy plan and apply it to destroy terraform resources
	// upon deletion of this object. Defaults to false.
	// +kubebuilder:default:=false
	// +optional
	DestroyResourcesOnDeletion bool `json:"destroyResourcesOnDeletion,omitempty"`

	// Name of a ServiceAccount for the runner Pod to provision Terraform resources.
	// Default to tf-runner.
	// +kubebuilder:default:=tf-runner
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Clean the runner pod up after each reconciliation cycle
	// +kubebuilder:default:=true
	// +optional
	AlwaysCleanupRunnerPod *bool `json:"alwaysCleanupRunnerPod,omitempty"`

	// Configure the termination grace period for the runner pod. Use this parameter
	// to allow the Terraform process to gracefully shutdown. Consider increasing for
	// large, complex or slow-moving Terraform managed resources.
	// +kubebuilder:default:=30
	// +optional
	RunnerTerminationGracePeriodSeconds *int64 `json:"runnerTerminationGracePeriodSeconds,omitempty"`

	// RefreshBeforeApply forces refreshing of the state before the apply step.
	// +kubebuilder:default:=false
	// +optional
	RefreshBeforeApply bool `json:"refreshBeforeApply,omitempty"`

	// +optional
	RunnerPodTemplate RunnerPodTemplate `json:"runnerPodTemplate,omitempty"`

	// EnableInventory enables the object to store resource entries as the inventory for external use.
	// +optional
	EnableInventory bool `json:"enableInventory,omitempty"`
}

type PlanStatus struct {
	// +optional
	LastApplied string `json:"lastApplied,omitempty"`

	// +optional
	Pending string `json:"pending,omitempty"`

	// +optional
	IsDestroyPlan bool `json:"isDestroyPlan,omitempty"`

	// +optional
	IsDriftDetectionPlan bool `json:"isDriftDetectionPlan,omitempty"`
}

// TerraformStatus defines the observed state of Terraform
type TerraformStatus struct {
	meta.ReconcileRequestStatus `json:",inline"`

	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// The last successfully applied revision.
	// The revision format for Git sources is <branch|tag>/<commit-sha>.
	// +optional
	LastAppliedRevision string `json:"lastAppliedRevision,omitempty"`

	// LastAttemptedRevision is the revision of the last reconciliation attempt.
	// +optional
	LastAttemptedRevision string `json:"lastAttemptedRevision,omitempty"`

	// LastPlannedRevision is the revision used by the last planning process.
	// The result could be either no plan change or a new plan generated.
	// +optional
	LastPlannedRevision string `json:"lastPlannedRevision,omitempty"`

	// LastDriftDetectedAt is the time when the last drift was detected
	// +optional
	LastDriftDetectedAt *metav1.Time `json:"lastDriftDetectedAt,omitempty"`

	// LastAppliedByDriftDetectionAt is the time when the last drift was detected and
	// terraform apply was performed as a result
	// +optional
	LastAppliedByDriftDetectionAt *metav1.Time `json:"lastAppliedByDriftDetectionAt,omitempty"`

	// +optional
	AvailableOutputs []string `json:"availableOutputs,omitempty"`

	// +optional
	Plan PlanStatus `json:"plan,omitempty"`

	// Inventory contains the list of Terraform resource object references that have been successfully applied.
	// +optional
	Inventory *ResourceInventory `json:"inventory,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=tf
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description=""
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description=""

// Terraform is the Schema for the terraforms API
type Terraform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TerraformSpec   `json:"spec,omitempty"`
	Status TerraformStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TerraformList contains a list of Terraform
type TerraformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Terraform `json:"items"`
}

// BackendConfigSpec is for specifying configuration for Terraform's Kubernetes backend
type BackendConfigSpec struct {

	// Disable is to completely disable the backend configuration.
	// +optional
	Disable bool `json:"disable"`

	// +optional
	SecretSuffix string `json:"secretSuffix"`

	// +optional
	InClusterConfig bool `json:"inClusterConfig"`

	// +optional
	ConfigPath string `json:"configPath,omitempty"`

	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

const (
	TerraformKind             = "Terraform"
	TerraformFinalizer        = "finalizers.tf.contrib.fluxcd.io"
	MaxConditionMessageLength = 20000
	DisabledValue             = "disabled"
	ApprovePlanAutoValue      = "auto"
	ApprovePlanDisableValue   = "disable"

	// ArtifactFailedReason represents the fact that the
	// source artifact download failed.
	ArtifactFailedReason = "ArtifactFailed"

	TFExecNewFailedReason      = "TFExecNewFailed"
	TFExecInitFailedReason     = "TFExecInitFailed"
	VarsGenerationFailedReason = "VarsGenerationFailed"
	DriftDetectionFailedReason = "DriftDetectionFailed"
	DriftDetectedReason        = "DriftDetected"
	NoDriftReason              = "NoDrift"
	TFExecPlanFailedReason     = "TFExecPlanFailed"
	TFExecApplyFailedReason    = "TFExecApplyFailed"
	TFExecOutputFailedReason   = "TFExecOutputFailed"
	OutputsWritingFailedReason = "OutputsWritingFailed"
	HealthChecksFailedReason   = "HealthChecksFailed"
	TFExecApplySucceedReason   = "TerraformAppliedSucceed"
)

// SetTerraformReadiness sets the ReadyCondition, ObservedGeneration, and LastAttemptedRevision, on the Terraform.
func SetTerraformReadiness(terraform *Terraform, status metav1.ConditionStatus, reason, message string, revision string) {
	newCondition := metav1.Condition{
		Type:    meta.ReadyCondition,
		Status:  status,
		Reason:  reason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	terraform.Status.ObservedGeneration = terraform.Generation
	terraform.Status.LastAttemptedRevision = revision
}

func TerraformApplying(terraform Terraform, revision string, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    "Apply",
		Status:  metav1.ConditionUnknown,
		Reason:  meta.ProgressingReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	if revision != "" {
		(&terraform).Status.LastAttemptedRevision = revision
	}
	return terraform
}

func TerraformOutputsAvailable(terraform Terraform, availableOutputs []string, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    "Output",
		Status:  metav1.ConditionTrue,
		Reason:  "TerraformOutputsAvailable",
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	(&terraform).Status.AvailableOutputs = availableOutputs
	return terraform
}

func TerraformOutputsWritten(terraform Terraform, revision string, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    "Output",
		Status:  metav1.ConditionTrue,
		Reason:  "TerraformOutputsWritten",
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	SetTerraformReadiness(&terraform, metav1.ConditionTrue, "TerraformOutputsWritten", message+": "+revision, revision)
	return terraform
}

func TerraformApplied(terraform Terraform, revision string, message string, isDestroyApply bool, entries []ResourceRef) Terraform {
	newCondition := metav1.Condition{
		Type:    "Apply",
		Status:  metav1.ConditionTrue,
		Reason:  TFExecApplySucceedReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)

	if terraform.Status.Plan.IsDriftDetectionPlan {
		(&terraform).Status.LastAppliedByDriftDetectionAt = &metav1.Time{Time: time.Now()}
	}

	(&terraform).Status.Plan = PlanStatus{
		LastApplied:   terraform.Status.Plan.Pending,
		Pending:       "",
		IsDestroyPlan: isDestroyApply,
	}
	if revision != "" {
		(&terraform).Status.LastAppliedRevision = revision
	}

	if len(entries) > 0 {
		(&terraform).Status.Inventory = &ResourceInventory{Entries: entries}
	}

	SetTerraformReadiness(&terraform, metav1.ConditionTrue, TFExecApplySucceedReason, message+": "+revision, revision)
	return terraform
}

func TerraformPlannedWithChanges(terraform Terraform, revision string, message string) Terraform {
	planRev := strings.Replace(revision, "/", "-", 1)
	planId := fmt.Sprintf("plan-%s", planRev)
	newCondition := metav1.Condition{
		Type:    "Plan",
		Status:  metav1.ConditionTrue,
		Reason:  "TerraformPlannedWithChanges",
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	(&terraform).Status.Plan = PlanStatus{
		LastApplied:          terraform.Status.Plan.LastApplied,
		Pending:              planId,
		IsDestroyPlan:        terraform.Spec.Destroy,
		IsDriftDetectionPlan: terraform.HasDrift(),
	}
	if revision != "" {
		(&terraform).Status.LastAttemptedRevision = revision
		(&terraform).Status.LastPlannedRevision = revision
	}

	shortPlanId := planId
	parts := strings.SplitN(revision, "/", 2)
	if len(parts) == 2 {
		if len(parts[1]) >= 10 {
			shortPlanId = fmt.Sprintf("plan-%s-%s", parts[0], parts[1][0:10])
		}
	}
	approveMessage := fmt.Sprintf("%s: set approvePlan: \"%s\" to approve this plan.", message, shortPlanId)
	SetTerraformReadiness(&terraform, metav1.ConditionUnknown, "TerraformPlannedWithChanges", approveMessage, revision)
	return terraform
}

func TerraformPlannedNoChanges(terraform Terraform, revision string, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    "Plan",
		Status:  metav1.ConditionFalse,
		Reason:  "TerraformPlannedNoChanges",
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	(&terraform).Status.Plan = PlanStatus{
		LastApplied:   terraform.Status.Plan.LastApplied,
		Pending:       "",
		IsDestroyPlan: terraform.Spec.Destroy,
	}
	if revision != "" {
		(&terraform).Status.LastAttemptedRevision = revision
		(&terraform).Status.LastPlannedRevision = revision
	}

	SetTerraformReadiness(&terraform, metav1.ConditionTrue, "TerraformPlannedNoChanges", message+": "+revision, revision)
	return terraform
}

// TerraformProgressing resets the conditions of the given Terraform to a single
// ReadyCondition with status ConditionUnknown.
func TerraformProgressing(terraform Terraform, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    meta.ReadyCondition,
		Status:  metav1.ConditionUnknown,
		Reason:  meta.ProgressingReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	return terraform
}

// TerraformNotReady registers a failed apply attempt of the given Terraform.
func TerraformNotReady(terraform Terraform, revision, reason, message string) Terraform {
	SetTerraformReadiness(&terraform, metav1.ConditionFalse, reason, trimString(message, MaxConditionMessageLength), revision)
	if revision != "" {
		terraform.Status.LastAttemptedRevision = revision
	}
	return terraform
}

func TerraformAppliedFailResetPlanAndNotReady(terraform Terraform, revision, reason, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    "Apply",
		Status:  metav1.ConditionFalse,
		Reason:  "TerraformAppliedFail",
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	terraform = TerraformNotReady(terraform, revision, reason, message)
	terraform.Status.Plan.Pending = ""
	return terraform
}

func TerraformDriftDetected(terraform Terraform, revision, reason, message string) Terraform {
	(&terraform).Status.LastDriftDetectedAt = &metav1.Time{Time: time.Now()}
	SetTerraformReadiness(&terraform, metav1.ConditionFalse, reason, trimString(message, MaxConditionMessageLength), revision)
	return terraform
}

func TerraformNoDrift(terraform Terraform, revision, reason, message string) Terraform {
	SetTerraformReadiness(&terraform, metav1.ConditionTrue, reason, message+": "+revision, revision)
	return terraform
}

func TerraformHealthCheckFailed(terraform Terraform, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    "HealthCheck",
		Status:  metav1.ConditionFalse,
		Reason:  HealthChecksFailedReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	return terraform
}

func TerraformHealthCheckSucceeded(terraform Terraform, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    "HealthCheck",
		Status:  metav1.ConditionTrue,
		Reason:  "HealthChecksSucceed",
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	return terraform
}

// HasDrift returns true if drift has been detected since the last successful apply
func (in Terraform) HasDrift() bool {
	for _, condition := range in.Status.Conditions {
		if condition.Type == "Apply" &&
			condition.Status == metav1.ConditionTrue &&
			in.Status.LastDriftDetectedAt != nil &&
			(*in.Status.LastDriftDetectedAt).After(condition.LastTransitionTime.Time) {
			return true
		}
	}
	return false
}

// GetRetryInterval returns the retry interval
func (in Terraform) GetRetryInterval() time.Duration {
	if in.Spec.RetryInterval != nil {
		return in.Spec.RetryInterval.Duration
	}
	return in.Spec.Interval.Duration
}

// GetStatusConditions returns a pointer to the Status.Conditions slice.
func (in *Terraform) GetStatusConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

func (in Terraform) ToBytes(scheme *runtime.Scheme) ([]byte, error) {
	return runtime.Encode(
		serializer.NewCodecFactory(scheme).LegacyCodec(
			corev1.SchemeGroupVersion,
			GroupVersion,
			sourcev1.GroupVersion,
		), &in)
}

func (in *Terraform) FromBytes(b []byte, scheme *runtime.Scheme) error {
	return runtime.DecodeInto(
		serializer.NewCodecFactory(scheme).LegacyCodec(
			corev1.SchemeGroupVersion,
			GroupVersion,
			sourcev1.GroupVersion,
		), b, in)
}

func (in *Terraform) GetRunnerHostname(ip string) string {
	prefix := strings.ReplaceAll(ip, ".", "-")
	return fmt.Sprintf("%s.%s.pod.cluster.local", prefix, in.Namespace)
}

func (in *TerraformSpec) GetAlwaysCleanupRunnerPod() bool {
	if in.AlwaysCleanupRunnerPod == nil {
		return true
	}

	return *in.AlwaysCleanupRunnerPod
}

func trimString(str string, limit int) string {
	if len(str) <= limit {
		return str
	}

	return str[0:limit] + "..."
}

func init() {
	SchemeBuilder.Register(&Terraform{}, &TerraformList{})
}
