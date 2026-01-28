package imagepullsecrets

import (
	"context"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	openmcpresources "github.com/openmcp-project/controller-utils/pkg/resources"
	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ManagedPullSecret struct {
	// PlatformCluster is the cluster the service provider runs in
	PlatformCluster *clusters.Cluster
	// SourceNamespace defines the origin namespace of the secret in the platform cluster
	SourceNamespace string
}

// adds every pull secret defined in the provider config to the namespace of the velero instance in the workload cluster
func (mps ManagedPullSecret) Configure(workloadCluster resources.ManagedCluster, providerConfig v1alpha1.ProviderConfig) {
	for _, pullSecret := range providerConfig.Spec.ImagePullSecrets {
		secret := resources.NewManagedObject(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pullSecret.Name,
				Namespace: workloadCluster.GetDefaultNamespace(),
			},
		}, resources.ManagedObjectContext{
			ReconcileFunc: func(ctx context.Context, o client.Object) error {
				oSecret := o.(*corev1.Secret)

				sourceSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pullSecret.Name,
						Namespace: mps.SourceNamespace,
					},
				}
				// retrieve source secret from platform cluster
				if err := mps.PlatformCluster.Client().Get(ctx, client.ObjectKeyFromObject(sourceSecret), sourceSecret); err != nil {
					return err
				}
				mutator := openmcpresources.NewSecretMutator(pullSecret.Name, workloadCluster.GetDefaultNamespace(), sourceSecret.Data, corev1.SecretTypeDockerConfigJson)
				return mutator.Mutate(oSecret)
			},
			StatusFunc: resources.SimpleStatus,
		})
		workloadCluster.AddObject(secret)
	}

}
