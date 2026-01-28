package resources

import (
	"context"
	"errors"
	"fmt"

	"github.com/openmcp-project/service-provider-velero/pkg/meta"
	"github.com/openmcp-project/service-provider-velero/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	OperationResultDeletionRequested controllerutil.OperationResult = "deletionRequested"
	OperationResultDeleted           controllerutil.OperationResult = "deleted"
	OperationResultOrphaned          controllerutil.OperationResult = OperationResultDeleted
)

type MutateFn func(o client.Object) error

type dependents map[ManagedObject][]dependency

func NewManager(instanceID string) *Manager {
	return &Manager{
		instanceID: instanceID,
		clusters:   []ManagedCluster{},
	}
}

type Manager struct {
	instanceID string
	clusters   []ManagedCluster
}

func (m *Manager) AddCluster(mc ManagedCluster) {
	m.clusters = append(m.clusters, mc)
}

func (m *Manager) Apply(ctx context.Context) []Result {
	return m.reconcileObjects(ctx, false)
}

func (m *Manager) Delete(ctx context.Context) []Result {
	return m.reconcileObjects(ctx, true)
}

func (m *Manager) reconcileObjects(ctx context.Context, isDeletion bool) []Result {
	dependents := m.getDependents()

	// Apply objects from each cluster.
	results := []Result{}
	for _, mc := range m.clusters {
		for _, mo := range mc.GetObjects() {
			result := m.reconcileObject(ctx, mc, mo, dependents, isDeletion)
			results = append(results, result)
		}
	}

	return results
}

func (m *Manager) reconcileObject(ctx context.Context, mc ManagedCluster, mo ManagedObject, dependents dependents, isDeletion bool) Result {
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
	} else {
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
}

func (m *Manager) checkForDependents(ctx context.Context, deps []dependency) error {
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
		errs = append(errs, fmt.Errorf("dependent object still exists: %s", utils.ObjectID(obj)))
	}
	return errors.Join(errs...)
}

func (m *Manager) getDependents() dependents {
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

func AllDeleted(results []Result) bool {
	for _, r := range results {
		if r.OperationResult != OperationResultDeleted {
			return false
		}
	}
	return true
}
