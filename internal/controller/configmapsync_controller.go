/*
Copyright 2024.

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
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "github.com/jason-cky/resource-replicator-operator/api/v1"
	"github.com/jason-cky/resource-replicator-operator/internal/utils"
	kcorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigMapSyncReconciler reconciles a ConfigMapSync object
type ConfigMapSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var (
	labelKey = "configmapsync.replicator/controlled-by"
)

//+kubebuilder:rbac:groups=apps.replicator,resources=configmapsyncs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps.replicator,resources=configmapsyncs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps.replicator,resources=configmapsyncs/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=configmaps/finalizers,verbs=update

func (r *ConfigMapSyncReconciler) getAllNamespaces(ctx context.Context) (kcorev1.NamespaceList, error) {
	var allNamespaces kcorev1.NamespaceList
	if err := r.List(ctx, &allNamespaces, &client.ListOptions{}); err != nil {
		return allNamespaces, err
	}

	return allNamespaces, nil
}

func (r *ConfigMapSyncReconciler) upsertDestinationConfigMap(
	ctx context.Context,
	req ctrl.Request,
	destinationConfigMapName types.NamespacedName,
	sourceConfigMap *kcorev1.ConfigMap,
	configMapSync *appsv1.ConfigMapSync,
) error {
	log := log.FromContext(ctx)
	destinationConfigMap := &kcorev1.ConfigMap{}
	if err := r.Get(ctx, destinationConfigMapName, destinationConfigMap); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating ConfigMap in destination namespace", "Namespace", destinationConfigMapName.Namespace)
			destinationConfigMap = &kcorev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapSync.Spec.ConfigMapName,
					Namespace: destinationConfigMapName.Namespace,
				},
				Data: sourceConfigMap.Data, // Copy data from source to destination
			}
			if err := r.Create(ctx, destinationConfigMap); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		log.Info("Updating ConfigMap in destination namespace", "Namespace", destinationConfigMapName.Namespace)
		destinationConfigMap.Data = sourceConfigMap.Data // Update data from source to destination
		if err := r.Update(ctx, destinationConfigMap); err != nil {
			return err
		}
	}
	// set owner labels to destination configmap
	destinationConfigMap.SetLabels(map[string]string{
		labelKey: fmt.Sprintf("%v.%v", req.Namespace, req.Name),
	})
	if err := r.Update(ctx, destinationConfigMap); err != nil {
		return err
	}
	return nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the ConfigMapSync object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *ConfigMapSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Get the ConfigMapSync object
	configMapSync := &appsv1.ConfigMapSync{}
	if err := r.Get(ctx, req.NamespacedName, configMapSync); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	allNamespaces, err := r.getAllNamespaces(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	destinationNamespaces, err := utils.GetReplicateNamespaces(&allNamespaces, configMapSync.Spec.DestinationNamespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// List all configmaps that are controlled by this configmapsync object
	var controlledConfigMaps kcorev1.ConfigMapList
	labels, err := labels.Parse(fmt.Sprintf("%v=%v.%v", labelKey, req.Namespace, req.Name))
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.List(ctx, &controlledConfigMaps, &client.ListOptions{
		LabelSelector: labels,
	}); err != nil {
		return ctrl.Result{}, err
	}

	// Get source configmap
	sourceConfigMap := &kcorev1.ConfigMap{}
	sourceConfigMapName := types.NamespacedName{
		Namespace: configMapSync.Spec.SourceNamespace,
		Name:      configMapSync.Spec.ConfigMapName,
	}
	if err := r.Get(ctx, sourceConfigMapName, sourceConfigMap); err != nil {
		return ctrl.Result{}, err
	}
	// Set owner labels to source configmap
	sourceConfigMap.SetLabels(map[string]string{
		labelKey: fmt.Sprintf("%v.%v", req.Namespace, req.Name),
	})
	if err := r.Update(ctx, sourceConfigMap); err != nil {
		return ctrl.Result{}, err
	}

	// delete all uncontrolled configmaps
	// this can occur when the ConfigMapSync.Spec.destinationNamespace is patched
	for _, controlledConfigMap := range controlledConfigMaps.Items {
		if controlledConfigMap.Namespace != configMapSync.Spec.SourceNamespace && !utils.NameInStringArray(controlledConfigMap.Namespace, destinationNamespaces) {
			if err := r.Delete(ctx, &controlledConfigMap); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// replicate source configmap to destination namespace
	for _, destinationNamespace := range destinationNamespaces {
		if destinationNamespace != configMapSync.Spec.SourceNamespace {
			destinationConfigMapName := types.NamespacedName{
				Namespace: destinationNamespace,
				Name:      configMapSync.Spec.ConfigMapName,
			}
			err := r.upsertDestinationConfigMap(ctx, req, destinationConfigMapName, sourceConfigMap, configMapSync)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigMapSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.ConfigMapSync{}).
		Owns(&kcorev1.ConfigMap{}).
		Watches(
			&kcorev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.mapDestinationConfigMapsToReconcile)).
		Watches(
			&kcorev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(r.mapNamespaceToReconcile)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Complete(r)
}

func (r *ConfigMapSyncReconciler) mapDestinationConfigMapsToReconcile(ctx context.Context, object client.Object) []reconcile.Request {
	configmap := object.(*kcorev1.ConfigMap)
	val, ok := configmap.Labels[labelKey]
	if !ok {
		return nil
	}

	configMapSyncNamespace, configMapSyncName := strings.Split(val, ".")[0], strings.Split(val, ".")[1]

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{Namespace: configMapSyncNamespace, Name: configMapSyncName},
		},
	}
}

// trigger reconcile on CUD of namespaces
// for instance, syncing configmaps to new namespaces if destination namespace regex matches
func (r *ConfigMapSyncReconciler) mapNamespaceToReconcile(ctx context.Context, object client.Object) []reconcile.Request {
	log := log.FromContext(ctx)

	// Get the ConfigMapSync object
	var configMapSyncList appsv1.ConfigMapSyncList
	if err := r.List(ctx, &configMapSyncList, &client.ListOptions{}); err != nil {
		log.Error(err, "error listing all configMapSyncs")
		return nil
	}
	var reconcileRequests []reconcile.Request

	for _, configMapSync := range configMapSyncList.Items {
		reconcileRequests = append(reconcileRequests, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: configMapSync.Namespace, Name: configMapSync.Name},
		})
	}
	return reconcileRequests
}
