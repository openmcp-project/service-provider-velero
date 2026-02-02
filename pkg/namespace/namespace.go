package namespace

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openmcp-project/service-provider-velero/pkg/resources"
)

// Configure adds a managed Namespace object to the given ManagedCluster.
func Configure(cluster resources.ManagedCluster, deletionPolicy resources.DeletionPolicy) {
	ns := resources.NewManagedObject(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cluster.GetDefaultNamespace(),
		},
	}, resources.ManagedObjectContext{
		ReconcileFunc:  resources.NoOp,
		DeletionPolicy: deletionPolicy,
		StatusFunc:     resources.SimpleStatus,
	})
	cluster.AddObject(ns)
}
