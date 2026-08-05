package resources

import (
	"context"
	"errors"
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openmcp-project/service-provider-velero/pkg/meta"
	"github.com/openmcp-project/service-provider-velero/pkg/objectutils"
)

const (
	// OperationResultDeletionFailed indicates failed to be deleted
	OperationResultDeletionFailed controllerutil.OperationResult = "deletionFailed"
	// OperationResultDeletionRequested indicates that an object has been marked for deletion
	OperationResultDeletionRequested controllerutil.OperationResult = "deletionRequested"
	// OperationResultDeleted indicates that an object has been deleted
	OperationResultDeleted controllerutil.OperationResult = "deleted"
	// OperationResultOrphaned indicates that an object has been orphaned
	OperationResultOrphaned controllerutil.OperationResult = OperationResultDeleted
)

type dependents map[ManagedObject][]dependency

// Manager manages the objects of an arbitrary number of clusters
type Manager interface {
	AddCluster(mc ManagedCluster)
	AddCleaner(oc OrphanCleaner)
	Apply(context.Context) ([]Result, error)
	Delete(context.Context) ([]Result, error)
}

// OrphanCleaner removes any previously managed objects that are no longer part of the desired state.
type OrphanCleaner interface {
	// []Result contains cleanup errors that can be mapped to a managed object.
	// error represents cleanup errors that cannot be mapped to a managed object.
	Cleanup(ctx context.Context) ([]Result, error)
}

// NewManager creates a new Manager instance.
func NewManager(instanceID string) Manager {
	return &managerImpl{
		instanceID: instanceID,
		clusters:   []ManagedCluster{},
		cleaners:   []OrphanCleaner{},
	}
}

// managerImpl manages clusters and invokes reconciliation of ManagedObjects.
type managerImpl struct {
	instanceID string
	clusters   []ManagedCluster
	cleaners   []OrphanCleaner
}

// AddCluster adds a cluster to a Manager.
func (m *managerImpl) AddCluster(mc ManagedCluster) {
	m.clusters = append(m.clusters, mc)
}

// Apply invokes reconciliation of all ManagedObjects.
func (m *managerImpl) Apply(ctx context.Context) ([]Result, error) {
	return m.reconcileObjects(ctx, false)
}

// AddCleaner adds a cleaner to a Manager.
func (m *managerImpl) AddCleaner(cleaner OrphanCleaner) {
	m.cleaners = append(m.cleaners, cleaner)
}

// Delete invokes deletion of all ManagedObjects.
func (m *managerImpl) Delete(ctx context.Context) ([]Result, error) {
	return m.reconcileObjects(ctx, true)
}

func (m *managerImpl) reconcileObjects(ctx context.Context, isDeletion bool) ([]Result, error) {
	dependents := m.getDependents()

	// Apply objects from each cluster.
	results := []Result{}
	for _, mc := range m.clusters {
		for _, mo := range mc.GetObjects() {
			result := m.reconcileObject(ctx, mc, mo, dependents, isDeletion)
			results = append(results, result)
		}
	}

	// remove any redundant resources like secret copies that are no longer part of the desired state.
	for _, c := range m.cleaners {
		result, err := c.Cleanup(ctx)
		if err != nil {
			return results, err
		}
		results = slices.Concat(results, result)
	}

	return results, nil
}

func (m *managerImpl) reconcileObject(ctx context.Context, mc ManagedCluster, mo ManagedObject, dependents dependents, isDeletion bool) Result {
	client := mc.GetClient()
	obj := mo.GetObject()

	if isDeletion {
		if err := m.checkForDependents(ctx, dependents[mo]); err != nil {
			return Result{
				Object:          mo,
				Cluster:         mc,
				OperationResult: controllerutil.OperationResultNone,
				Error:           err,
			}
		}

		if mo.GetDeletionPolicy() == Orphan {
			return Result{
				Object:          mo,
				Cluster:         mc,
				OperationResult: OperationResultOrphaned,
				Error:           nil,
			}
		}

		err := client.Delete(ctx, obj)
		if apierrors.IsNotFound(err) {
			return Result{
				Object:          mo,
				Cluster:         mc,
				OperationResult: OperationResultDeleted,
				Error:           nil,
			}
		}
		return Result{
			Object:          mo,
			Cluster:         mc,
			OperationResult: OperationResultDeletionRequested,
			Error:           err,
		}
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, client, obj, func() error {
		meta.SetManagedBy(obj)
		meta.SetInstanceID(obj, m.instanceID)
		return mo.Reconcile(ctx)
	})
	return Result{
		Object:          mo,
		Cluster:         mc,
		OperationResult: opResult,
		Error:           err,
	}
}

func (m *managerImpl) checkForDependents(ctx context.Context, deps []dependency) error {
	errs := []error{}
	for _, dep := range deps {
		obj := dep.Object.GetObject()
		err := dep.Cluster.GetClient().Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if apierrors.IsNotFound(err) {
			// "Not found" is the success case: The object which depends on us does not exist anymore.
			continue
		}
		if err != nil {
			// Some unexpected error occurred.
			errs = append(errs, err)
			continue
		}
		// No error occurred, the GET request has been successful.
		// The object still exists and depends on us.
		errs = append(errs, fmt.Errorf("dependent object still exists: %s", objectutils.ObjectID(obj)))
	}
	return errors.Join(errs...)
}

func (m *managerImpl) getDependents() dependents {
	deps := dependents{}
	for _, mc := range m.clusters {
		for _, mo := range mc.GetObjects() {
			for _, dep := range mo.GetDependencies() {
				if deps[dep] == nil {
					deps[dep] = []dependency{}
				}
				deps[dep] = append(deps[dep], dependency{
					Object:  mo,
					Cluster: mc,
				})
			}
		}
	}
	return deps
}

// Result summarizes a reconciliation result.
type Result struct {
	Object          ManagedObject
	Cluster         ManagedCluster
	OperationResult controllerutil.OperationResult
	Error           error
}

type dependency struct {
	Object  ManagedObject
	Cluster ManagedCluster
}

// AllDeleted returns true if every item's operation result is OperationResultDeleted.
func AllDeleted(results []Result) bool {
	for _, r := range results {
		if r.OperationResult != OperationResultDeleted {
			return false
		}
	}
	return true
}
