package resources

import (
	"context"

	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DeletionPolicy string

const (
	Orphan DeletionPolicy = "orphan"
	Delete DeletionPolicy = "delete"
)

type ReconcileFunc func(ctx context.Context, o client.Object) error

// NoOp does not do anything with the provided object and returns nil.
func NoOp(context.Context, client.Object) error {
	return nil
}

type StatusFunc func(o client.Object) Status

func SimpleStatus(o client.Object) Status {
	if !o.GetDeletionTimestamp().IsZero() {
		return Status{
			Phase:   v1alpha1.Terminating,
			Message: "Resource is terminating.",
		}
	}
	if o.GetUID() == "" {
		return Status{
			Phase:   v1alpha1.Pending,
			Message: "Resource has not been created yet.",
		}
	}
	return Status{
		Phase:   v1alpha1.Ready,
		Message: "Resource exists.",
	}
}

type Status struct {
	Phase   v1alpha1.InstancePhase
	Message string
}

func NewManagedObject(o client.Object, moc ManagedObjectContext) ManagedObject {
	if moc.DeletionPolicy == "" {
		moc.DeletionPolicy = Delete
	}

	return &managedObject{
		object:         o,
		reconcileFunc:  moc.ReconcileFunc,
		dependencies:   moc.DependsOn,
		deletionPolicy: moc.DeletionPolicy,
		statusFunc:     moc.StatusFunc,
	}
}

type ManagedObjectContext struct {
	ReconcileFunc  ReconcileFunc
	DependsOn      []ManagedObject
	DeletionPolicy DeletionPolicy
	StatusFunc     StatusFunc
}

type ManagedObject interface {
	GetObject() client.Object
	Reconcile(ctx context.Context) error
	GetDependencies() []ManagedObject
	GetDeletionPolicy() DeletionPolicy
	GetStatus() Status
}

var _ ManagedObject = &managedObject{}

type managedObject struct {
	object         client.Object
	reconcileFunc  ReconcileFunc
	statusFunc     StatusFunc
	dependencies   []ManagedObject
	deletionPolicy DeletionPolicy
}

// GetStatus implements ManagedObject.
func (m *managedObject) GetStatus() Status {
	if m.statusFunc != nil {
		return m.statusFunc(m.object)
	}
	return Status{
		Phase:   v1alpha1.Unknown,
		Message: "No status function defined.",
	}
}

// GetDeletionPolicy implements ManagedObject.
func (m *managedObject) GetDeletionPolicy() DeletionPolicy {
	return m.deletionPolicy
}

// GetDependencies implements ManagedObject.
func (m *managedObject) GetDependencies() []ManagedObject {
	return m.dependencies
}

// Reconcile implements ManagedObject.
func (m *managedObject) Reconcile(ctx context.Context) error {
	if m.reconcileFunc != nil {
		return m.reconcileFunc(ctx, m.object)
	}
	return nil
}

// GetObject implements ManagedObject.
func (m *managedObject) GetObject() client.Object {
	return m.object
}
