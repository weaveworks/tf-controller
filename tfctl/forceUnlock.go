package tfctl

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ForceUnlock will set the corresponding field and request a reconciliation for
// the corresponding resource.
func (c *CLI) ForceUnlock(out io.Writer, resource, lockID string) error {
	key := types.NamespacedName{
		Name:      resource,
		Namespace: c.namespace,
	}

	err := setForceUnlockAndReconcile(context.TODO(), c.client, key, lockID)

	if err != nil {
		return err
	}

	fmt.Fprintf(out, " Reconcile requested for %s/%s\n", c.namespace, resource)
	return nil
}

func setForceUnlockAndReconcile(ctx context.Context, kubeClient client.Client, namespacedName types.NamespacedName, lockID string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		terraform := &infrav1.Terraform{}

		if err := kubeClient.Get(ctx, namespacedName, terraform); err != nil {
			return err
		}

		patch := client.MergeFrom(terraform.DeepCopy())

		if terraform.Spec.BackendConfig == nil {
			terraform.Spec.BackendConfig = &infrav1.BackendConfigSpec{
				State: &infrav1.BackendConfigStateSpec{
					ForceUnlock: lockID,
				},
			}
		} else {
			if terraform.Spec.BackendConfig.State == nil {
				terraform.Spec.BackendConfig.State = &infrav1.BackendConfigStateSpec{
					ForceUnlock: lockID,
				}
			} else {
				if terraform.Spec.BackendConfig.State.ForceUnlock != infrav1.StateForceUnlockAutoValue {
					terraform.Spec.BackendConfig.State.ForceUnlock = lockID
				}
			}
		}

		if ann := terraform.GetAnnotations(); ann == nil {
			terraform.SetAnnotations(map[string]string{
				meta.ReconcileRequestAnnotation: time.Now().Format(time.RFC3339Nano),
			})
		} else {
			ann[meta.ReconcileRequestAnnotation] = time.Now().Format(time.RFC3339Nano)
			terraform.SetAnnotations(ann)
		}

		return kubeClient.Patch(ctx, terraform, patch)
	})
}
