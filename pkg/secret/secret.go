package secret

import (
	"context"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	openmcpresources "github.com/openmcp-project/controller-utils/pkg/resources"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// adds every pull secret defined in the provider config to the namespace of the velero instance in the workload cluster
func Configure(cluster resources.ManagedCluster, platformCluster *clusters.Cluster, imagePullSecrets []corev1.LocalObjectReference, sourceNamespace string) {
	for _, pullSecret := range imagePullSecrets {
		secret := resources.NewManagedObject(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pullSecret.Name,
				Namespace: cluster.GetDefaultNamespace(),
			},
		}, resources.ManagedObjectContext{
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
				mutator := openmcpresources.NewSecretMutator(pullSecret.Name, cluster.GetDefaultNamespace(), sourceSecret.Data, corev1.SecretTypeDockerConfigJson)
				return mutator.Mutate(oSecret)
			},
			StatusFunc: resources.SimpleStatus,
		})
		cluster.AddObject(secret)
	}
}
