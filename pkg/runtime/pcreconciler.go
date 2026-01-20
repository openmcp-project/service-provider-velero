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

package runtime

import (
	"context"
	"reflect"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openmcp-project/controller-utils/pkg/clusters"

	v1alpha1 "github.com/openmcp-project/service-provider-velero/api/v1alpha1"
)

// ProviderConfigReconciler reconciles a ProviderConfig object
type PCReconciler[T ProviderConfig] struct {
	platformCluster       *clusters.Cluster
	onboardingCluster     *clusters.Cluster
	providerUpdateChannel chan event.GenericEvent
}

func NewPCReconciler[T ProviderConfig]() *PCReconciler[T] {
	return &PCReconciler[T]{}
}

func (r *PCReconciler[T]) WithPlatformCluster(c *clusters.Cluster) *PCReconciler[T] {
	r.platformCluster = c
	return r
}

func (r *PCReconciler[T]) WithOnboardingCluster(c *clusters.Cluster) *PCReconciler[T] {
	r.onboardingCluster = c
	return r
}

func (r *PCReconciler[T]) WithUpdateChannel(c chan event.GenericEvent) *PCReconciler[T] {
	r.providerUpdateChannel = c
	return r
}

// helper to create an empty ProviderConfig objects
// background is the pointer/value receiver mismatch of the generated api types
// that don't satisfy client.Object
func (r *PCReconciler[T]) emptyObject() T {
	var t T
	// create elem based on type
	val := reflect.New(reflect.TypeOf(t).Elem())
	// cast empty elem back
	return val.Interface().(T)
}

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *PCReconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj v1alpha1.ProviderConfig
	notify := event.GenericEvent{}
	if err := r.platformCluster.Client().Get(ctx, req.NamespacedName, &obj); err != nil {
		r.providerUpdateChannel <- notify
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		r.providerUpdateChannel <- notify
		return ctrl.Result{}, nil
	}
	notify.Object = obj.DeepCopy()
	r.providerUpdateChannel <- notify
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PCReconciler[T]) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WatchesRawSource(source.Kind(r.platformCluster.Cluster().GetCache(), r.emptyObject(), &handler.TypedEnqueueRequestForObject[T]{})).
		Named("providerconfig").
		Complete(r)
}
