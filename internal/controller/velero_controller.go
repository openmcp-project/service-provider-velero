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
	"fmt"
	"slices"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/types"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	ctrlerrors "github.com/openmcp-project/controller-utils/pkg/errors"
	"github.com/openmcp-project/extensibility-utils/pkg/objectmanager"

	"github.com/openmcp-project/opencontrolplane-runtime/pkg/serviceprovider"
	"github.com/openmcp-project/opencontrolplane-runtime/pkg/serviceprovider/clusteraccess"

	apiv1alpha1 "github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/authz"
	"github.com/openmcp-project/service-provider-velero/pkg/crds"
	"github.com/openmcp-project/service-provider-velero/pkg/deployment"
	"github.com/openmcp-project/service-provider-velero/pkg/instance"
	"github.com/openmcp-project/service-provider-velero/pkg/namespace"
	"github.com/openmcp-project/service-provider-velero/pkg/secret"
)

// VeleroReconciler reconciles a Velero object
type VeleroReconciler struct {
	// OnboardingCluster is the cluster where this controller watches Velero resources and reacts to their changes.
	OnboardingCluster *clusters.Cluster
	// PlatformCluster is the cluster where this controller is deployed and configured.
	PlatformCluster *clusters.Cluster
	// PodNamespace is the namespace where this controller is deployed in.
	PodNamespace string
	// Create a manager for the obj to reconcile
	CreateManager func(client.Object) objectmanager.Manager
}

// CreateOrUpdate is called on every add or update event
func (r *VeleroReconciler) CreateOrUpdate(ctx context.Context, obj *apiv1alpha1.Velero, pc *apiv1alpha1.ProviderConfig, clusters clusteraccess.ClusterContext) (ctrl.Result, error) {
	mgr, err := r.createObjectManager(ctx, obj, pc, clusters)
	if err != nil {
		serviceprovider.StatusProgressing(obj, "ReconcileError", err.Error())
		return ctrl.Result{}, ctrlerrors.IgnoreInvalidUserInput(err)
	}
	results, err := mgr.Apply(ctx)
	obj.Status.Resources = results.ManagedObjects()
	if err != nil {
		log.FromContext(ctx).Error(err, "failed to create or update managed objects")
		serviceprovider.StatusProgressing(obj, "ReconcileError", err.Error())
		return ctrl.Result{}, err
	}
	if results.Requeue {
		serviceprovider.StatusProgressing(obj, "Reconciling", "Reconcile in progress")
		return ctrl.Result{
			RequeueAfter: time.Second * 5,
		}, nil
	}
	serviceprovider.StatusReady(obj)
	return ctrl.Result{}, nil
}

// Delete is called on every delete event
func (r *VeleroReconciler) Delete(ctx context.Context, obj *apiv1alpha1.Velero, pc *apiv1alpha1.ProviderConfig, clusters clusteraccess.ClusterContext) (ctrl.Result, error) {
	mgr, err := r.createObjectManager(ctx, obj, pc, clusters)
	if err != nil {
		serviceprovider.StatusProgressing(obj, "ReconcileError", err.Error())
		return ctrl.Result{}, ctrlerrors.IgnoreInvalidUserInput(err)
	}
	results, err := mgr.Delete(ctx)
	obj.Status.Resources = results.ManagedObjects()
	if err != nil {
		log.FromContext(ctx).Error(err, "failed to delete managed objects")
		serviceprovider.StatusTerminatingWithReason(obj, "ReconcileError", err.Error())
		return ctrl.Result{}, err
	}
	serviceprovider.StatusTerminating(obj)
	if results.Requeue {
		return ctrl.Result{
			RequeueAfter: time.Second * 5,
		}, nil
	}
	return ctrl.Result{}, nil
}

func (r *VeleroReconciler) createObjectManager(ctx context.Context, obj *apiv1alpha1.Velero, pc *apiv1alpha1.ProviderConfig, clusters clusteraccess.ClusterContext) (objectmanager.Manager, error) {
	err := r.ensureInstanceID(ctx, obj)
	if err != nil {
		return nil, err
	}
	// ensure that all images are available for the requested velero and plugin versions
	images := resolveImages(obj.Spec, *pc)
	if images == nil {
		return nil, fmt.Errorf("%w: requested version is not available", ctrlerrors.ErrInvalidUserInput)
	}
	workloadCluster := objectmanager.NewCluster(clusters.WorkloadCluster, instance.Namespace(obj), objectmanager.WorkloadCluster)
	mcpCluster := objectmanager.NewCluster(clusters.MCPCluster, "velero", objectmanager.ManagedControlPlane)

	// ### MCP RESOURCES ###
	// set namespace deletion policy orphan to prevent deleting end user data that we are not aware of
	namespace.Configure(mcpCluster, objectmanager.Orphan)
	mcpServiceAccount := &authn.ManagedServiceAccount{
		NamespacedName: types.NamespacedName{
			Name:      "velero-server",
			Namespace: mcpCluster.DefaultNamespace(),
		},
	}
	tokenFunc := mcpServiceAccount.Configure(workloadCluster, mcpCluster, pc.PollInterval())
	if err := crds.Configure(mcpCluster); err != nil {
		return nil, err
	}
	authz.Configure(mcpCluster, mcpServiceAccount)
	// create 'dummy' deployment
	deployment.ConfigureMcp(mcpCluster, images["velero"], instance.GetID(obj))

	// ### WORKLOAD RESOURCES ###
	namespace.Configure(workloadCluster, objectmanager.Delete)
	secret.Configure(workloadCluster, r.PlatformCluster, pc.Spec.ImagePullSecrets, r.PodNamespace)
	deployment.Configure(workloadCluster, mcpCluster.DefaultNamespace(), obj, pc.Spec.ImagePullSecrets, images, tokenFunc)

	// ### MANAGE WORKLOAD AND MCP CLUSTER ###
	// mgr := resources.NewManager(instance.GetID(obj))
	mgr := r.CreateManager(obj)
	mgr.AddCluster(mcpCluster)
	mgr.AddCluster(workloadCluster)

	// create cleaner to remove orphaned pull secret copies from workload cluster
	secretsToKeep := append(slices.Clone(pc.Spec.ImagePullSecrets), mcpServiceAccount.SecretRef())
	workloadSecretCleaner := secret.NewSecretCleaner(workloadCluster, workloadCluster.DefaultNamespace(), secretsToKeep)
	mgr.AddCleaner(workloadSecretCleaner)

	return mgr, nil
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

// returns the image with its version if a requested component is available
// or "" if a requested component is not available
func resolveImage(name string, version string, pc apiv1alpha1.ProviderConfig) string {
	for _, availableImage := range pc.Spec.AvailableImages {
		if availableImage.Name != name {
			continue
		}
		for _, availableVersion := range availableImage.Versions {
			if availableVersion == version {
				return fmt.Sprintf("%s:%s", availableImage.Image, availableVersion)
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
