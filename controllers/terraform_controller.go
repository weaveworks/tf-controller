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

package controllers

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/events"
	"github.com/fluxcd/pkg/runtime/metrics"
	"github.com/fluxcd/pkg/runtime/predicates"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-exec/tfexec"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/mtls"
	"github.com/weaveworks/tf-controller/runner"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	kuberecorder "k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// TerraformReconciler reconciles a Terraform object
type TerraformReconciler struct {
	client.Client
	httpClient      *retryablehttp.Client
	EventRecorder   kuberecorder.EventRecorder
	MetricsRecorder *metrics.Recorder
	StatusPoller    *polling.StatusPoller
	Scheme          *runtime.Scheme
	CertRotator     *mtls.CertRotator
	RunnerGRPCPort  int
}

//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms/finalizers,verbs=get;create;update;patch;delete
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=buckets;gitrepositories,verbs=get;list;watch
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=buckets/status;gitrepositories/status,verbs=get
//+kubebuilder:rbac:groups="",resources=configmaps;secrets;serviceaccounts,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Terraform object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *TerraformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (retResult ctrl.Result, retErr error) {

	<-r.CertRotator.Ready

	isCAValid, err := r.CertRotator.IsCAValid()
	if isCAValid == false && r.CertRotator.TriggerCARotation != nil {
		readyCh := make(chan *mtls.TriggerResult)
		r.CertRotator.TriggerCARotation <- mtls.Trigger{Namespace: "", Ready: readyCh}
		<-readyCh
	}

	log := ctrl.LoggerFrom(ctx)
	reconcileStart := time.Now()

	var terraform infrav1.Terraform
	if err := r.Get(ctx, req.NamespacedName, &terraform); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Record suspended status metric
	defer r.recordSuspensionMetric(ctx, terraform)

	// Add our finalizer if it does not exist
	if !controllerutil.ContainsFinalizer(&terraform, infrav1.TerraformFinalizer) {
		controllerutil.AddFinalizer(&terraform, infrav1.TerraformFinalizer)
		if err := r.Update(ctx, &terraform); err != nil {
			log.Error(err, "unable to register finalizer")
			return ctrl.Result{}, err
		}
	}

	// TODO create Runner Pod
	// TODO wait for the Runner Pod to start
	runnerClient, closeConn, err := r.LookupOrCreateRunner(ctx, terraform)
	if err != nil {
		log.Error(err, "unable to lookup or create runner")
		if closeConn != nil {
			if err := closeConn(); err != nil {
				log.Error(err, "unable to close connection")
			}
		}
		return ctrl.Result{}, err
	}
	log.Info("runner is running")

	defer func(ctx context.Context, cli client.Client, terraform infrav1.Terraform) {
		if closeConn != nil {
			if err := closeConn(); err != nil {
				log.Error(err, "unable to close connection")
				retErr = err
			}
		}

		if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
			// nothing to delete
			log.Info("insecure local runner")
			return
		}

		if terraform.Spec.GetAlwaysCleanupRunnerPod() == true {
			podKey := getRunnerPodObjectKey(terraform)
			var pod corev1.Pod
			if err := cli.Get(ctx, podKey, &pod); err != nil {
				if !apierrors.IsNotFound(err) {
					log.Error(err, "unable to get pod for cleaning up")
					retErr = err
				}
				return
			}

			if err := cli.Delete(ctx, &pod); err != nil {
				log.Error(err, "unable to delete pod")
				retErr = err
				return
			}

			// wait for runner pod complete termination
			var (
				interval = time.Second * 5
				timeout  = time.Second * 120
			)
			err := wait.PollImmediate(interval, timeout, func() (bool, error) {
				var runnerPod corev1.Pod
				err := r.Get(ctx, getRunnerPodObjectKey(terraform), &runnerPod)
				if err == nil {
					return false, nil
				}

				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			})
			if err != nil {
				retErr = fmt.Errorf("failed waiting for the terminating runner pod: %v", err)
				log.Error(retErr, "error in polling")
			}
		}
	}(ctx, r.Client, terraform)

	// resolve source reference
	log.Info("getting source")
	sourceObj, err := r.getSource(ctx, terraform)

	// Examine if the object is under deletion
	if !terraform.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, terraform, runnerClient, sourceObj)
	}

	// Return early if the Terraform is suspended.
	if terraform.Spec.Suspend {
		log.Info("Reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("Source '%s' not found", terraform.Spec.SourceRef.String())
			terraform = infrav1.TerraformNotReady(terraform, "", infrav1.ArtifactFailedReason, msg)
			if err := r.patchStatus(ctx, req.NamespacedName, terraform.Status); err != nil {
				log.Error(err, "unable to update status for source not found")
				return ctrl.Result{Requeue: true}, err
			}
			r.recordReadinessMetric(ctx, terraform)
			log.Info(msg)
			// do not requeue immediately, when the source is created the watcher should trigger a reconciliation
			return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
		} else {
			// retry on transient errors
			log.Error(err, "retry")
			return ctrl.Result{Requeue: true}, err
		}
	}

	if sourceObj.GetArtifact() == nil {
		msg := "Source is not ready, artifact not found"
		terraform = infrav1.TerraformNotReady(terraform, "", infrav1.ArtifactFailedReason, msg)
		if err := r.patchStatus(ctx, req.NamespacedName, terraform.Status); err != nil {
			log.Error(err, "unable to update status for artifact not found")
			return ctrl.Result{Requeue: true}, err
		}
		r.recordReadinessMetric(ctx, terraform)
		log.Info(msg)
		// do not requeue immediately, when the artifact is created the watcher should trigger a reconciliation
		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	}

	// If revision is changed, and there's no intend to apply,
	// we should clear the Pending Plan to trigger re-plan
	if sourceObj.GetArtifact().Revision != terraform.Status.LastAttemptedRevision && !r.shouldApply(terraform) {
		terraform.Status.Plan.Pending = ""
		if err := r.Status().Update(ctx, &terraform); err != nil {
			log.Error(err, "unable to update status to clear pending plan (revision != last attempted)")
			return ctrl.Result{Requeue: true}, err
		}
	}

	// Return early if it's manually mode and pending
	if terraform.Status.Plan.Pending != "" && !r.forceOrAutoApply(terraform) && !r.shouldApply(terraform) {
		log.Info("reconciliation is stopped to wait for a manual approve")
		return ctrl.Result{}, nil
	}

	// reconcile Terraform by applying the latest revision
	reconciledTerraform, reconcileErr := r.reconcile(ctx, runnerClient, *terraform.DeepCopy(), sourceObj)
	if err := r.patchStatus(ctx, req.NamespacedName, reconciledTerraform.Status); err != nil {
		log.Error(err, "unable to update status after reconciliation")
		return ctrl.Result{Requeue: true}, err
	}
	r.recordReadinessMetric(ctx, reconciledTerraform)

	if reconcileErr != nil && reconcileErr.Error() == infrav1.DriftDetectedReason {
		log.Error(reconcileErr, fmt.Sprintf("Drift detected after %s, next try in %s",
			time.Since(reconcileStart).String(),
			terraform.GetRetryInterval().String()),
			"revision",
			sourceObj.GetArtifact().Revision)
		r.fireEvent(ctx, reconciledTerraform, sourceObj.GetArtifact().Revision, events.EventSeverityError, reconcileErr.Error(), nil)
		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	} else if reconcileErr != nil {
		// broadcast the reconciliation failure and requeue at the specified retry interval
		log.Error(reconcileErr, fmt.Sprintf("Reconciliation failed after %s, next try in %s",
			time.Since(reconcileStart).String(),
			terraform.GetRetryInterval().String()),
			"revision",
			sourceObj.GetArtifact().Revision)
		r.fireEvent(ctx, reconciledTerraform, sourceObj.GetArtifact().Revision, events.EventSeverityError, reconcileErr.Error(), nil)
		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	}

	if reconciledTerraform.Status.Plan.Pending != "" && !r.forceOrAutoApply(reconciledTerraform) {
		log.Info("Reconciliation is stopped to wait for a manual approve")
		return ctrl.Result{}, nil
	}

	// next reconcile is .Spec.Interval in the future
	log.Info("requeue")
	return ctrl.Result{RequeueAfter: terraform.Spec.Interval.Duration}, nil
}

func getRunnerPodObjectKey(terraform infrav1.Terraform) types.NamespacedName {
	return types.NamespacedName{Namespace: terraform.Namespace, Name: fmt.Sprintf("%s-tf-runner", terraform.Name)}
}

func (r *TerraformReconciler) shouldDetectDrift(terraform infrav1.Terraform, revision string) bool {
	// Please do not optimize this logic, as we'd like others to easily understand the logics behind this behaviour.

	// return false when drift detection is disabled
	if terraform.Spec.DisableDriftDetection == true {
		return false
	}

	// not support when Destroy == true
	if terraform.Spec.Destroy == true {
		return false
	}

	// new object
	if terraform.Status.LastAppliedRevision == "" &&
		terraform.Status.LastPlannedRevision == "" &&
		terraform.Status.LastAttemptedRevision == "" {
		return false
	}

	// thing worked normally, no change pending
	// then, we do drift detection
	if terraform.Status.LastAttemptedRevision == terraform.Status.LastAppliedRevision &&
		terraform.Status.LastAttemptedRevision == terraform.Status.LastPlannedRevision &&
		terraform.Status.LastAttemptedRevision == revision &&
		terraform.Status.Plan.Pending == "" {
		return true
	}

	// last time source changed with non-TF file, so we planned but no changes
	// this time, it needs drift detection
	if terraform.Status.LastAttemptedRevision == terraform.Status.LastPlannedRevision &&
		terraform.Status.LastAttemptedRevision == revision &&
		terraform.Status.Plan.Pending == "" {
		return true
	}

	return false
}

func (r *TerraformReconciler) forceOrAutoApply(terraform infrav1.Terraform) bool {
	return terraform.Spec.Force || terraform.Spec.ApprovePlan == infrav1.ApprovePlanAutoValue
}

func (r *TerraformReconciler) shouldPlan(terraform infrav1.Terraform) bool {
	// Please do not optimize this logic, as we'd like others to easily understand the logics behind this behaviour.
	if terraform.Spec.Force {
		return true
	}

	if terraform.Status.Plan.Pending == "" {
		return true
	} else if terraform.Status.Plan.Pending != "" {
		return false
	}
	return false
}

func (r *TerraformReconciler) shouldApply(terraform infrav1.Terraform) bool {
	// Please do not optimize this logic, as we'd like others to easily understand the logics behind this behaviour.
	if terraform.Spec.Force {
		return true
	}

	if terraform.Spec.ApprovePlan == "" {
		return false
	} else if terraform.Spec.ApprovePlan == infrav1.ApprovePlanAutoValue && terraform.Status.Plan.Pending != "" {
		return true
	} else if terraform.Spec.ApprovePlan == terraform.Status.Plan.Pending {
		return true
	} else if strings.HasPrefix(terraform.Status.Plan.Pending, terraform.Spec.ApprovePlan) {
		return true
	}
	return false
}

func (r *TerraformReconciler) shouldWriteOutputs(terraform infrav1.Terraform, outputs map[string]tfexec.OutputMeta) bool {
	if terraform.Spec.WriteOutputsToSecret != nil && len(outputs) > 0 {
		return true
	}

	return false
}

func (r *TerraformReconciler) shouldDoHealthChecks(terraform infrav1.Terraform) bool {
	if terraform.Spec.HealthChecks == nil || len(terraform.Spec.HealthChecks) < 1 {
		return false
	}

	var applyCondition metav1.Condition
	var hcCondition metav1.Condition
	for _, c := range terraform.Status.Conditions {
		if c.Type == "Apply" {
			applyCondition = c
		} else if c.Type == "HealthCheck" {
			hcCondition = c
		}
	}

	// health checks were previously performed but failed
	// do health check again
	if hcCondition.Reason == infrav1.HealthChecksFailedReason {
		return true
	}

	// terraform was applied and no health check performed yet
	// do health check
	if applyCondition.Reason == infrav1.TFExecApplySucceedReason &&
		hcCondition.Reason == "" {
		return true
	}

	return false
}

func (r *TerraformReconciler) reconcile(ctx context.Context, runnerClient runner.RunnerClient, terraform infrav1.Terraform, sourceObj sourcev1.Source) (retTerraform infrav1.Terraform, retErr error) {
	log := ctrl.LoggerFrom(ctx)
	revision := sourceObj.GetArtifact().Revision
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	var (
		tfInstance string
		tmpDir     string
		err        error
	)
	log.Info("setting up terraform")
	terraform, tfInstance, tmpDir, err = r.setupTerraform(ctx, runnerClient, terraform, sourceObj, revision, objectKey)

	defer func() {
		cleanupDirReply, err := runnerClient.CleanupDir(ctx, &runner.CleanupDirRequest{TmpDir: tmpDir})
		if err != nil {
			log.Error(err, "clean up error")
			retErr = err
			return
		}

		if cleanupDirReply != nil {
			log.Info(fmt.Sprintf("clean up dir: %s", cleanupDirReply.Message))
		}
	}()

	if err != nil {
		log.Error(err, "error in terraform setup")
		return terraform, err
	}

	if r.shouldDetectDrift(terraform, revision) {
		var driftDetectionErr error // declared here to avoid shadowing on terraform variable
		terraform, driftDetectionErr = r.detectDrift(ctx, terraform, tfInstance, runnerClient, revision)

		// immediately return if no drift - reconciliation will retry normally
		if driftDetectionErr == nil {
			// reconcile outputs only when outputs are missing
			if outputsDrifted, err := r.outputsMayBeDrifted(ctx, terraform); outputsDrifted == true && err == nil {
				terraform, err = r.processOutputs(ctx, runnerClient, terraform, tfInstance, revision)
				if err != nil {
					log.Error(err, "error processing outputs")
					return terraform, err
				}
			} else if err != nil {
				log.Error(err, "error checking for output drift")
				return terraform, err
			}

			return terraform, nil
		}

		// immediately return if err is not about drift
		if driftDetectionErr.Error() != infrav1.DriftDetectedReason {
			log.Error(driftDetectionErr, "detected non drift error")
			return terraform, driftDetectionErr
		}

		// immediately return if drift is detected but it's not "force" or "auto"
		if driftDetectionErr.Error() == infrav1.DriftDetectedReason && !r.forceOrAutoApply(terraform) {
			log.Error(driftDetectionErr, "will not force / auto apply detected drift")
			return terraform, driftDetectionErr
		}

		// ok, patch and continue
		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after drift detection")
			return terraform, err
		}
	}

	// return early if we're in drift-detection-only mode
	if terraform.Spec.ApprovePlan == infrav1.ApprovePlanDisableValue {
		log.Info("approve plan disabled")
		return terraform, nil
	}

	if r.shouldPlan(terraform) {
		terraform, err = r.plan(ctx, terraform, tfInstance, runnerClient, revision)
		if err != nil {
			log.Error(err, "error planning")
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after planing")
			return terraform, err
		}
	}

	if r.shouldApply(terraform) {
		terraform, err = r.apply(ctx, terraform, tfInstance, runnerClient, revision)
		if err != nil {
			log.Error(err, "error applying")
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after applying")
			return terraform, err
		}
	} else {
		log.Info("should apply == false")
	}

	terraform, err = r.processOutputs(ctx, runnerClient, terraform, tfInstance, revision)
	if err != nil {
		log.Error(err, "error process outputs")
		return terraform, err
	}

	if r.shouldDoHealthChecks(terraform) {
		terraform, err = r.doHealthChecks(ctx, terraform, runnerClient)
		if err != nil {
			log.Error(err, "error with health check")
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after doing health checks")
			return terraform, err
		}
	}

	return terraform, nil
}

func (r *TerraformReconciler) processOutputs(ctx context.Context, runnerClient runner.RunnerClient, terraform infrav1.Terraform, tfInstance string, revision string) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	outputs := map[string]tfexec.OutputMeta{}
	var err error
	terraform, err = r.obtainOutputs(ctx, terraform, tfInstance, runnerClient, revision, &outputs)
	if err != nil {
		return terraform, err
	}

	if r.shouldWriteOutputs(terraform, outputs) {
		terraform, err = r.writeOutput(ctx, terraform, runnerClient, outputs, revision)
		if err != nil {
			return terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after writing outputs")
			return terraform, err
		}
	}

	return terraform, nil
}

func (r *TerraformReconciler) obtainOutputs(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string, outputs *map[string]tfexec.OutputMeta) (infrav1.Terraform, error) {
	outputReply, err := runnerClient.Output(ctx, &runner.OutputRequest{
		TfInstance: tfInstance,
	})
	if err != nil {
		// TODO should not be this Error
		// warning-like status is enough
		err = fmt.Errorf("error running Output: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecOutputFailedReason,
			err.Error(),
		), err
	}
	*outputs = convertOutputs(outputReply.Outputs)

	var availableOutputs []string
	for k := range *outputs {
		availableOutputs = append(availableOutputs, k)
	}
	if len(availableOutputs) > 0 {
		sort.Strings(availableOutputs)
		terraform = infrav1.TerraformOutputsAvailable(terraform, availableOutputs, "Outputs available")
	}

	return terraform, nil
}

func (r *TerraformReconciler) setupTerraform(ctx context.Context, runnerClient runner.RunnerClient, terraform infrav1.Terraform, sourceObj sourcev1.Source, revision string, objectKey types.NamespacedName) (infrav1.Terraform, string, string, error) {
	log := ctrl.LoggerFrom(ctx)

	tfInstance := "0"
	tmpDir := ""

	terraform = infrav1.TerraformProgressing(terraform, "Initializing")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform initialization")
		return terraform, tfInstance, tmpDir, err
	}

	// download artifact and extract files
	buf, err := r.downloadAsBytes(sourceObj.GetArtifact())
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.ArtifactFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}

	uploadAndExtractReply, err := runnerClient.UploadAndExtract(ctx, &runner.UploadAndExtractRequest{
		Namespace: terraform.Namespace,
		Name:      terraform.Name,
		TarGz:     buf.Bytes(),
		Path:      terraform.Spec.Path,
	})
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.ArtifactFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}
	workingDir := uploadAndExtractReply.WorkingDir
	tmpDir = uploadAndExtractReply.TmpDir

	var backendConfig string
	DisableTFK8SBackend := os.Getenv("DISABLE_TF_K8S_BACKEND") == "1"

	if terraform.Spec.BackendConfig != nil {
		backendConfig = fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix     = "%s"
    in_cluster_config = %v
    config_path       = "%s"
    namespace         = "%s"
  }
}
`,
			terraform.Spec.BackendConfig.SecretSuffix,
			terraform.Spec.BackendConfig.InClusterConfig,
			terraform.Spec.BackendConfig.ConfigPath,
			terraform.Namespace)
	} else if DisableTFK8SBackend && terraform.Spec.BackendConfig == nil {
		backendConfig = `
terraform {
	backend "local" { }
}`
	} else if terraform.Spec.BackendConfig == nil {
		// TODO must be tested in cluster only
		backendConfig = fmt.Sprintf(`
terraform {
  backend "kubernetes" {
    secret_suffix     = "%s"
    in_cluster_config = true
    namespace         = "%s"
  }
}
`, terraform.Name, terraform.Namespace)
	}

	if r.backendCompletelyDisable(terraform) {
		log.Info("BackendConfig is completely disabled")
	} else {
		writeBackendConfigReply, err := runnerClient.WriteBackendConfig(ctx,
			&runner.WriteBackendConfigRequest{
				DirPath:       workingDir,
				BackendConfig: []byte(backendConfig),
			})
		if err != nil {
			log.Error(err, "write backend config error")
			return terraform, tfInstance, tmpDir, err
		}
		log.Info(fmt.Sprintf("write backend config: %s", writeBackendConfigReply.Message))
	}

	var tfrcFilepath string
	if terraform.Spec.CliConfigSecretRef != nil {
		cliConfigSecretRef := *(terraform.Spec.CliConfigSecretRef.DeepCopy())
		if cliConfigSecretRef.Namespace == "" {
			cliConfigSecretRef.Namespace = terraform.Namespace
		}

		processCliConfigReply, err := runnerClient.ProcessCliConfig(ctx, &runner.ProcessCliConfigRequest{
			DirPath:   workingDir,
			Namespace: cliConfigSecretRef.Namespace,
			Name:      cliConfigSecretRef.Name,
		})
		if err != nil {
			err = fmt.Errorf("cannot process cli config: %s", err.Error())
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.TFExecNewFailedReason,
				err.Error(),
			), tfInstance, tmpDir, err
		}
		tfrcFilepath = processCliConfigReply.FilePath
	}

	lookPathReply, err := runnerClient.LookPath(ctx,
		&runner.LookPathRequest{
			File: "terraform",
		})
	if err != nil {
		err = fmt.Errorf("cannot find Terraform binary: %s in %s", err, os.Getenv("PATH"))
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecNewFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}
	execPath := lookPathReply.ExecPath

	newTerraformReply, err := runnerClient.NewTerraform(ctx,
		&runner.NewTerraformRequest{
			// TarGz:      tarGzBytes,
			WorkingDir: workingDir,
			ExecPath:   execPath,
		})
	if err != nil {
		err = fmt.Errorf("error running NewTerraform: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecNewFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}

	tfInstance = newTerraformReply.Id
	envs := map[string]string{}

	for _, env := range terraform.Spec.RunnerPodTemplate.Spec.Env {
		envs[env.Name] = env.Value
	}

	disableTestLogging := os.Getenv("DISABLE_TF_LOGS") == "1"
	if !disableTestLogging {
		envs["DISABLE_TF_LOGS"] = "1"
	}

	if tfrcFilepath != "" {
		envs["TF_CLI_CONFIG_FILE"] = tfrcFilepath
	}

	if setEnvReply, err := runnerClient.SetEnv(ctx,
		&runner.SetEnvRequest{
			TfInstance: tfInstance,
			Envs:       envs,
		}); err != nil {
		err = fmt.Errorf("error setting env for Terraform: %s %s", err, setEnvReply.Message)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecInitFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}

	log.Info("new terraform", "workingDir", workingDir)

	// TODO we currently use a fork version of TFExec to workaround the forceCopy bug
	// https://github.com/hashicorp/terraform-exec/issues/262
	/*
		initOpts := []tfexec.InitOption{tfexec.Upgrade(true), tfexec.ForceCopy(true)}
		if r.backendCompletelyDisable(terraform) {
			initOpts = append(initOpts, tfexec.ForceCopy(false))
		}
	*/

	initRequest := &runner.InitRequest{
		TfInstance: tfInstance,
		Upgrade:    true,
		ForceCopy:  true,
	}
	if r.backendCompletelyDisable(terraform) {
		initRequest.ForceCopy = false
	}

	initReply, err := runnerClient.Init(ctx, initRequest)
	if err != nil {
		err = fmt.Errorf("error running Init: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecInitFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}
	log.Info(fmt.Sprintf("init reply: %s", initReply.Message))

	log.Info("tfexec initialized terraform")

	terraformBytes, err := terraform.ToBytes(r.Scheme)
	if err != nil {
		// transient error?
		return terraform, tfInstance, tmpDir, err
	}
	generateVarsForTFReply, err := runnerClient.GenerateVarsForTF(ctx, &runner.GenerateVarsForTFRequest{
		Terraform:  terraformBytes,
		WorkingDir: workingDir,
	})
	if err != nil {
		// transient error?
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.VarsGenerationFailedReason,
			err.Error(),
		), tfInstance, tmpDir, err
	}
	log.Info(fmt.Sprintf("generate vars from tf: %s", generateVarsForTFReply.Message))

	log.Info("generated var files from spec")
	return terraform, tfInstance, tmpDir, nil
}

func (r *TerraformReconciler) detectDrift(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)

	log.Info("calling detectDrift ...")

	const (
		driftFilename = "tfdrift"
	)

	planRequest := &runner.PlanRequest{
		TfInstance: tfInstance,
		Out:        driftFilename,
		Refresh:    true,
	}
	if r.backendCompletelyDisable(terraform) {
		planRequest.Out = ""
		planRequest.Refresh = true
	}

	planReply, err := runnerClient.Plan(ctx, planRequest)
	if err != nil {
		err = fmt.Errorf("error running Plan: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.DriftDetectionFailedReason,
			err.Error(),
		), err
	}
	drifted := planReply.Drifted
	log.Info(fmt.Sprintf("plan for drift: %s found drift: %v", planReply.Message, planReply.Drifted))

	if drifted {
		var rawOutput string
		if r.backendCompletelyDisable(terraform) {
			rawOutput = "not available"
		} else {
			showPlanFileRawReply, err := runnerClient.ShowPlanFileRaw(ctx, &runner.ShowPlanFileRawRequest{
				TfInstance: tfInstance,
				Filename:   driftFilename,
			})
			if err != nil {
				return infrav1.TerraformNotReady(
					terraform,
					revision,
					infrav1.DriftDetectionFailedReason,
					err.Error(),
				), err
			}
			rawOutput = showPlanFileRawReply.RawOutput
			log.Info(fmt.Sprintf("show plan: %s", showPlanFileRawReply.RawOutput))
		}

		// If drift detected & we use the auto mode, then we continue
		terraform = infrav1.TerraformDriftDetected(terraform, revision, infrav1.DriftDetectedReason, rawOutput)
		return terraform, fmt.Errorf(infrav1.DriftDetectedReason)
	}

	terraform = infrav1.TerraformNoDrift(terraform, revision, infrav1.NoDriftReason, "No drift")
	return terraform, nil
}

func (r *TerraformReconciler) plan(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)

	log.Info("calling plan ...")

	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}
	terraform = infrav1.TerraformProgressing(terraform, "Terraform Planning")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform planning")
		return terraform, err
	}

	const tfplanFilename = "tfplan"

	planRequest := &runner.PlanRequest{
		TfInstance: tfInstance,
		Out:        tfplanFilename,
		Refresh:    true, // be careful, refresh requires to be true by default
	}

	if r.backendCompletelyDisable(terraform) {
		planRequest.Out = ""
	}

	// check if destroy is set to true or
	// the object is being deleted and DestroyResourcesOnDeletion is set to true
	if terraform.Spec.Destroy ||
		!terraform.ObjectMeta.DeletionTimestamp.IsZero() &&
			terraform.Spec.DestroyResourcesOnDeletion {
		log.Info("plan to destroy")
		planRequest.Destroy = true
	}

	planReply, err := runnerClient.Plan(ctx, planRequest)
	if err != nil {
		err = fmt.Errorf("error running Plan: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}
	drifted := planReply.Drifted
	log.Info(fmt.Sprintf("plan: %s, found drift: %v", planReply.Message, drifted))

	saveTFPlanReply, err := runnerClient.SaveTFPlan(ctx, &runner.SaveTFPlanRequest{
		TfInstance:               tfInstance,
		BackendCompletelyDisable: r.backendCompletelyDisable(terraform),
		Name:                     terraform.Name,
		Namespace:                terraform.Namespace,
		Uuid:                     string(terraform.GetUID()),
		Revision:                 revision,
	})
	if err != nil {
		err = fmt.Errorf("error saving plan secret: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}
	log.Info(fmt.Sprintf("save tfplan: %s", saveTFPlanReply.Message))

	if drifted {
		terraform = infrav1.TerraformPlannedWithChanges(terraform, revision, "Plan generated")
	} else {
		terraform = infrav1.TerraformPlannedNoChanges(terraform, revision, "Plan no changes")
	}

	return terraform, nil
}

func (r *TerraformReconciler) backendCompletelyDisable(terraform infrav1.Terraform) bool {
	return terraform.Spec.BackendConfig != nil && terraform.Spec.BackendConfig.Disable == true
}

func (r *TerraformReconciler) apply(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string) (infrav1.Terraform, error) {

	const (
		TFPlanName = "tfplan"
	)

	log := ctrl.LoggerFrom(ctx)
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	terraform = infrav1.TerraformProgressing(terraform, "Applying")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform applying")
		return terraform, err
	}

	loadTFPlanReply, err := runnerClient.LoadTFPlan(ctx, &runner.LoadTFPlanRequest{
		TfInstance:               tfInstance,
		Name:                     terraform.Name,
		Namespace:                terraform.Namespace,
		BackendCompletelyDisable: r.backendCompletelyDisable(terraform),
		PendingPlan:              terraform.Status.Plan.Pending,
	})
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	log.Info(fmt.Sprintf("load tf plan: %s", loadTFPlanReply.Message))

	terraform = infrav1.TerraformApplying(terraform, revision, "Apply started")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "error recording apply status: %s", err)
		return terraform, err
	}

	applyRequest := &runner.ApplyRequest{
		TfInstance:         tfInstance,
		RefreshBeforeApply: terraform.Spec.RefreshBeforeApply,
	}
	if r.backendCompletelyDisable(terraform) {
		// do nothing
	} else {
		applyRequest.DirOrPlan = TFPlanName
	}

	var isDestroyApplied bool

	var inventoryEntries []infrav1.ResourceRef

	// this a special case, when backend is completely disabled.
	// we need to use "destroy" command instead of apply
	if r.backendCompletelyDisable(terraform) && terraform.Spec.Destroy == true {
		destroyReply, err := runnerClient.Destroy(ctx, &runner.DestroyRequest{
			TfInstance: tfInstance,
		})
		log.Info(fmt.Sprintf("destroy: %s", destroyReply.Message))
		if err != nil {
			err = fmt.Errorf("error running Destroy: %s", err)
			return infrav1.TerraformAppliedFailResetPlanAndNotReady(
				terraform,
				revision,
				infrav1.TFExecApplyFailedReason,
				err.Error(),
			), err
		}
		isDestroyApplied = true
	} else {
		applyReply, err := runnerClient.Apply(ctx, applyRequest)
		if err != nil {
			err = fmt.Errorf("error running Apply: %s", err)
			return infrav1.TerraformAppliedFailResetPlanAndNotReady(
				terraform,
				revision,
				infrav1.TFExecApplyFailedReason,
				err.Error(),
			), err
		}
		log.Info(fmt.Sprintf("apply: %s", applyReply.Message))

		isDestroyApplied = terraform.Status.Plan.IsDestroyPlan

		if terraform.Spec.EnableInventory {
			getInventoryRequest := &runner.GetInventoryRequest{TfInstance: tfInstance}
			getInventoryReply, err := runnerClient.GetInventory(ctx, getInventoryRequest)
			if err != nil {
				err = fmt.Errorf("error getting inventory after Apply: %s", err)
				return infrav1.TerraformAppliedFailResetPlanAndNotReady(
					terraform,
					revision,
					infrav1.TFExecApplyFailedReason,
					err.Error(),
				), err
			}
			for _, iv := range getInventoryReply.Inventories {
				inventoryEntries = append(inventoryEntries, infrav1.ResourceRef{
					Name:       iv.GetName(),
					Type:       iv.GetType(),
					Identifier: iv.GetIdentifier(),
				})
			}
			log.Info(fmt.Sprintf("got inventory - entries count: %d", len(inventoryEntries)))
		} else {
			log.Info("inventory is disabled by default")
		}
	}

	terraform = infrav1.TerraformApplied(terraform, revision, "Applied successfully", isDestroyApplied, inventoryEntries)

	return terraform, nil
}

func convertOutputs(outputs map[string]*runner.OutputMeta) map[string]tfexec.OutputMeta {
	result := map[string]tfexec.OutputMeta{}
	for k, v := range outputs {
		result[k] = tfexec.OutputMeta{
			Sensitive: v.Sensitive,
			Type:      v.Type,
			Value:     v.Value,
		}
	}
	return result
}

func (r *TerraformReconciler) writeOutput(ctx context.Context, terraform infrav1.Terraform, runnerClient runner.RunnerClient, outputs map[string]tfexec.OutputMeta, revision string) (infrav1.Terraform, error) {
	log := ctrl.LoggerFrom(ctx)

	wots := terraform.Spec.WriteOutputsToSecret
	data := map[string][]byte{}

	// if not specified .spec.writeOutputsToSecret.outputs,
	// then it means export all outputs
	if len(wots.Outputs) == 0 {
		for output, v := range outputs {
			ct, err := ctyjson.UnmarshalType(v.Type)
			if err != nil {
				return terraform, err
			}
			// if it's a string, we can embed it directly into Secret's data
			switch ct {
			case cty.String:
				cv, err := ctyjson.Unmarshal(v.Value, ct)
				if err != nil {
					return terraform, err
				}
				data[output] = []byte(cv.AsString())
			// there's no need to unmarshal and convert to []byte
			// we'll just pass the []byte directly from OutputMeta Value
			case cty.Number, cty.Bool:
				data[output] = v.Value
			default:
				outputBytes, err := json.Marshal(v.Value)
				if err != nil {
					return terraform, err
				}
				data[output] = outputBytes
			}
		}
	} else {
		// filter only defined output
		// output maybe contain mapping output:mapped_name
		for _, outputMapping := range wots.Outputs {
			parts := strings.SplitN(outputMapping, ":", 2)
			var output string
			var mappedTo string
			if len(parts) == 1 {
				output = parts[0]
				mappedTo = parts[0]
				// no mapping
			} else if len(parts) == 2 {
				output = parts[0]
				mappedTo = parts[1]
			} else {
				log.Error(fmt.Errorf("invalid mapping format"), outputMapping)
				continue
			}

			v := outputs[output]
			ct, err := ctyjson.UnmarshalType(v.Type)
			if err != nil {
				return terraform, err
			}
			switch ct {
			case cty.String:
				cv, err := ctyjson.Unmarshal(v.Value, ct)
				if err != nil {
					return terraform, err
				}
				data[mappedTo] = []byte(cv.AsString())
			// there's no need to unmarshal and convert to []byte
			// we'll just pass the []byte directly from OutputMeta Value
			case cty.Number, cty.Bool:
				data[mappedTo] = v.Value
			default:
				outputBytes, err := json.Marshal(v.Value)
				if err != nil {
					return terraform, err
				}
				data[mappedTo] = outputBytes
			}
		}
	}

	if len(data) == 0 || terraform.Spec.Destroy == true {
		return infrav1.TerraformOutputsWritten(terraform, revision, "No Outputs written"), nil
	}

	writeOutputsReply, err := runnerClient.WriteOutputs(ctx, &runner.WriteOutputsRequest{
		Namespace:  terraform.Namespace,
		Name:       terraform.Name,
		SecretName: terraform.Spec.WriteOutputsToSecret.Name,
		Uuid:       string(terraform.UID),
		Data:       data,
	})
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.OutputsWritingFailedReason,
			err.Error(),
		), err
	}
	log.Info(fmt.Sprintf("write outputs: %s", writeOutputsReply.Message))

	return infrav1.TerraformOutputsWritten(terraform, revision, "Outputs written"), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TerraformReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconciles int, httpRetry int) error {
	const (
		gitRepositoryIndexKey string = ".metadata.gitRepository"
		bucketIndexKey        string = ".metadata.bucket"
	)

	// Index the Terraforms by the GitRepository references they (may) point at.
	if err := mgr.GetCache().IndexField(context.TODO(), &infrav1.Terraform{}, gitRepositoryIndexKey,
		r.IndexBy(sourcev1.GitRepositoryKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Index the Terraforms by the Bucket references they (may) point at.
	if err := mgr.GetCache().IndexField(context.TODO(), &infrav1.Terraform{}, bucketIndexKey,
		r.IndexBy(sourcev1.BucketKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Configure the retryable http client used for fetching artifacts.
	// By default it retries 10 times within a 3.5 minutes window.
	httpClient := retryablehttp.NewClient()
	httpClient.RetryWaitMin = 5 * time.Second
	httpClient.RetryWaitMax = 30 * time.Second
	httpClient.RetryMax = httpRetry
	httpClient.Logger = nil
	r.httpClient = httpClient

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.Terraform{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}),
		)).
		Watches(
			&source.Kind{Type: &sourcev1.GitRepository{}},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(infrav1.GitRepositoryIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		Watches(
			&source.Kind{Type: &sourcev1.Bucket{}},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(infrav1.BucketIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrentReconciles}).
		Complete(r)
}

func (r *TerraformReconciler) requestsForRevisionChangeOf(indexKey string) func(obj client.Object) []reconcile.Request {
	return func(obj client.Object) []reconcile.Request {
		repo, ok := obj.(interface {
			GetArtifact() *sourcev1.Artifact
		})
		if !ok {
			panic(fmt.Sprintf("Expected an object conformed with GetArtifact() method, but got a %T", obj))
		}
		// If we do not have an artifact, we have no requests to make
		if repo.GetArtifact() == nil {
			return nil
		}

		ctx := context.Background()
		var list infrav1.TerraformList
		if err := r.List(ctx, &list, client.MatchingFields{
			indexKey: client.ObjectKeyFromObject(obj).String(),
		}); err != nil {
			return nil
		}
		reqs := make([]reconcile.Request, len(list.Items))
		for i, t := range list.Items {
			// If the revision of the artifact equals to the last attempted revision,
			// we should not make a request for this Terraform
			if repo.GetArtifact().Revision == t.Status.LastAttemptedRevision {
				continue
			}
			reqs[i].NamespacedName.Name = t.Name
			reqs[i].NamespacedName.Namespace = t.Namespace
		}
		return reqs
	}

}

func (r *TerraformReconciler) getSource(ctx context.Context, terraform infrav1.Terraform) (sourcev1.Source, error) {
	var sourceObj sourcev1.Source
	sourceNamespace := terraform.GetNamespace()
	if terraform.Spec.SourceRef.Namespace != "" {
		sourceNamespace = terraform.Spec.SourceRef.Namespace
	}
	namespacedName := types.NamespacedName{
		Namespace: sourceNamespace,
		Name:      terraform.Spec.SourceRef.Name,
	}
	switch terraform.Spec.SourceRef.Kind {
	case sourcev1.GitRepositoryKind:
		var repository sourcev1.GitRepository
		err := r.Client.Get(ctx, namespacedName, &repository)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return sourceObj, err
			}
			return sourceObj, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}
		sourceObj = &repository
	case sourcev1.BucketKind:
		var bucket sourcev1.Bucket
		err := r.Client.Get(ctx, namespacedName, &bucket)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return sourceObj, err
			}
			return sourceObj, fmt.Errorf("unable to get source '%s': %w", namespacedName, err)
		}
		sourceObj = &bucket
	default:
		return sourceObj, fmt.Errorf("source `%s` kind '%s' not supported",
			terraform.Spec.SourceRef.Name, terraform.Spec.SourceRef.Kind)
	}
	return sourceObj, nil
}

func (r *TerraformReconciler) downloadAsBytes(artifact *sourcev1.Artifact) (*bytes.Buffer, error) {
	artifactURL := artifact.URL
	if hostname := os.Getenv("SOURCE_CONTROLLER_LOCALHOST"); hostname != "" {
		u, err := url.Parse(artifactURL)
		if err != nil {
			return nil, err
		}
		u.Host = hostname
		artifactURL = u.String()
	}

	req, err := retryablehttp.NewRequest(http.MethodGet, artifactURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download artifact, error: %w", err)
	}
	defer resp.Body.Close()

	// check response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download artifact from %s, status: %s", artifactURL, resp.Status)
	}

	var buf bytes.Buffer

	// verify checksum matches origin
	if err := r.verifyArtifact(artifact, &buf, resp.Body); err != nil {
		return nil, err
	}

	return &buf, nil
}

func (r *TerraformReconciler) recordReadinessMetric(ctx context.Context, terraform infrav1.Terraform) {
	if r.MetricsRecorder == nil {
		return
	}
	log := ctrl.LoggerFrom(ctx)

	objRef, err := reference.GetReference(r.Scheme, &terraform)
	if err != nil {
		log.Error(err, "unable to record readiness metric")
		return
	}
	if rc := apimeta.FindStatusCondition(terraform.Status.Conditions, meta.ReadyCondition); rc != nil {
		r.MetricsRecorder.RecordCondition(*objRef, *rc,
			!terraform.DeletionTimestamp.IsZero())
	} else {
		r.MetricsRecorder.RecordCondition(*objRef, metav1.Condition{
			Type:   meta.ReadyCondition,
			Status: metav1.ConditionUnknown,
		}, !terraform.DeletionTimestamp.IsZero())
	}
}

func (r *TerraformReconciler) recordSuspensionMetric(ctx context.Context, terraform infrav1.Terraform) {
	if r.MetricsRecorder == nil {
		return
	}
	log := ctrl.LoggerFrom(ctx)

	objRef, err := reference.GetReference(r.Scheme, &terraform)
	if err != nil {
		log.Error(err, "unable to record suspended metric")
		return
	}

	if !terraform.DeletionTimestamp.IsZero() {
		r.MetricsRecorder.RecordSuspend(*objRef, false)
	} else {
		r.MetricsRecorder.RecordSuspend(*objRef, terraform.Spec.Suspend)
	}
}

func (r *TerraformReconciler) patchStatus(ctx context.Context, objectKey types.NamespacedName, newStatus infrav1.TerraformStatus) error {
	var terraform infrav1.Terraform
	if err := r.Get(ctx, objectKey, &terraform); err != nil {
		return err
	}

	patch := client.MergeFrom(terraform.DeepCopy())
	terraform.Status = newStatus

	return r.Status().Patch(ctx, &terraform, patch)
}

func (r *TerraformReconciler) verifyArtifact(artifact *sourcev1.Artifact, buf *bytes.Buffer, reader io.Reader) error {
	hasher := sha256.New()

	// for backwards compatibility with source-controller v0.17.2 and older
	if len(artifact.Checksum) == 40 {
		hasher = sha1.New()
	}

	// compute checksum
	mw := io.MultiWriter(hasher, buf)
	if _, err := io.Copy(mw, reader); err != nil {
		return err
	}

	if checksum := fmt.Sprintf("%x", hasher.Sum(nil)); checksum != artifact.Checksum {
		return fmt.Errorf("failed to verify artifact: computed checksum '%s' doesn't match advertised '%s'",
			checksum, artifact.Checksum)
	}

	return nil
}

func (r *TerraformReconciler) IndexBy(kind string) func(o client.Object) []string {
	return func(o client.Object) []string {
		terraform, ok := o.(*infrav1.Terraform)
		if !ok {
			panic(fmt.Sprintf("Expected a Kustomization, got %T", o))
		}

		if terraform.Spec.SourceRef.Kind == kind {
			namespace := terraform.GetNamespace()
			if terraform.Spec.SourceRef.Namespace != "" {
				namespace = terraform.Spec.SourceRef.Namespace
			}
			return []string{fmt.Sprintf("%s/%s", namespace, terraform.Spec.SourceRef.Name)}
		}

		return nil
	}
}

func (r *TerraformReconciler) fireEvent(ctx context.Context, terraform infrav1.Terraform, revision, severity, msg string, metadata map[string]string) {
	if metadata == nil {
		metadata = map[string]string{}
	}
	if revision != "" {
		metadata[infrav1.GroupVersion.Group+"/revision"] = revision
	}

	reason := severity
	if c := apimeta.FindStatusCondition(terraform.Status.Conditions, meta.ReadyCondition); c != nil {
		reason = c.Reason
	}

	eventtype := "Normal"
	if severity == events.EventSeverityError {
		eventtype = "Warning"
	}

	r.EventRecorder.AnnotatedEventf(&terraform, metadata, eventtype, reason, msg)
}

func (r *TerraformReconciler) finalize(ctx context.Context, terraform infrav1.Terraform, runnerClient runner.RunnerClient, sourceObj sourcev1.Source) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	if terraform.Spec.DestroyResourcesOnDeletion {
		revision := sourceObj.GetArtifact().Revision
		terraform, tfInstance, tmpDir, err := r.setupTerraform(ctx, runnerClient, terraform, sourceObj, revision, objectKey)

		defer func() {
			cleanupDirReply, err := runnerClient.CleanupDir(ctx, &runner.CleanupDirRequest{TmpDir: tmpDir})
			if err != nil {
				log.Error(err, "clean up error")
			}
			if cleanupDirReply != nil {
				log.Info(fmt.Sprintf("clean up dir: %s", cleanupDirReply.Message))
			}
		}()

		if err != nil {
			return ctrl.Result{Requeue: true}, err
		}

		terraform, err = r.plan(ctx, terraform, tfInstance, runnerClient, revision)
		if err != nil {
			return ctrl.Result{Requeue: true}, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after planing")
			return ctrl.Result{Requeue: true}, err
		}

		terraform, err = r.apply(ctx, terraform, tfInstance, runnerClient, revision)
		if err != nil {
			return ctrl.Result{Requeue: true}, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after applying")
			return ctrl.Result{Requeue: true}, err
		}

		if err == nil {
			log.Info("finalizing destroyResourcesOnDeletion: ok")
		}
	}

	outputSecretName := ""
	hasSpecifiedOutputSecret := terraform.Spec.WriteOutputsToSecret != nil && terraform.Spec.WriteOutputsToSecret.Name != ""
	if hasSpecifiedOutputSecret {
		outputSecretName = terraform.Spec.WriteOutputsToSecret.Name
	}

	finalizeSecretsReply, err := runnerClient.FinalizeSecrets(ctx, &runner.FinalizeSecretsRequest{
		Namespace:                terraform.Namespace,
		Name:                     terraform.Name,
		HasSpecifiedOutputSecret: hasSpecifiedOutputSecret,
		OutputSecretName:         outputSecretName,
	})
	if err != nil {
		if e, ok := status.FromError(err); ok {
			switch e.Code() {
			case codes.Internal:
				// transient error
				return ctrl.Result{Requeue: true}, err
			case codes.NotFound:
				// do nothing, fall through
			}
		}
	}

	if err == nil {
		log.Info(fmt.Sprintf("finalizing secrets: %s", finalizeSecretsReply.Message))
	}

	// Record deleted status
	r.recordReadinessMetric(ctx, terraform)

	if err := r.Get(ctx, objectKey, &terraform); err != nil {
		return ctrl.Result{}, err
	}

	// Remove our finalizer from the list and update it
	controllerutil.RemoveFinalizer(&terraform, infrav1.TerraformFinalizer)
	if err := r.Update(ctx, &terraform); err != nil {
		return ctrl.Result{}, err
	}

	// Stop reconciliation as the object is being deleted
	return ctrl.Result{}, nil
}

func (r *TerraformReconciler) LookupOrCreateRunner(ctx context.Context, terraform infrav1.Terraform) (runner.RunnerClient, func() error, error) {
	return r.lookupOrCreateRunner_000(ctx, terraform)
}

//lookupOrCreateRunner_000
func (r *TerraformReconciler) lookupOrCreateRunner_000(ctx context.Context, terraform infrav1.Terraform) (runner.RunnerClient, func() error, error) {
	// we have to make sure that the secret is valid before we can create the runner.
	err := r.reconcileRunnerSecret(ctx, &terraform)
	if err != nil {
		return nil, nil, err
	}

	var hostname string
	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		hostname = "localhost"
	} else {
		podIP, err := r.reconcileRunnerPod(ctx, terraform)
		if err != nil {
			return nil, nil, err
		}
		hostname = terraform.GetRunnerHostname(podIP)
	}

	conn, err := r.getRunnerConnection(terraform, hostname, r.RunnerGRPCPort)
	if err != nil {
		return nil, nil, err
	}
	connClose := func() error { return conn.Close() }
	runnerClient := runner.NewRunnerClient(conn)
	return runnerClient, connClose, nil
}

func (r *TerraformReconciler) getRunnerConnection(terraform infrav1.Terraform, hostname string, port int) (*grpc.ClientConn, error) {
	// blocks until the tls cert is valid
	trigger := mtls.Trigger{
		Namespace: terraform.Namespace,
		Ready:     make(chan *mtls.TriggerResult),
	}

	r.CertRotator.TriggerNamespaceTLSGeneration <- trigger
	result := <-trigger.Ready

	tlsSecret := result.Secret
	err := result.Err
	if err != nil {
		return nil, fmt.Errorf("failed to get tls secret: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", hostname, port)
	credentials, err := mtls.GetGRPCClientCredentials(tlsSecret)
	if err != nil {
		return nil, err
	}

	const retryPolicy = `{
		"methodConfig": [{
		  "name": [{"service": "runner.Runner"}],
		  "waitForReady": true,
		  "retryPolicy": {
			  "MaxAttempts": 4,
			  "InitialBackoff": ".01s",
			  "MaxBackoff": ".01s",
			  "BackoffMultiplier": 1.0,
			  "RetryableStatusCodes": [ "UNAVAILABLE" ]
		  }
		}]}`

	return grpc.Dial(addr, grpc.WithTransportCredentials(credentials), grpc.WithDefaultServiceConfig(retryPolicy))
}

func (r *TerraformReconciler) doHealthChecks(ctx context.Context, terraform infrav1.Terraform, runnerClient runner.RunnerClient) (infrav1.Terraform, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("calling doHealthChecks ...")

	// get terraform output data for health check urls
	outputs := make(map[string]string)
	if terraform.Spec.WriteOutputsToSecret != nil && terraform.Spec.WriteOutputsToSecret.Name != "" {
		getOutputsReply, err := runnerClient.GetOutputs(ctx, &runner.GetOutputsRequest{
			Namespace:  terraform.Namespace,
			SecretName: terraform.Spec.WriteOutputsToSecret.Name,
		})
		if err != nil {
			err = fmt.Errorf("error getting terraform output for health checks: %s", err)
			return infrav1.TerraformHealthCheckFailed(
				terraform,
				err.Error(),
			), err
		}
		outputs = getOutputsReply.Outputs
	}

	for _, hc := range terraform.Spec.HealthChecks {
		// perform health check based on type
		switch hc.Type {
		case infrav1.HealthCheckTypeTCP:
			parsed, err := r.templateParse(outputs, hc.Address)
			if err != nil {
				err = fmt.Errorf("error getting terraform output for health checks: %s", err)
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}

			err = r.doTCPHealthCheck(ctx, hc.Name, parsed, hc.GetTimeout())
			if err != nil {
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}
		case infrav1.HealthCheckTypeHttpGet:
			parsed, err := r.templateParse(outputs, hc.URL)
			if err != nil {
				err = fmt.Errorf("error getting terraform output for health checks: %s", err)
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}

			err = r.doHTTPHealthCheck(ctx, hc.Name, parsed, hc.GetTimeout())
			if err != nil {
				return infrav1.TerraformHealthCheckFailed(
					terraform,
					err.Error(),
				), err
			}
		}
	}
	terraform = infrav1.TerraformHealthCheckSucceeded(terraform, "Health checks succeeded")
	return terraform, nil
}

func (r *TerraformReconciler) doTCPHealthCheck(ctx context.Context, name string, address string, timeout time.Duration) error {
	log := ctrl.LoggerFrom(ctx)

	// validate tcp address
	_, err := url.ParseRequestURI(address)
	if err != nil {
		return fmt.Errorf("invalid url for http health check: %s, %s", address, err)
	}

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return fmt.Errorf("failed to perform tcp health check for %s on %s: %s", name, address, err.Error())
	}

	err = conn.Close()
	if err != nil {
		log.Error(err, "Unexpected error closing TCP health check socket")
	}

	return nil
}

func (r *TerraformReconciler) doHTTPHealthCheck(ctx context.Context, name string, urlString string, timeout time.Duration) error {
	log := ctrl.LoggerFrom(ctx)

	// validate url
	_, err := url.ParseRequestURI(urlString)
	if err != nil {
		return fmt.Errorf("invalid url for http health check: %s, %s", urlString, err)
	}

	req, err := http.NewRequest("GET", urlString, nil)
	if err != nil {
		log.Error(err, "Unexpected error creating HTTP request")
		return err
	}

	ctxt, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()

	re, err := http.DefaultClient.Do(req.WithContext(ctxt))
	if err != nil {
		return fmt.Errorf("failed to perform http health check for %s on %s: %s", name, urlString, err.Error())
	}
	defer func() {
		if rerr := re.Body.Close(); rerr != nil {
			log.Error(err, "Unexpected error closing HTTP health check socket")
		}
	}()

	// read http body
	b, err := io.ReadAll(re.Body)
	if err != nil {
		return fmt.Errorf("failed to perform http health check for %s on %s, error reading body: %s", name, urlString, err.Error())
	}

	// check http status code
	if re.StatusCode >= http.StatusOK && re.StatusCode < http.StatusBadRequest {
		log.Info("HTTP health check succeeded for %s on %s, response: %v", name, urlString, *re)
		return nil
	}

	err = fmt.Errorf("failed to perform http health check for %s on %s, response body: %v", name, urlString, string(b))
	log.Error(err, "failed to perform http health check for %s on %s, response body: %v", name, urlString, string(b))
	return err
}

// parse template string from map[string]string
func (r *TerraformReconciler) templateParse(content map[string]string, text string) (string, error) {
	var b bytes.Buffer
	tmpl, err := template.New("tmpl").Parse(text)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(&b, content)
	if err != nil {
		err = fmt.Errorf("error getting terraform output for health checks: %s", err)
		return "", err
	}
	return b.String(), nil
}

// reconcileRunnerSecret reconciles the runner secret used for mTLS
//
// It should create the secret if it doesn't exist and then verify that the cert is valid
// if the cert is not present in the secret or is invalid, it will generate a new cert and
// write it to the secret. One secret per namespace is created in order to sidestep the need
// for specifying a pod ip in the certificate SAN field.
func (r *TerraformReconciler) reconcileRunnerSecret(ctx context.Context, terraform *infrav1.Terraform) error {
	log := ctrl.LoggerFrom(ctx)

	log.Info("trigger namespace tls secret generation")

	trigger := mtls.Trigger{
		Namespace: terraform.Namespace,
		Ready:     make(chan *mtls.TriggerResult),
	}
	r.CertRotator.TriggerNamespaceTLSGeneration <- trigger

	result := <-trigger.Ready
	if result.Err != nil {
		return errors.Wrap(result.Err, "failed to get tls generation result")
	}

	return nil
}

func (r *TerraformReconciler) reconcileRunnerPod(ctx context.Context, terraform infrav1.Terraform) (string, error) {
	runnerPodTemplate := runnerPodTemplate(terraform)

	runnerPod := *runnerPodTemplate.DeepCopy()
	runnerPodKey := client.ObjectKeyFromObject(&runnerPod)

	var err error
	err = r.Get(ctx, runnerPodKey, &runnerPod)
	if err != nil && apierrors.IsNotFound(err) == false {
		return "", err
	}

	const (
		interval = time.Second * 3
		timeout  = time.Second * 60
	)

	// if the old pod has been terminating, wait for it to be deleted.
	if err == nil && runnerPod.DeletionTimestamp != nil {
		podErr := wait.PollImmediate(interval, timeout, func() (bool, error) {
			err = r.Get(ctx, runnerPodKey, &runnerPod)

			if err != nil && apierrors.IsNotFound(err) == false {
				return false, err
			}

			if err != nil && apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, nil
		})
		if podErr != nil {
			return "", fmt.Errorf("failed to wait for the old pod termination: %v", podErr)
		}
	}

	if err != nil && apierrors.IsNotFound(err) {
		newRunnerPod := *runnerPodTemplate.DeepCopy()
		newRunnerPod.Spec = r.runnerPodSpec(terraform)
		if err := r.Create(ctx, &newRunnerPod); err != nil {
			return "", err
		}
	}

	if wait.PollImmediate(interval, timeout, func() (bool, error) {
		if err := r.Get(ctx, runnerPodKey, &runnerPod); err != nil {
			return false, fmt.Errorf("failed to get runner pod: %w", err)
		}
		if runnerPod.Status.PodIP != "" {
			return true, nil
		}
		return false, nil
	}) != nil {
		return "", fmt.Errorf("failed to create and obtain pod ip")
	}

	return runnerPod.Status.PodIP, nil
}

func getRunnerPodImage(image string) string {
	runnerPodImage := image
	if runnerPodImage == "" {
		runnerPodImage = os.Getenv("RUNNER_POD_IMAGE")
	}
	if runnerPodImage == "" {
		runnerPodImage = "ghcr.io/weaveworks/tf-runner:latest"
	}
	return runnerPodImage
}

func runnerPodTemplate(terraform infrav1.Terraform) corev1.Pod {
	podNamespace := terraform.Namespace
	podName := fmt.Sprintf("%s-tf-runner", terraform.Name)
	runnerPodTemplate := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: podNamespace,
			Name:      podName,
			Labels: map[string]string{
				"app.kubernetes.io/created-by": "tf-controller",
				"app.kubernetes.io/name":       "tf-runner",
				"app.kubernetes.io/instance":   podName,
				infrav1.RunnerLabel:            terraform.Namespace,
			},
			Annotations: terraform.Spec.RunnerPodTemplate.Metadata.Annotations,
		},
	}

	// add runner pod custom labels
	if len(terraform.Spec.RunnerPodTemplate.Metadata.Labels) != 0 {
		for k, v := range terraform.Spec.RunnerPodTemplate.Metadata.Labels {
			runnerPodTemplate.Labels[k] = v
		}
	}
	return runnerPodTemplate
}

func (r *TerraformReconciler) runnerPodSpec(terraform infrav1.Terraform) corev1.PodSpec {
	serviceAccountName := terraform.Spec.ServiceAccountName
	if serviceAccountName == "" {
		serviceAccountName = "tf-runner"
	}

	gracefulTermPeriod := terraform.Spec.RunnerTerminationGracePeriodSeconds
	envvars := []corev1.EnvVar{}
	envvarsMap := map[string]corev1.EnvVar{
		"POD_NAME": {
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		"POD_NAMESPACE": {
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
	}

	for _, envName := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		if envValue := os.Getenv(envName); envValue != "" {
			envvarsMap[envName] = corev1.EnvVar{
				Name:  envName,
				Value: envValue,
			}
		}
	}

	for _, env := range terraform.Spec.RunnerPodTemplate.Spec.Env {
		envvarsMap[env.Name] = env
	}

	for _, env := range envvarsMap {
		envvars = append(envvars, env)
	}

	vFalse := false
	vTrue := true
	vUser := int64(65532)
	tlsSecretName, err := r.CertRotator.GetRunnerTLSSecretName()
	if err != nil {
		tlsSecretName = infrav1.RunnerTLSSecretName
	}

	return corev1.PodSpec{
		TerminationGracePeriodSeconds: gracefulTermPeriod,
		Containers: []corev1.Container{
			{
				Name: "tf-runner",
				Args: []string{
					"--grpc-port", fmt.Sprintf("%d", r.RunnerGRPCPort),
					"--tls-secret-name", tlsSecretName,
				},
				Image:           getRunnerPodImage(terraform.Spec.RunnerPodTemplate.Spec.Image),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Ports: []corev1.ContainerPort{
					{
						Name:          "grpc",
						ContainerPort: int32(r.RunnerGRPCPort),
					},
				},
				Env:     envvars,
				EnvFrom: terraform.Spec.RunnerPodTemplate.Spec.EnvFrom,
				// TODO: this security context might break OpenShift because of SCC. We need verification.
				// TODO how to support it via Spec or Helm Chart
				SecurityContext: &corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
					AllowPrivilegeEscalation: &vFalse,
					RunAsNonRoot:             &vTrue,
					RunAsUser:                &vUser,
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
					ReadOnlyRootFilesystem: &vTrue,
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "temp",
						MountPath: "/tmp",
					},
				},
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "temp",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
		ServiceAccountName: serviceAccountName,
	}
}

func (r *TerraformReconciler) outputsMayBeDrifted(ctx context.Context, terraform infrav1.Terraform) (bool, error) {
	if terraform.Spec.WriteOutputsToSecret != nil {
		outputsSecretKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Spec.WriteOutputsToSecret.Name}
		var outputsSecret corev1.Secret
		err := r.Client.Get(ctx, outputsSecretKey, &outputsSecret)
		if err != nil && apierrors.IsNotFound(err) {
			return true, nil
		}

		return false, err
	}

	return false, nil
}
