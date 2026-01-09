/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"reflect"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	apiv1alpha1 "github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/authz"
	"github.com/openmcp-project/service-provider-velero/pkg/crds"
	"github.com/openmcp-project/service-provider-velero/pkg/deploy"
	"github.com/openmcp-project/service-provider-velero/pkg/namespace"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	spruntime "github.com/openmcp-project/service-provider-velero/pkg/runtime"
	"github.com/openmcp-project/service-provider-velero/pkg/utils"
)

// VeleroReconciler reconciles a Velero object
type VeleroReconciler struct {
	OnboardingCluster       *clusters.Cluster
	PlatformCluster         *clusters.Cluster
	ClusterAccessReconciler clusteraccess.Reconciler
}

// CreateOrUpdate is called on every add or update event
func (r *VeleroReconciler) CreateOrUpdate(ctx context.Context, obj *apiv1alpha1.Velero, _ *apiv1alpha1.ProviderConfig, mcp *clusters.Cluster) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	mgr, err := configResources(obj, mcp, mcp)
	if err != nil {
		return ctrl.Result{}, err
	}
	spruntime.StatusProgressing(obj, "Reconciling", "Reconcile in progress")
	results := mgr.Apply(ctx)
	for _, r := range results {
		if r.Error != nil {
			l.Error(r.Error, utils.ObjectID(r.Object.GetObject()))
		}
	}
	managedResources := resultsToResources(results)
	obj.Status.Resources = managedResources
	if allResourcesReady(managedResources) {
		spruntime.StatusReady(obj)
	}
	return ctrl.Result{}, nil
}

// Delete is called on every delete event
func (r *VeleroReconciler) Delete(ctx context.Context, obj *apiv1alpha1.Velero, _ *apiv1alpha1.ProviderConfig, mcp *clusters.Cluster) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	mgr, err := configResources(obj, mcp, mcp)
	if err != nil {
		return ctrl.Result{}, err
	}
	spruntime.StatusTerminating(obj)
	results := mgr.Delete(ctx)
	for _, r := range results {
		if r.Error != nil {
			l.Error(r.Error, utils.ObjectID(r.Object.GetObject()))
		}
	}
	if resources.AllDeleted(results) {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{
		RequeueAfter: time.Second * 5,
	}, nil
}

func configResources(obj *apiv1alpha1.Velero, mcp *clusters.Cluster, workload *clusters.Cluster) (*resources.Manager, error) {
	workloadCluster := resources.NewManagedCluster(workload.Client(), workload.RESTConfig(), "velero")
	mcpCluster := resources.NewManagedCluster(mcp.Client(), mcp.RESTConfig(), "velero")
	// ### MCP RESOURCES ###
	namespace.Configure(mcpCluster)
	// service account
	mcpServiceAccount := &authn.ManagedServiceAccount{
		NamespacedName: types.NamespacedName{
			Name:      "velero-server",
			Namespace: mcpCluster.GetDefaultNamespace(),
		},
	}
	tokenFunc := mcpServiceAccount.Configure(workloadCluster, mcpCluster)
	if err := crds.Configure(mcpCluster); err != nil {
		return nil, err
	}
	// creates ClusterRolebinding to ClusterRole cluster-admin for ServiceAccount 'velero-server'
	authz.Configure(mcpCluster, mcpServiceAccount)

	// ### WORKLOAD RESOURCES ###
	// create velero namespace
	namespace.Configure(workloadCluster)
	deploy.Configure(workloadCluster, mcpCluster.GetDefaultNamespace(), obj, tokenFunc)

	// manager
	mgr := resources.NewManager()
	mgr.AddCluster(mcpCluster)
	mgr.AddCluster(workloadCluster)
	return mgr, nil
}

func resultsToResources(results []resources.Result) []apiv1alpha1.ManagedResource {
	resources := []apiv1alpha1.ManagedResource{}
	for _, res := range results {
		obj := res.Object.GetObject()
		status := res.Object.GetStatus()
		resources = append(resources, apiv1alpha1.ManagedResource{
			TypedObjectReference: corev1.TypedObjectReference{
				Kind:      reflect.TypeOf(obj).Elem().Name(),
				Name:      obj.GetName(),
				Namespace: nilIfEmptyString(obj.GetNamespace()),
			},
			Phase:   status.Phase,
			Message: status.Message,
		})
	}
	return resources
}

func nilIfEmptyString(str string) *string {
	if str == "" {
		return nil
	}
	return ptr.To(str)
}

func allResourcesReady(resources []apiv1alpha1.ManagedResource) bool {
	for _, res := range resources {
		if res.Phase != apiv1alpha1.Ready {
			return false
		}
	}
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *VeleroReconciler) SetupWithManager(mgr ctrl.Manager, providerConfigUpdates chan event.GenericEvent) error {
	spReconciler := spruntime.SPReconciler[*apiv1alpha1.Velero, *apiv1alpha1.ProviderConfig]{
		OnboardingCluster:       r.OnboardingCluster,
		PlatformCluster:         r.PlatformCluster,
		ClusterAccessReconciler: r.ClusterAccessReconciler,
		DomainServiceReconciler: r,
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1alpha1.Velero{}).
		// sets up reconciles whenever provider config controller sends update events
		WatchesRawSource(
			source.Channel(
				providerConfigUpdates,
				handler.EnqueueRequestsFromMapFunc(
					func(ctx context.Context, obj client.Object) []reconcile.Request {
						// update cached provider config
						if obj != nil {
							copyPC := obj.(*apiv1alpha1.ProviderConfig).DeepCopy()
							spReconciler.ProviderConfig.Store(&copyPC)
						} else {
							spReconciler.ProviderConfig.Store(nil)
						}
						// reconcile all existing objects
						var list apiv1alpha1.VeleroList
						if err := r.OnboardingCluster.Client().List(ctx, &list); err != nil {
							return nil
						}
						reqs := make([]reconcile.Request, len(list.Items))
						for i := range list.Items {
							reqs[i] = reconcile.Request{
								NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
							}
						}
						return reqs
					},
				)),
		).
		Named("velero").
		Complete(&spReconciler)
}
