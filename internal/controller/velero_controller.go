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
	"errors"
	"fmt"
	"reflect"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/openmcp-project/controller-utils/pkg/clusters"

	apiv1alpha1 "github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/authz"
	"github.com/openmcp-project/service-provider-velero/pkg/crds"
	"github.com/openmcp-project/service-provider-velero/pkg/deployment"
	"github.com/openmcp-project/service-provider-velero/pkg/imagepullsecrets"
	"github.com/openmcp-project/service-provider-velero/pkg/instance"
	"github.com/openmcp-project/service-provider-velero/pkg/namespace"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	spruntime "github.com/openmcp-project/service-provider-velero/pkg/runtime"
	"github.com/openmcp-project/service-provider-velero/pkg/utils"
)

// VeleroReconciler reconciles a Velero object
type VeleroReconciler struct {
	OnboardingCluster *clusters.Cluster
	// PodNamespace is the namespace the service provider pod runs in
	// e.g. required to resolve image pull secret references from the provider config
	PodNamespace string
}

// CreateOrUpdate is called on every add or update event
func (r *VeleroReconciler) CreateOrUpdate(ctx context.Context, obj *apiv1alpha1.Velero, pc *apiv1alpha1.ProviderConfig, clusters spruntime.ClusterContext) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	spruntime.StatusProgressing(obj, "Reconciling", "Reconcile in progress")
	err := r.ensureInstanceID(ctx, obj)
	if err != nil {
		return ctrl.Result{}, err
	}
	mgr, err := r.configResources(obj, pc, clusters)
	if err != nil {
		return ctrl.Result{}, err
	}
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
func (r *VeleroReconciler) Delete(ctx context.Context, obj *apiv1alpha1.Velero, pc *apiv1alpha1.ProviderConfig, clusters spruntime.ClusterContext) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	spruntime.StatusTerminating(obj)
	mgr, err := r.configResources(obj, pc, clusters)
	if err != nil {
		return ctrl.Result{}, err
	}
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

func (r *VeleroReconciler) configResources(obj *apiv1alpha1.Velero, pc *apiv1alpha1.ProviderConfig, clusters spruntime.ClusterContext) (*resources.Manager, error) {
	// check all requested compoments are available
	images := resolveImages(obj.Spec, *pc)
	if images == nil {
		return nil, errors.New("requested version is not available")
	}
	workloadCluster := resources.NewManagedCluster(clusters.WorkloadCluster.Client(), clusters.WorkloadCluster.RESTConfig(), instance.Namespace(obj), resources.WorkloadCluter)
	mcpCluster := resources.NewManagedCluster(clusters.MCPCluster.Client(), clusters.MCPCluster.RESTConfig(), "velero", resources.ManagedControlPlane)
	// ### MCP RESOURCES ###
	// deletion policy orphan to prevent deleting end user data that we are not aware of
	namespace.Configure(mcpCluster, resources.Orphan)
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
	// create 'dummy' deployment
	deployment.ConfigureMcp(mcpCluster, mcpCluster.GetDefaultNamespace(), images["velero"], obj)

	// ### WORKLOAD RESOURCES ###
	// create velero namespace
	// TODO link onboarding api object and managed resources with additional instance status/label/annotation
	namespace.Configure(workloadCluster, resources.Delete)
	// sync image pull secrets to workload cluster
	secretManager := imagepullsecrets.ManagedPullSecret{
		PlatformCluster: clusters.PlatformCluster,
		SourceNamespace: r.PodNamespace,
	}
	secretManager.Configure(workloadCluster, *pc)
	// server deployment
	deployment.Configure(workloadCluster, mcpCluster.GetDefaultNamespace(), obj, *pc, images, tokenFunc)

	// manager
	mgr := resources.NewManager(instance.GetID(obj))
	mgr.AddCluster(mcpCluster)
	mgr.AddCluster(workloadCluster)
	return mgr, nil
}

func resultsToResources(results []resources.Result) []apiv1alpha1.ManagedResource {
	resources := []apiv1alpha1.ManagedResource{}
	for _, res := range results {
		obj := res.Object.GetObject()
		status := res.Object.GetStatus(apiv1alpha1.ResourceLocation(res.Cluster.GetClusterType()))
		resources = append(resources, apiv1alpha1.ManagedResource{
			TypedObjectReference: corev1.TypedObjectReference{
				Kind:      reflect.TypeOf(obj).Elem().Name(),
				Name:      obj.GetName(),
				Namespace: nilIfEmptyString(obj.GetNamespace()),
			},
			Phase:    status.Phase,
			Message:  status.Message,
			Location: status.Location,
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

// maps the requested components to their images
// returns nil if component/version is not available
func resolveImages(velero apiv1alpha1.VeleroSpec, pc apiv1alpha1.ProviderConfig) map[string]string {
	veleroServerImage := resolveImage("velero", velero.Version, pc)
	if veleroServerImage == "" {
		return nil
	}
	res := map[string]string{}
	res["velero"] = veleroServerImage
	for _, plugin := range velero.Plugins {
		image := resolveImage(plugin.Name, plugin.Version, pc)
		if image == "" {
			return nil
		}
		res[plugin.Name] = image
	}
	return res
}

// returns the image of a requested component
// or "" if the requested component is not available
func resolveImage(name string, version string, pc apiv1alpha1.ProviderConfig) string {
	for _, availableImage := range pc.Spec.AvailableImages {
		if availableImage.Name != name {
			continue
		}
		for _, availableVersion := range availableImage.Versions {
			if availableVersion == version {
				return availableImage.Image
			}
		}
	}
	return ""
}

// sets an instance id that is used to label every managed resource and create an instance namespace on the workload cluster
func (r *VeleroReconciler) ensureInstanceID(ctx context.Context, obj *apiv1alpha1.Velero) error {
	if len(instance.GetID(obj)) == 0 {
		instance.SetID(obj, instance.GenerateID(obj))
		if err := r.OnboardingCluster.Client().Update(ctx, obj); err != nil {
			return fmt.Errorf("failed to set instance id of velero resource %s/%s: %w", obj.Namespace, obj.Name, err)
		}
	}
	return nil
}
