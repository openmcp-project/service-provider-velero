package meta

import "sigs.k8s.io/controller-runtime/pkg/client"

const (
	// LabelManagedBy defines the managed-by label that is added to every managed object.
	LabelManagedBy = "app.kubernetes.io/managed-by"
	// LabelManagedByValue defines the managed-by value that is added to every managed object.
	LabelManagedByValue = "service-provider-velero"

	labelInstanceID = "velero.services.openmcp.cloud/instance-id"
)

// SetManagedBy sets the managed-by label of the given client.Object.
func SetManagedBy(o client.Object) {
	labels := o.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[LabelManagedBy] = LabelManagedByValue
	o.SetLabels(labels)
}

// ManagedBy returns the managed-by label of the given client.Object.
func ManagedBy() client.ListOption {
	return client.MatchingLabels{
		LabelManagedBy: LabelManagedByValue,
	}
}

// SetInstanceID sets the instance id label of the given client.Object.
func SetInstanceID(o client.Object, instanceID string) {
	labels := o.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[labelInstanceID] = instanceID
}
