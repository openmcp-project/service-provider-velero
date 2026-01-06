package meta

import "sigs.k8s.io/controller-runtime/pkg/client"

const (
	LabelManagedBy      = "app.kubernetes.io/managed-by"
	LabelManagedByValue = "service-provider-velero"
)

func SetManagedBy(o client.Object) {
	labels := o.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[LabelManagedBy] = LabelManagedByValue
	o.SetLabels(labels)
}

func ManagedBy() client.ListOption {
	return client.MatchingLabels{
		LabelManagedBy: LabelManagedByValue,
	}
}
