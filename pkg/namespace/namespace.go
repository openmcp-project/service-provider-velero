package namespace

import (
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
