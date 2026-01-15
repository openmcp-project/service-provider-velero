package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// DomainServiceReconciler implements any business logic required to manage your APIObject
type DomainServiceReconciler[T APIObject, PC ProviderConfig] interface {
	// CreateOrUpdate is called on every add or update event
	CreateOrUpdate(ctx context.Context, obj T, pc PC, clusters ClusterContext) (ctrl.Result, error)
	// Delete is called on every delete event
	Delete(ctx context.Context, obj T, pc PC, clusters ClusterContext) (ctrl.Result, error)
}

// ClusterContext provides access to any potential target cluster
type ClusterContext struct {
	MCPCluster      *clusters.Cluster
	WorkloadCluster *clusters.Cluster
	PlatformCluster *clusters.Cluster
}

// APIObject represents an onboarding api type
type APIObject interface {
	client.Object
	APIObjectStatus
	Finalizer() string
}

// APIObjectStatus represents the status type of an onboarding api type
type APIObjectStatus interface {
	// GetStatus returns the status object
	GetStatus() any
	// GetConditions returns the status object
	GetConditions() *[]metav1.Condition
	// SetPhase sets Status.Phase
	SetPhase(string)
	// SetObservedGeneration sets Status.ObservedGeneration
	SetObservedGeneration(int64)
}

// ProviderConfig represents the config for platform operators
// The ProviderConfig is passed to the DomainServiceReconcile to reconcile APIObjects
type ProviderConfig interface {
	client.Object
	// PollIntveral can be used to periodically requeue, preventing managed objects
	// from drifting on the target cluster.  Return 0 if not required.
	PollInterval() time.Duration
}

// SPReconciler implements a generic reconcile loop to separate platform
// and service provider developer space.
type SPReconciler[T APIObject, PC ProviderConfig] struct {
	PlatformCluster         *clusters.Cluster
	OnboardingCluster       *clusters.Cluster
	ClusterAccessReconciler clusteraccess.Reconciler
	DomainServiceReconciler DomainServiceReconciler[T, PC]
	ProviderConfig          atomic.Pointer[PC]
}

// helper to create an empty APIObject
// background is the pointer/value receiver mismatch of the generated api types
// that don't satisfy client.Object
func (r *SPReconciler[T, PC]) emptyAPIObject() T {
	var t T
	// create elem based on type
	val := reflect.New(reflect.TypeOf(t).Elem())
	// cast empty elem back
	return val.Interface().(T)
}

// Reconcile orchestrates platform and DomainServiceReconciler logic to reconcile APIObjects
func (r *SPReconciler[T, PC]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	// common reconciler logic including get obj, providerconfig, mcp/workload access
	obj := r.emptyAPIObject()
	if err := r.OnboardingCluster.Client().Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	oldObj := obj.DeepCopyObject().(T)
	providerConfig := r.ProviderConfig.Load()
	if providerConfig == nil {
		StatusProgressing(obj, "ReconcileError", "No ProviderConfig found")
		r.updateStatus(ctx, obj, oldObj)
		return ctrl.Result{}, errors.New("provider config missing")
	}
	providerConfigCopy := (*providerConfig).DeepCopyObject().(PC)
	// core crud
	deleted := !obj.GetDeletionTimestamp().IsZero()
	var res ctrl.Result
	var err error
	if deleted {
		res, err = r.delete(ctx, obj, providerConfigCopy)
	} else {
		res, err = r.createOrUpdate(ctx, obj, providerConfigCopy)
		r.updateStatus(ctx, obj, oldObj)
	}
	// return based on result/err
	if err != nil {
		l.Error(err, "reconcile failed")
		return ctrl.Result{}, err
	}
	if res.RequeueAfter > 0 {
		return res, nil
	}
	// fallback to poll interval to prevent 'managed service' drift
	return ctrl.Result{
		RequeueAfter: providerConfigCopy.PollInterval(),
	}, nil
}

func (r *SPReconciler[T, PC]) updateStatus(ctx context.Context, new T, old T) {
	if equality.Semantic.DeepEqual(old.GetStatus(), new.GetStatus()) {
		return
	}
	if err := r.OnboardingCluster.Client().Status().Patch(ctx, new, client.MergeFrom(old)); err != nil {
		l := logf.FromContext(ctx)
		l.Error(err, "Patch status failed")
	}
}

func (r *SPReconciler[T, PC]) mcp(ctx context.Context, req ctrl.Request) (*clusters.Cluster, ctrl.Result, error) {
	res, err := r.ClusterAccessReconciler.Reconcile(ctx, req)
	if err != nil {
		return nil, ctrl.Result{}, err
	}
	if res.RequeueAfter > 0 {
		return nil, res, nil
	}
	mcpCluster, err := r.ClusterAccessReconciler.MCPCluster(ctx, req)
	if err != nil {
		return nil, ctrl.Result{}, err
	}
	return mcpCluster, ctrl.Result{}, nil
}

func (r *SPReconciler[T, PC]) workloadCluster(ctx context.Context, req ctrl.Request) (*clusters.Cluster, ctrl.Result, error) {
	res, err := r.ClusterAccessReconciler.Reconcile(ctx, req)
	if err != nil {
		return nil, ctrl.Result{}, err
	}
	if res.RequeueAfter > 0 {
		return nil, res, nil
	}
	workloadCluster, err := r.ClusterAccessReconciler.WorkloadCluster(ctx, req)
	if err != nil {
		return nil, ctrl.Result{}, err
	}
	return workloadCluster, ctrl.Result{}, nil
}

func (r *SPReconciler[T, PC]) delete(ctx context.Context, obj T, pc PC) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	oldObj := obj.DeepCopyObject().(T)

	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)}
	accessRequestsInDeletion, err := r.areAccessRequestsInDeletion(ctx, req)
	if err != nil {
		l.Error(err, "failed to check access requests for landscaper instance")
		return reconcile.Result{}, err
	}
	if !accessRequestsInDeletion {
		clusters, res, err := r.clusters(ctx, req)
		if err != nil {
			StatusProgressing(obj, "ReconcileError", "cluster setup error")
			return ctrl.Result{}, err
		}
		if res.RequeueAfter > 0 {
			StatusProgressing(obj, "Reconciling", "clusters being setup")
			return res, nil
		}
		res, err = r.DomainServiceReconciler.Delete(ctx, obj, pc, clusters)
		r.updateStatus(ctx, obj, oldObj)
		if err != nil {
			return ctrl.Result{}, err
		}
		if res.RequeueAfter > 0 {
			return res, nil
		}
	}
	// remove cluster access
	res, err := r.ClusterAccessReconciler.ReconcileDelete(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}
	// make sure to not drop the object before cleanup has been done
	if res.RequeueAfter > 0 {
		return res, nil
	}
	// remove finalizer
	controllerutil.RemoveFinalizer(obj, obj.Finalizer())
	if err := r.OnboardingCluster.Client().Update(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
func (r *SPReconciler[T, PC]) createOrUpdate(ctx context.Context, obj T, pc PC) (ctrl.Result, error) {
	if _, err := controllerutil.CreateOrUpdate(ctx, r.OnboardingCluster.Client(), obj, func() error {
		controllerutil.AddFinalizer(obj, obj.Finalizer())
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)}
	clusters, res, err := r.clusters(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}
	if res.RequeueAfter > 0 {
		return res, nil
	}
	return r.DomainServiceReconciler.CreateOrUpdate(ctx, obj, pc, clusters)
}

// areAccessRequestsInDeletion determines if the access requests for a reconcile request are in deletion.
// It returns true if at least one of the access requests (mcp, workload) is deleted or has a deletion timestamp.
func (r *SPReconciler[T, PC]) areAccessRequestsInDeletion(ctx context.Context, req ctrl.Request) (bool, error) {
	accessRequest, err := r.ClusterAccessReconciler.MCPAccessRequest(ctx, req)
	if apierrors.IsNotFound(err) {
		return true, nil
	} else if err != nil {
		return false, err
	} else if accessRequest.DeletionTimestamp != nil {
		return true, nil
	}

	accessRequest, err = r.ClusterAccessReconciler.WorkloadAccessRequest(ctx, req)
	if apierrors.IsNotFound(err) {
		return true, nil
	} else if err != nil {
		return false, err
	} else if accessRequest.DeletionTimestamp != nil {
		return true, nil
	}

	return false, nil
}

func (r *SPReconciler[T, PC]) clusters(ctx context.Context, req ctrl.Request) (ClusterContext, ctrl.Result, error) {
	l := logf.FromContext(ctx)
	mcp, res, err := r.mcp(ctx, req)
	if err != nil {
		return ClusterContext{}, res, err
	}
	workloadCluster, res, err := r.workloadCluster(ctx, req)
	if err != nil {
		l.Error(err, "workload cluster access error")
		return ClusterContext{}, res, err
	}
	if mcp == nil || workloadCluster == nil {
		return ClusterContext{}, res, errors.New("cluster access missing")
	}
	return ClusterContext{
		MCPCluster:      mcp,
		WorkloadCluster: workloadCluster,
	}, res, nil
}
