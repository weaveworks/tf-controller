<h1>Terraform API reference</h1>
<p>Packages:</p>
<ul class="simple">
<li>
<a href="#infra.contrib.fluxcd.io%2fv1alpha1">infra.contrib.fluxcd.io/v1alpha1</a>
</li>
</ul>
<h2 id="infra.contrib.fluxcd.io/v1alpha1">infra.contrib.fluxcd.io/v1alpha1</h2>
Resource Types:
<ul class="simple"></ul>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.BackendConfigSpec">BackendConfigSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformSpec">TerraformSpec</a>)
</p>
<p>BackendConfigSpec is for specifying configuration for Terraform&rsquo;s Kubernetes backend</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>disable</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Disable is to completely disable the backend configuration.</p>
</td>
</tr>
<tr>
<td>
<code>secretSuffix</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>inClusterConfig</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>configPath</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>labels</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>state</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.BackendConfigStateSpec">
BackendConfigStateSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.BackendConfigStateSpec">BackendConfigStateSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.BackendConfigSpec">BackendConfigSpec</a>)
</p>
<p>BackendConfigStateSpec allows the user to set ForceUnlock</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>forceUnlock</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ForceUnlock a Terraform state if it has become locked for any reason.</p>
<p>Leave this empty to do nothing, set this to the value of the <code>Lock Info: ID: [value]</code>,
e.g. <code>f2ab685b-f84d-ac0b-a125-378a22877e8d</code>, to force unlock the state and
finally set this to <code>auto</code> to have the state lock automatically unlocked.</p>
<p>WARNING: Only use <code>auto</code> in the cases where you are absolutely certain that
no other system is using this state, you could otherwise end up in a bad place
See <a href="https://www.terraform.io/language/state/locking#force-unlock">https://www.terraform.io/language/state/locking#force-unlock</a> for more
information on the terraform state lock and force unlock.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.CrossNamespaceSourceReference">CrossNamespaceSourceReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformSpec">TerraformSpec</a>)
</p>
<p>CrossNamespaceSourceReference contains enough information to let you locate the
typed Kubernetes resource object at cluster level.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>API version of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>kind</code><br>
<em>
string
</em>
</td>
<td>
<p>Kind of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name of the referent.</p>
</td>
</tr>
<tr>
<td>
<code>namespace</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Namespace of the referent, defaults to the namespace of the Kubernetes resource object that contains the reference.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.HealthCheck">HealthCheck
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformSpec">TerraformSpec</a>)
</p>
<p>HealthCheck contains configuration needed to perform a health check after
terraform is applied.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name of the health check.</p>
</td>
</tr>
<tr>
<td>
<code>type</code><br>
<em>
string
</em>
</td>
<td>
<p>Type of the health check, valid values are (&lsquo;tcp&rsquo;, &lsquo;http&rsquo;).
If tcp is specified, address is required.
If http is specified, url is required.</p>
</td>
</tr>
<tr>
<td>
<code>url</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>URL to perform http health check on. Required when http type is specified.
Go template can be used to reference values from the terraform output
(e.g. <a href="https://example.org">https://example.org</a>, {{.output_url}}).</p>
</td>
</tr>
<tr>
<td>
<code>address</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Address to perform tcp health check on. Required when tcp type is specified.
Go template can be used to reference values from the terraform output
(e.g. 127.0.0.1:8080, {{.address}}:{{.port}}).</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The timeout period at which the connection should timeout if unable to
complete the request.
When not specified, default 20s timeout is used.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.PlanStatus">PlanStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformStatus">TerraformStatus</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>lastApplied</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>pending</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>isDestroyPlan</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>isDriftDetectionPlan</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.ResourceInventory">ResourceInventory
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformStatus">TerraformStatus</a>)
</p>
<p>ResourceInventory contains a list of Kubernetes resource object references that have been applied by a Kustomization.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>entries</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.ResourceRef">
[]ResourceRef
</a>
</em>
</td>
<td>
<p>Entries of Kubernetes resource object references.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.ResourceRef">ResourceRef
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.ResourceInventory">ResourceInventory</a>)
</p>
<p>ResourceRef contains the information necessary to locate a resource within a cluster.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>n</code><br>
<em>
string
</em>
</td>
<td>
<p>Terraform resource&rsquo;s name.</p>
</td>
</tr>
<tr>
<td>
<code>t</code><br>
<em>
string
</em>
</td>
<td>
<p>Type is Terraform resource&rsquo;s type</p>
</td>
</tr>
<tr>
<td>
<code>id</code><br>
<em>
string
</em>
</td>
<td>
<p>ID is the resource identifier. This is cloud-specific. For example, ARN is an ID on AWS.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.RunnerPodMetadata">RunnerPodMetadata
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.RunnerPodTemplate">RunnerPodTemplate</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>labels</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Labels to add to the runner pod</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Annotations to add to the runner pod</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.RunnerPodSpec">RunnerPodSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.RunnerPodTemplate">RunnerPodTemplate</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>image</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Runner pod image to use other than default</p>
</td>
</tr>
<tr>
<td>
<code>envFrom</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#envfromsource-v1-core">
[]Kubernetes core/v1.EnvFromSource
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of sources to populate environment variables in the container.
The keys defined within a source must be a C_IDENTIFIER. All invalid keys
will be reported as an event when the container is starting. When a key exists in multiple
sources, the value associated with the last source will take precedence.
Values defined by an Env with a duplicate key will take precedence.
Cannot be updated.</p>
</td>
</tr>
<tr>
<td>
<code>env</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#envvar-v1-core">
[]Kubernetes core/v1.EnvVar
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of environment variables to set in the container.
Cannot be updated.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.RunnerPodTemplate">RunnerPodTemplate
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformSpec">TerraformSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.RunnerPodMetadata">
RunnerPodMetadata
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>spec</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.RunnerPodSpec">
RunnerPodSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<br/>
<br/>
<table>
<tr>
<td>
<code>image</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Runner pod image to use other than default</p>
</td>
</tr>
<tr>
<td>
<code>envFrom</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#envfromsource-v1-core">
[]Kubernetes core/v1.EnvFromSource
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of sources to populate environment variables in the container.
The keys defined within a source must be a C_IDENTIFIER. All invalid keys
will be reported as an event when the container is starting. When a key exists in multiple
sources, the value associated with the last source will take precedence.
Values defined by an Env with a duplicate key will take precedence.
Cannot be updated.</p>
</td>
</tr>
<tr>
<td>
<code>env</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#envvar-v1-core">
[]Kubernetes core/v1.EnvVar
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of environment variables to set in the container.
Cannot be updated.</p>
</td>
</tr>
</table>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.Terraform">Terraform
</h3>
<p>Terraform is the Schema for the terraforms API</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformSpec">
TerraformSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>approvePlan</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ApprovePlan specifies name of a plan wanted to approve.
If its value is &ldquo;auto&rdquo;, the controller will automatically approve every plan.</p>
</td>
</tr>
<tr>
<td>
<code>destroy</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Destroy produces a destroy plan. Applying the plan will destroy all resources.</p>
</td>
</tr>
<tr>
<td>
<code>backendConfig</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.BackendConfigSpec">
BackendConfigSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>vars</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.Variable">
[]Variable
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of input variables to set for the Terraform program.</p>
</td>
</tr>
<tr>
<td>
<code>varsFrom</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.VarsReference">
[]VarsReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of references to a Secret or a ConfigMap to generate variables for
Terraform resources based on its data, selectively by varsKey. Values of the later
Secret / ConfigMap with the samek keys will override those of the former.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<p>The interval at which to reconcile the Terraform.</p>
</td>
</tr>
<tr>
<td>
<code>retryInterval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The interval at which to retry a previously failed reconciliation.
When not specified, the controller uses the TerraformSpec.Interval
value to retry failures.</p>
</td>
</tr>
<tr>
<td>
<code>path</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Path to the directory containing Terraform (.tf) files.
Defaults to &lsquo;None&rsquo;, which translates to the root path of the SourceRef.</p>
</td>
</tr>
<tr>
<td>
<code>sourceRef</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.CrossNamespaceSourceReference">
CrossNamespaceSourceReference
</a>
</em>
</td>
<td>
<p>SourceRef is the reference of the source where the Terraform files are stored.</p>
</td>
</tr>
<tr>
<td>
<code>suspend</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Suspend is to tell the controller to suspend subsequent TF executions,
it does not apply to already started executions. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>force</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Force instructs the controller to unconditionally
re-plan and re-apply TF resources. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>writeOutputsToSecret</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.WriteOutputsToSecretSpec">
WriteOutputsToSecretSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A list of target secrets for the outputs to be written as.</p>
</td>
</tr>
<tr>
<td>
<code>disableDriftDetection</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Disable automatic drift detection. Drift detection may be resource intensive in
the context of a large cluster or complex Terraform statefile. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>cliConfigSecretRef</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#secretreference-v1-core">
Kubernetes core/v1.SecretReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>healthChecks</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.HealthCheck">
[]HealthCheck
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of health checks to be performed.</p>
</td>
</tr>
<tr>
<td>
<code>destroyResourcesOnDeletion</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Create destroy plan and apply it to destroy terraform resources
upon deletion of this object. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>serviceAccountName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Name of a ServiceAccount for the runner Pod to provision Terraform resources.
Default to tf-runner.</p>
</td>
</tr>
<tr>
<td>
<code>alwaysCleanupRunnerPod</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Clean the runner pod up after each reconciliation cycle</p>
</td>
</tr>
<tr>
<td>
<code>runnerTerminationGracePeriodSeconds</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Configure the termination grace period for the runner pod. Use this parameter
to allow the Terraform process to gracefully shutdown. Consider increasing for
large, complex or slow-moving Terraform managed resources.</p>
</td>
</tr>
<tr>
<td>
<code>refreshBeforeApply</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>RefreshBeforeApply forces refreshing of the state before the apply step.</p>
</td>
</tr>
<tr>
<td>
<code>runnerPodTemplate</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.RunnerPodTemplate">
RunnerPodTemplate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>enableInventory</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnableInventory enables the object to store resource entries as the inventory for external use.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformStatus">
TerraformStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.TerraformSpec">TerraformSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.Terraform">Terraform</a>)
</p>
<p>TerraformSpec defines the desired state of Terraform</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>approvePlan</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ApprovePlan specifies name of a plan wanted to approve.
If its value is &ldquo;auto&rdquo;, the controller will automatically approve every plan.</p>
</td>
</tr>
<tr>
<td>
<code>destroy</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Destroy produces a destroy plan. Applying the plan will destroy all resources.</p>
</td>
</tr>
<tr>
<td>
<code>backendConfig</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.BackendConfigSpec">
BackendConfigSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>vars</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.Variable">
[]Variable
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of input variables to set for the Terraform program.</p>
</td>
</tr>
<tr>
<td>
<code>varsFrom</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.VarsReference">
[]VarsReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of references to a Secret or a ConfigMap to generate variables for
Terraform resources based on its data, selectively by varsKey. Values of the later
Secret / ConfigMap with the samek keys will override those of the former.</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<p>The interval at which to reconcile the Terraform.</p>
</td>
</tr>
<tr>
<td>
<code>retryInterval</code><br>
<em>
<a href="https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The interval at which to retry a previously failed reconciliation.
When not specified, the controller uses the TerraformSpec.Interval
value to retry failures.</p>
</td>
</tr>
<tr>
<td>
<code>path</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Path to the directory containing Terraform (.tf) files.
Defaults to &lsquo;None&rsquo;, which translates to the root path of the SourceRef.</p>
</td>
</tr>
<tr>
<td>
<code>sourceRef</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.CrossNamespaceSourceReference">
CrossNamespaceSourceReference
</a>
</em>
</td>
<td>
<p>SourceRef is the reference of the source where the Terraform files are stored.</p>
</td>
</tr>
<tr>
<td>
<code>suspend</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Suspend is to tell the controller to suspend subsequent TF executions,
it does not apply to already started executions. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>force</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Force instructs the controller to unconditionally
re-plan and re-apply TF resources. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>writeOutputsToSecret</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.WriteOutputsToSecretSpec">
WriteOutputsToSecretSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A list of target secrets for the outputs to be written as.</p>
</td>
</tr>
<tr>
<td>
<code>disableDriftDetection</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Disable automatic drift detection. Drift detection may be resource intensive in
the context of a large cluster or complex Terraform statefile. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>cliConfigSecretRef</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#secretreference-v1-core">
Kubernetes core/v1.SecretReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>healthChecks</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.HealthCheck">
[]HealthCheck
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of health checks to be performed.</p>
</td>
</tr>
<tr>
<td>
<code>destroyResourcesOnDeletion</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Create destroy plan and apply it to destroy terraform resources
upon deletion of this object. Defaults to false.</p>
</td>
</tr>
<tr>
<td>
<code>serviceAccountName</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Name of a ServiceAccount for the runner Pod to provision Terraform resources.
Default to tf-runner.</p>
</td>
</tr>
<tr>
<td>
<code>alwaysCleanupRunnerPod</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Clean the runner pod up after each reconciliation cycle</p>
</td>
</tr>
<tr>
<td>
<code>runnerTerminationGracePeriodSeconds</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Configure the termination grace period for the runner pod. Use this parameter
to allow the Terraform process to gracefully shutdown. Consider increasing for
large, complex or slow-moving Terraform managed resources.</p>
</td>
</tr>
<tr>
<td>
<code>refreshBeforeApply</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>RefreshBeforeApply forces refreshing of the state before the apply step.</p>
</td>
</tr>
<tr>
<td>
<code>runnerPodTemplate</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.RunnerPodTemplate">
RunnerPodTemplate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>enableInventory</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnableInventory enables the object to store resource entries as the inventory for external use.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.TerraformStatus">TerraformStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.Terraform">Terraform</a>)
</p>
<p>TerraformStatus defines the observed state of Terraform</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>ReconcileRequestStatus</code><br>
<em>
<a href="https://godoc.org/github.com/fluxcd/pkg/apis/meta#ReconcileRequestStatus">
github.com/fluxcd/pkg/apis/meta.ReconcileRequestStatus
</a>
</em>
</td>
<td>
<p>
(Members of <code>ReconcileRequestStatus</code> are embedded into this type.)
</p>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the last reconciled generation.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#condition-v1-meta">
[]Kubernetes meta/v1.Condition
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>lastAppliedRevision</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The last successfully applied revision.
The revision format for Git sources is <branch|tag>/<commit-sha>.</p>
</td>
</tr>
<tr>
<td>
<code>lastAttemptedRevision</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAttemptedRevision is the revision of the last reconciliation attempt.</p>
</td>
</tr>
<tr>
<td>
<code>lastPlannedRevision</code><br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastPlannedRevision is the revision used by the last planning process.
The result could be either no plan change or a new plan generated.</p>
</td>
</tr>
<tr>
<td>
<code>lastDriftDetectedAt</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastDriftDetectedAt is the time when the last drift was detected</p>
</td>
</tr>
<tr>
<td>
<code>lastAppliedByDriftDetectionAt</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastAppliedByDriftDetectionAt is the time when the last drift was detected and
terraform apply was performed as a result</p>
</td>
</tr>
<tr>
<td>
<code>availableOutputs</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>plan</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.PlanStatus">
PlanStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>inventory</code><br>
<em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.ResourceInventory">
ResourceInventory
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Inventory contains the list of Terraform resource object references that have been successfully applied.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.Variable">Variable
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformSpec">TerraformSpec</a>)
</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the variable</p>
</td>
</tr>
<tr>
<td>
<code>value</code><br>
<em>
<a href="https://pkg.go.dev/k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1?tab=doc#JSON">
Kubernetes pkg/apis/apiextensions/v1.JSON
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>valueFrom</code><br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.23/#envvarsource-v1-core">
Kubernetes core/v1.EnvVarSource
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.VarsReference">VarsReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformSpec">TerraformSpec</a>)
</p>
<p>VarsReference contain a reference of a Secret or a ConfigMap to generate
variables for Terraform resources based on its data, selectively by varsKey.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>kind</code><br>
<em>
string
</em>
</td>
<td>
<p>Kind of the values referent, valid values are (&lsquo;Secret&rsquo;, &lsquo;ConfigMap&rsquo;).</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name of the values referent. Should reside in the same namespace as the
referring resource.</p>
</td>
</tr>
<tr>
<td>
<code>varsKeys</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>VarsKeys is the data key where the values.yaml or a specific value can be
found at. Defaults to all keys.</p>
</td>
</tr>
<tr>
<td>
<code>optional</code><br>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Optional marks this VarsReference as optional. When set, a not found error
for the values reference is ignored, but any VarsKey or
transient error will still result in a reconciliation failure.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<h3 id="infra.contrib.fluxcd.io/v1alpha1.WriteOutputsToSecretSpec">WriteOutputsToSecretSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#infra.contrib.fluxcd.io/v1alpha1.TerraformSpec">TerraformSpec</a>)
</p>
<p>WriteOutputsToSecretSpec defines where to store outputs, and which outputs to be stored.</p>
<div class="md-typeset__scrollwrap">
<div class="md-typeset__table">
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the Secret to be written</p>
</td>
</tr>
<tr>
<td>
<code>outputs</code><br>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Outputs contain the selected names of outputs to be written
to the secret. Empty array means writing all outputs, which is default.</p>
</td>
</tr>
</tbody>
</table>
</div>
</div>
<div class="admonition note">
<p class="last">This page was automatically generated with <code>gen-crd-api-reference-docs</code></p>
</div>
