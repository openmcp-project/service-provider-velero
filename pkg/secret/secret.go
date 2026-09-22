package secret

import (
	"context"
	"errors"
	"slices"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	openmcpresources "github.com/openmcp-project/controller-utils/pkg/resources"
	"github.com/openmcp-project/extensibility-utils/pkg/objectmanager"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiv1alpha1 "github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/meta"
)

// ErrSecretCleanup is an user-facing error that indicates secret cleanup failures
var ErrSecretCleanup = errors.New("secret cleanup failed")

// Configure adds every pull secret defined in the provider config to the namespace of the velero instance in the workload cluster
func Configure(cluster objectmanager.Cluster, platformCluster *clusters.Cluster, imagePullSecrets []corev1.LocalObjectReference, sourceNamespace string) {
	for _, pullSecret := range imagePullSecrets {
		secret := objectmanager.NewObject(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pullSecret.Name,
				Namespace: cluster.DefaultNamespace(),
			},
		}, objectmanager.ObjectConfig{
			ReconcileFunc: func(ctx context.Context, o client.Object) error {
				oSecret := o.(*corev1.Secret)
				sourceSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pullSecret.Name,
						Namespace: sourceNamespace,
					},
				}
				// retrieve source secret from platform cluster
				if err := platformCluster.Client().Get(ctx, client.ObjectKeyFromObject(sourceSecret), sourceSecret); err != nil {
					return err
				}
				mutator := openmcpresources.NewSecretMutator(pullSecret.Name, cluster.DefaultNamespace(), sourceSecret.Data, corev1.SecretTypeDockerConfigJson)
				return mutator.Mutate(oSecret)
			},
			StatusFunc: objectmanager.SimpleStatus,
		})
		cluster.AddObject(secret)
	}
}

var _ objectmanager.Cleaner = &secretCleaner{}

type secretCleaner struct {
	cluster       objectmanager.Cluster
	namespace     string
	secretsToKeep []corev1.LocalObjectReference
}

// NewSecretCleaner removes redundant pull secrets in the given target namespace
// by removing any secret labeled as managed by service-provider-velero that is not in secretsToKeep.
func NewSecretCleaner(cluster objectmanager.Cluster, namespace string, secretsToKeep []corev1.LocalObjectReference) objectmanager.Cleaner {
	return &secretCleaner{
		cluster:       cluster,
		namespace:     namespace,
		secretsToKeep: secretsToKeep,
	}
}

func (c *secretCleaner) Cleanup(ctx context.Context) ([]objectmanager.Result, error) {
	results := []objectmanager.Result{}
	secretCopies := &corev1.SecretList{}
	cl := c.cluster.Client()
	if err := cl.List(ctx, secretCopies,
		client.InNamespace(c.namespace),
		client.MatchingLabels{meta.LabelManagedBy: meta.LabelManagedByValue},
	); err != nil {
		log.FromContext(ctx).Error(err, "failed to list secrets for orphan cleanup")
		return nil, ErrSecretCleanup
	}
	for _, secret := range secretCopies.Items {
		if !slices.ContainsFunc(c.secretsToKeep, func(ref corev1.LocalObjectReference) bool { return secret.Name == ref.Name }) {
			if err := cl.Delete(ctx, &secret); client.IgnoreNotFound(err) != nil {
				results = append(results, c.cleanupErrorResult(&secret, err))
			}
		}
	}
	return results, nil
}

func (c *secretCleaner) cleanupErrorResult(obj *corev1.Secret, err error) objectmanager.Result {
	return objectmanager.Result{
		Object: objectmanager.NewObject(
			obj,
			objectmanager.ObjectConfig{
				StatusFunc:     cleanupErrorStatus,
				DeletionPolicy: objectmanager.Delete,
			}),
		Cluster:         c.cluster,
		OperationResult: objectmanager.OperationResultDeletionFailed,
		Error:           err,
	}
}

func cleanupErrorStatus(_ client.Object) objectmanager.ManagedObjectStatus {
	return objectmanager.ManagedObjectStatus{
		Phase:   string(apiv1alpha1.Terminating),
		Message: "Secret cleanup failed",
	}
}
