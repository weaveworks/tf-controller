package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/fluxcd/pkg/untar"
	"github.com/go-logr/logr"
	"github.com/hashicorp/terraform-exec/tfexec"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TFPlanName                = "tfplan"
	SavedPlanSecretAnnotation = "savedPlan"
)

type LocalPrintfer struct {
	logger logr.Logger
}

func (l LocalPrintfer) Printf(format string, v ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

type TerraformRunnerServer struct {
	UnimplementedRunnerServer
	tf *tfexec.Terraform
	client.Client
	Scheme *runtime.Scheme
	Done   chan os.Signal
}

const loggerName = "runner.terraform"

func (r *TerraformRunnerServer) LookPath(ctx context.Context, req *LookPathRequest) (*LookPathReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName).WithName(loggerName)
	log.Info("looking for path", "file", req.File)
	execPath, err := exec.LookPath(req.File)
	if err != nil {
		log.Error(err, "unable to look for path", "file", req.File)
		return nil, err
	}
	return &LookPathReply{ExecPath: execPath}, nil
}

func (r *TerraformRunnerServer) UploadAndExtract(ctx context.Context, req *UploadAndExtractRequest) (*UploadAndExtractReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("preparing for Upload and Extraction")
	tmpDir, err := securejoin.SecureJoin(os.TempDir(), fmt.Sprintf("%s-%s", req.Namespace, req.Name))
	if err != nil {
		log.Error(err, "unable to join securely", "tmpDir", os.TempDir(), "namespace", req.Namespace, "name", req.Name)
		return nil, err
	}

	buf := bytes.NewBuffer(req.TarGz)
	if _, err = untar.Untar(buf, tmpDir); err != nil {
		log.Error(err, "unable to extract tar file", "namespace", req.Namespace, "name", req.Name)
		return nil, fmt.Errorf("failed to untar artifact, error: %w", err)
	}

	dirPath, err := securejoin.SecureJoin(tmpDir, req.Path)
	if err != nil {
		log.Error(err, "unable to join securely", "tmpDir", tmpDir, "path", req.Path)
		return nil, err
	}

	if _, err := os.Stat(dirPath); err != nil {
		log.Error(err, "path not found", "dirPath", dirPath)
		err = fmt.Errorf("terraform path not found: %w", err)
		return nil, err
	}

	return &UploadAndExtractReply{WorkingDir: dirPath, TmpDir: tmpDir}, nil
}

func (r *TerraformRunnerServer) CleanupDir(ctx context.Context, req *CleanupDirRequest) (*CleanupDirReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("cleanup TmpDir", "tmpDir", req.TmpDir)
	err := os.RemoveAll(req.TmpDir)
	if err != nil {
		log.Error(err, "error cleaning up TmpDir", "tmpDir", req.TmpDir)
		return nil, err
	}

	return &CleanupDirReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) WriteBackendConfig(ctx context.Context, req *WriteBackendConfigRequest) (*WriteBackendConfigReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	const backendConfigPath = "generated_backend_config.tf"
	log.Info("write backend config", "path", req.DirPath, "config", backendConfigPath)
	filePath, err := securejoin.SecureJoin(req.DirPath, backendConfigPath)
	if err != nil {
		log.Error(err, "unable to join securely", "dirPath", req.DirPath, "config", backendConfigPath)
		return nil, err
	}

	log.Info("write config to file", "filePath", filePath)
	err = os.WriteFile(filePath, req.BackendConfig, 0644)
	if err != nil {
		log.Error(err, "unable to write file", "filePath", filePath)
		return nil, err
	}

	return &WriteBackendConfigReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) ProcessCliConfig(ctx context.Context, req *ProcessCliConfigRequest) (*ProcessCliConfigReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("processing configuration", "namespace", req.Namespace, "name", req.Name)
	cliConfigKey := types.NamespacedName{Namespace: req.Namespace, Name: req.Name}
	var cliConfig corev1.Secret
	if err := r.Get(ctx, cliConfigKey, &cliConfig); err != nil {
		log.Error(err, "unable to get secret", "namespace", req.Namespace, "name", req.Name)
		return nil, err
	}

	if len(cliConfig.Data) != 1 {
		err := fmt.Errorf("expect the secret to contain 1 data")
		log.Error(err, "secret missing data", "namespace", req.Namespace, "name", req.Name)
		return nil, err
	}

	var tfrcFilepath string
	for tfrcFilename, v := range cliConfig.Data {
		if strings.HasSuffix(tfrcFilename, ".tfrc") {
			var err error
			tfrcFilepath, err = securejoin.SecureJoin(req.DirPath, tfrcFilename)
			if err != nil {
				log.Error(err, "secure join error", "dirPath", req.DirPath, "tfrcFilename", tfrcFilename)
				return nil, err
			}
			err = os.WriteFile(tfrcFilepath, v, 0644)
			if err != nil {
				log.Error(err, "write file error", "tfrcFilename", tfrcFilename, "fileMode", "0644")
				return nil, err
			}
		} else {
			err := fmt.Errorf("expect the secret key to end with .tfrc")
			log.Error(err, "file extension error", "tfrcFilename", tfrcFilename)
			return nil, err
		}
	}
	return &ProcessCliConfigReply{FilePath: tfrcFilepath}, nil
}

func (r *TerraformRunnerServer) NewTerraform(ctx context.Context, req *NewTerraformRequest) (*NewTerraformReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("creating new terraform", "workingDir", req.WorkingDir, "execPath", req.ExecPath)
	tf, err := tfexec.NewTerraform(req.WorkingDir, req.ExecPath)
	if err != nil {
		log.Error(err, "unable to create new terraform", "workingDir", req.WorkingDir, "execPath", req.ExecPath)
		return nil, err
	}

	// hold only 1 instance
	r.tf = tf

	disableTestLogging := os.Getenv("DISABLE_TF_LOGS") == "1"
	if !disableTestLogging {
		r.tf.SetStdout(os.Stdout)
		r.tf.SetStderr(os.Stderr)
		if os.Getenv("ENABLE_SENSITIVE_TF_LOGS") == "1" {
			r.tf.SetLogger(&LocalPrintfer{logger: log})
		}
	}

	return &NewTerraformReply{Id: "1"}, nil
}

func (r *TerraformRunnerServer) SetEnv(ctx context.Context, req *SetEnvRequest) (*SetEnvReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("setting envvars")

	if req.TfInstance != "1" {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	log.Info("getting envvars from os environments")
	envs := utils.EnvMap(os.Environ())
	for k, v := range req.Envs {
		envs[k] = v
	}
	err := r.tf.SetEnv(envs)
	if err != nil {
		log.Error(err, "unable to set envvars", "envvars", envs)
		return nil, err
	}

	return &SetEnvReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Init(ctx context.Context, req *InitRequest) (*InitReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("initializing")
	if req.TfInstance != "1" {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	initOpts := []tfexec.InitOption{tfexec.Upgrade(req.Upgrade), tfexec.ForceCopy(req.ForceCopy)}
	err := r.tf.Init(ctx, initOpts...)
	if err != nil {
		log.Error(err, "unable to initialize")
		return nil, err
	}

	return &InitReply{Message: "ok"}, nil
}

// GenerateVarsForTF renders the Terraform variables as a json file for the given inputs
// variables supplied in the varsFrom field will override those specified in the spec
func (r *TerraformRunnerServer) GenerateVarsForTF(ctx context.Context, req *GenerateVarsForTFRequest) (*GenerateVarsForTFReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("setting up the input variables")
	var terraform infrav1.Terraform
	err := terraform.FromBytes(req.Terraform, r.Scheme)
	if err != nil {
		log.Error(err, "there was a problem getting the terraform resource")
		return nil, err
	}

	log.Info("mapping the Spec.Vars")
	vars := map[string]*apiextensionsv1.JSON{}
	if len(terraform.Spec.Vars) > 0 {
		for _, v := range terraform.Spec.Vars {
			vars[v.Name] = v.Value
		}
	}

	log.Info("mapping the Spec.VarsFrom")
	// varsFrom overwrite vars
	for _, vf := range terraform.Spec.VarsFrom {
		objectKey := types.NamespacedName{
			Namespace: terraform.Namespace,
			Name:      vf.Name,
		}
		if vf.Kind == "Secret" {
			var s corev1.Secret
			err := r.Get(ctx, objectKey, &s)
			if err != nil && vf.Optional == false {
				log.Error(err, "unable to get object key", "objectKey", objectKey, "secret", s.ObjectMeta.Name)
				return nil, err
			}
			// if VarsKeys is null, use all
			if vf.VarsKeys == nil {
				for key, val := range s.Data {
					vars[key], err = utils.JSONEncodeBytes(val)
					if err != nil {
						err := fmt.Errorf("failed to encode key %s with error: %w", key, err)
						log.Error(err, "encoding failure")
						return nil, err
					}
				}
			} else {
				for _, key := range vf.VarsKeys {
					vars[key], err = utils.JSONEncodeBytes(s.Data[key])
					if err != nil {
						err := fmt.Errorf("failed to encode key %s with error: %w", key, err)
						log.Error(err, "encoding failure")
						return nil, err
					}
				}
			}
		} else if vf.Kind == "ConfigMap" {
			var cm corev1.ConfigMap
			err := r.Get(ctx, objectKey, &cm)
			if err != nil && vf.Optional == false {
				log.Error(err, "unable to get object key", "objectKey", objectKey, "configmap", cm.ObjectMeta.Name)
				return nil, err
			}

			// if VarsKeys is null, use all
			if vf.VarsKeys == nil {
				for key, val := range cm.Data {
					vars[key], err = utils.JSONEncodeBytes([]byte(val))
					if err != nil {
						err := fmt.Errorf("failed to encode key %s with error: %w", key, err)
						log.Error(err, "encoding failure")
						return nil, err
					}
				}
				for key, val := range cm.BinaryData {
					vars[key], err = utils.JSONEncodeBytes(val)
					if err != nil {
						err := fmt.Errorf("failed to encode key %s with error: %w", key, err)
						log.Error(err, "encoding failure")
						return nil, err
					}
				}
			} else {
				for _, key := range vf.VarsKeys {
					if val, ok := cm.Data[key]; ok {
						vars[key], err = utils.JSONEncodeBytes([]byte(val))
						if err != nil {
							err := fmt.Errorf("failed to encode key %s with error: %w", key, err)
							log.Error(err, "encoding failure")
							return nil, err
						}
					}
					if val, ok := cm.BinaryData[key]; ok {
						vars[key], err = utils.JSONEncodeBytes(val)
						if err != nil {
							log.Error(err, "encoding failure")
							return nil, err
						}
					}
				}
			}
		}
	}

	jsonBytes, err := json.Marshal(vars)
	if err != nil {
		log.Error(err, "unable to marshal the data")
		return nil, err
	}

	varFilePath := filepath.Join(req.WorkingDir, "generated.auto.tfvars.json")
	if err := ioutil.WriteFile(varFilePath, jsonBytes, 0644); err != nil {
		err = fmt.Errorf("error generating var file: %s", err)
		log.Error(err, "unable to write the data to file", "filePath", varFilePath)
		return nil, err
	}

	return &GenerateVarsForTFReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Plan(ctx context.Context, req *PlanRequest) (*PlanReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("creating a plan")
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-r.Done:
			cancel()
		case <-ctx.Done():
		}
	}()

	if req.TfInstance != "1" {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	var planOpt []tfexec.PlanOption
	if req.Out != "" {
		planOpt = append(planOpt, tfexec.Out(req.Out))
	}

	if req.Refresh == false {
		planOpt = append(planOpt, tfexec.Refresh(req.Refresh))
	}

	if req.Destroy {
		planOpt = append(planOpt, tfexec.Destroy(req.Destroy))
	}

	drifted, err := r.tf.Plan(ctx, planOpt...)
	if err != nil {
		log.Error(err, "error creating the plan")
		return nil, err
	}
	return &PlanReply{
		Drifted: drifted,
		Message: "ok",
	}, nil
}

func (r *TerraformRunnerServer) ShowPlanFileRaw(ctx context.Context, req *ShowPlanFileRawRequest) (*ShowPlanFileRawReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("show the raw plan file")
	if req.TfInstance != "1" {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	rawOutput, err := r.tf.ShowPlanFileRaw(ctx, req.Filename)
	if err != nil {
		log.Error(err, "unable to get the raw plan output")
		return nil, err
	}

	return &ShowPlanFileRawReply{RawOutput: rawOutput}, nil
}

func (r *TerraformRunnerServer) SaveTFPlan(ctx context.Context, req *SaveTFPlanRequest) (*SaveTFPlanReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("save the plan")
	if req.TfInstance != "1" {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	var tfplan []byte
	if req.BackendCompletelyDisable {
		tfplan = []byte("dummy plan")
	} else {
		var err error
		tfplan, err = ioutil.ReadFile(filepath.Join(r.tf.WorkingDir(), TFPlanName))
		if err != nil {
			err = fmt.Errorf("error running Plan: %s", err)
			log.Error(err, "unable to run the plan")
			return nil, err
		}
	}

	tfplanObjectKey := types.NamespacedName{Name: "tfplan-default-" + req.Name, Namespace: req.Namespace}
	var tfplanSecret corev1.Secret
	tfplanSecretExists := true
	if err := r.Client.Get(ctx, tfplanObjectKey, &tfplanSecret); err != nil {
		if errors.IsNotFound(err) {
			tfplanSecretExists = false
		} else {
			err = fmt.Errorf("error getting tfplanSecret: %s", err)
			log.Error(err, "unable to get the plan secret")
			return nil, err
		}
	}

	if tfplanSecretExists {
		if err := r.Client.Delete(ctx, &tfplanSecret); err != nil {
			err = fmt.Errorf("error deleting tfplanSecret: %s", err)
			log.Error(err, "unable to delete the plan secret")
			return nil, err
		}
	}

	planRev := strings.Replace(req.Revision, "/", "-", 1)
	planName := "plan-" + planRev

	tfplan, err := utils.GzipEncode(tfplan)
	if err != nil {
		log.Error(err, "unable to encode the plan revision", "planName", planName, "planRev", planRev)
		return nil, err
	}

	tfplanData := map[string][]byte{TFPlanName: tfplan}
	tfplanSecret = corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tfplan-default-" + req.Name,
			Namespace: req.Namespace,
			Annotations: map[string]string{
				"encoding":                "gzip",
				SavedPlanSecretAnnotation: planName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: infrav1.GroupVersion.Group + "/" + infrav1.GroupVersion.Version,
					Kind:       infrav1.TerraformKind,
					Name:       req.Name,
					UID:        types.UID(req.Uuid),
				},
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: tfplanData,
	}

	if err := r.Client.Create(ctx, &tfplanSecret); err != nil {
		err = fmt.Errorf("error recording plan status: %s", err)
		log.Error(err, "unable to create plan secret")
		return nil, err
	}

	return &SaveTFPlanReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) LoadTFPlan(ctx context.Context, req *LoadTFPlanRequest) (*LoadTFPlanReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("loading plan from secret")
	if req.TfInstance != "1" {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	tfplanSecretKey := types.NamespacedName{Namespace: req.Namespace, Name: "tfplan-default-" + req.Name}
	tfplanSecret := corev1.Secret{}
	err := r.Get(ctx, tfplanSecretKey, &tfplanSecret)
	if err != nil {
		err = fmt.Errorf("error getting plan secret: %s", err)
		log.Error(err, "unable to get secret")
		return nil, err
	}

	if tfplanSecret.Annotations[SavedPlanSecretAnnotation] != req.PendingPlan {
		err = fmt.Errorf("error pending plan and plan's name in the secret are not matched: %s != %s",
			req.PendingPlan,
			tfplanSecret.Annotations[SavedPlanSecretAnnotation])
		log.Error(err, "plan name mismatch")
		return nil, err
	}

	if req.BackendCompletelyDisable {
		// do nothing
	} else {
		tfplan := tfplanSecret.Data[TFPlanName]
		tfplan, err = utils.GzipDecode(tfplan)
		if err != nil {
			log.Error(err, "unable to decode the plan")
			return nil, err
		}
		err = ioutil.WriteFile(filepath.Join(r.tf.WorkingDir(), TFPlanName), tfplan, 0644)
		if err != nil {
			err = fmt.Errorf("error saving plan file to disk: %s", err)
			log.Error(err, "unable to write the plan to disk")
			return nil, err
		}
	}

	return &LoadTFPlanReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Destroy(ctx context.Context, req *DestroyRequest) (*DestroyReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("running destroy")
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-r.Done:
			cancel()
		case <-ctx.Done():
		}
	}()

	if req.TfInstance != "1" {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	if err := r.tf.Destroy(ctx); err != nil {
		log.Error(err, "unable to destroy")
		return nil, err
	}

	return &DestroyReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) Apply(ctx context.Context, req *ApplyRequest) (*ApplyReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("running apply")
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-r.Done:
			cancel()
		case <-ctx.Done():
		}
	}()

	if req.TfInstance != "1" {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	var applyOpt []tfexec.ApplyOption
	if req.DirOrPlan != "" {
		applyOpt = []tfexec.ApplyOption{tfexec.DirOrPlan(req.DirOrPlan)}
	}

	if req.RefreshBeforeApply {
		applyOpt = []tfexec.ApplyOption{tfexec.Refresh(true)}
	}

	if err := r.tf.Apply(ctx, applyOpt...); err != nil {
		log.Error(err, "unable to apply plan")
		return nil, err
	}

	return &ApplyReply{Message: "ok"}, nil
}

func getInventoryFromTerraformModule(m *tfjson.StateModule) []*Inventory {
	var result []*Inventory
	for _, r := range m.Resources {
		var id string
		// TODO ARN is AWS-specific. Will need to support other cloud identifiers in the future.
		if val, ok := r.AttributeValues["arn"]; ok {
			id = val.(string)
		} else if val, ok := r.AttributeValues["id"]; ok {
			id = val.(string)
		}
		result = append(result, &Inventory{
			Name:       r.Name,
			Type:       r.Type,
			Identifier: id,
		})
	}

	// recursively get all resources from submodules
	for _, childModule := range m.ChildModules {
		childInventory := getInventoryFromTerraformModule(childModule)
		result = append(result, childInventory...)
	}

	return result
}

func (r *TerraformRunnerServer) GetInventory(ctx context.Context, req *GetInventoryRequest) (*GetInventoryReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("get inventory")
	if req.TfInstance != "1" {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "get inventory: no terraform")
		return nil, err
	}
	state, err := r.tf.Show(ctx)
	if err != nil {
		log.Error(err, "get inventory: unable to get state via show command")
		return nil, err
	}

	// state contains no values after resource destruction for example
	if state.Values == nil {
		log.Info("get inventory: state values is nil")
		return &GetInventoryReply{Inventories: []*Inventory{}}, nil
	}

	if state.Values.RootModule == nil {
		log.Info("get inventory: root module is nil")
		return &GetInventoryReply{Inventories: []*Inventory{}}, nil
	}

	return &GetInventoryReply{Inventories: getInventoryFromTerraformModule(state.Values.RootModule)}, nil
}

func (r *TerraformRunnerServer) Output(ctx context.Context, req *OutputRequest) (*OutputReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("creating outputs")
	if req.TfInstance != "1" {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	outputs, err := r.tf.Output(ctx)
	if err != nil {
		log.Error(err, "unable to get outputs")
		return nil, err
	}

	outputReply := &OutputReply{Outputs: map[string]*OutputMeta{}}
	for k, v := range outputs {
		outputReply.Outputs[k] = &OutputMeta{
			Sensitive: v.Sensitive,
			Type:      v.Type,
			Value:     v.Value,
		}
	}
	return outputReply, nil
}

func (r *TerraformRunnerServer) WriteOutputs(ctx context.Context, req *WriteOutputsRequest) (*WriteOutputsReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("write outputs to secret")

	objectKey := types.NamespacedName{Namespace: req.Namespace, Name: req.SecretName}
	var outputSecret corev1.Secret

	if err := r.Client.Get(ctx, objectKey, &outputSecret); err == nil {
		if err := r.Client.Delete(ctx, &outputSecret); err != nil {
			log.Error(err, "unable to delete secret")
			return nil, err
		}
	} else if apierrors.IsNotFound(err) == false {
		log.Error(err, "unable to get output secret")
		return nil, err
	}

	block := true
	outputSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.SecretName,
			Namespace: req.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         infrav1.GroupVersion.Version + "/" + infrav1.GroupVersion.Version,
					Kind:               infrav1.TerraformKind,
					Name:               req.Name,
					UID:                types.UID(req.Uuid),
					BlockOwnerDeletion: &block,
				},
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: req.Data,
	}

	err := r.Client.Create(ctx, &outputSecret)
	if err != nil {
		log.Error(err, "unable to create secret")
		return nil, err
	}

	return &WriteOutputsReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) GetOutputs(ctx context.Context, req *GetOutputsRequest) (*GetOutputsReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("get outputs")
	outputKey := types.NamespacedName{Namespace: req.Namespace, Name: req.SecretName}
	outputSecret := corev1.Secret{}
	err := r.Client.Get(ctx, outputKey, &outputSecret)
	if err != nil {
		err = fmt.Errorf("error getting terraform output for health checks: %s", err)
		log.Error(err, "unable to check terraform health")
		return nil, err
	}

	outputs := map[string]string{}
	// parse map[string][]byte to map[string]string for go template parsing
	if len(outputSecret.Data) > 0 {
		for k, v := range outputSecret.Data {
			outputs[k] = string(v)
		}
	}

	return &GetOutputsReply{Outputs: outputs}, nil
}

func (r *TerraformRunnerServer) FinalizeSecrets(ctx context.Context, req *FinalizeSecretsRequest) (*FinalizeSecretsReply, error) {
	log := ctrl.LoggerFrom(ctx).WithName(loggerName)
	log.Info("finalize the output secrets")
	planObjectKey := types.NamespacedName{Namespace: req.Namespace, Name: "tfplan-default-" + req.Name}
	var planSecret corev1.Secret
	if err := r.Client.Get(ctx, planObjectKey, &planSecret); err == nil {
		if err := r.Client.Delete(ctx, &planSecret); err != nil {
			// transient failure
			log.Error(err, "unable to delete the plan secret")
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if apierrors.IsNotFound(err) {
		log.Error(err, "plan secret not found")
		return nil, status.Error(codes.NotFound, err.Error())
	} else {
		// transient failure
		log.Error(err, "unable to get the plan secret")
		return nil, status.Error(codes.Internal, err.Error())
	}

	if req.HasSpecifiedOutputSecret {
		outputsObjectKey := types.NamespacedName{Namespace: req.Namespace, Name: req.OutputSecretName}
		var outputsSecret corev1.Secret
		if err := r.Client.Get(ctx, outputsObjectKey, &outputsSecret); err == nil {
			if err := r.Client.Delete(ctx, &outputsSecret); err != nil {
				// transient failure
				log.Error(err, "unable to delete the output secret")
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else if apierrors.IsNotFound(err) {
			log.Error(err, "output secret not found")
			return nil, status.Error(codes.NotFound, err.Error())
		} else {
			// transient failure
			log.Error(err, "unable to get the output secret")
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &FinalizeSecretsReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) ForceUnlock(ctx context.Context, req *ForceUnlockRequest) (*ForceUnlockReply, error) {
	r.tf.ForceUnlock(ctx, req.LockIdentifier)
	return nil, nil
}
