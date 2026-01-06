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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/openmcp-operator/lib/clusteraccess"

	apiv1alpha1 "github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	spruntime "github.com/openmcp-project/service-provider-velero/pkg/runtime"
)

// VeleroReconciler reconciles a Velero object
type VeleroReconciler struct {
	OnboardingCluster       *clusters.Cluster
	PlatformCluster         *clusters.Cluster
	ClusterAccessReconciler clusteraccess.Reconciler
}

// CreateOrUpdate is called on every add or update event
func (r *VeleroReconciler) CreateOrUpdate(_ context.Context, _ *apiv1alpha1.Velero, _ *apiv1alpha1.ProviderConfig, _ *clusters.Cluster) (ctrl.Result, error) {
	// TODO
	return ctrl.Result{}, nil
}

// Delete is called on every delete event
func (r *VeleroReconciler) Delete(_ context.Context, _ *apiv1alpha1.Velero, _ *apiv1alpha1.ProviderConfig, _ *clusters.Cluster) (ctrl.Result, error) {
	// TODO
	return ctrl.Result{}, nil
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
