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
	log := log.FromContext(ctx)

	// Get the ConfigMapSync object
	configMapSync := &appsv1.ConfigMapSync{}
	if err := r.Get(ctx, req.NamespacedName, configMapSync); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
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
	// Set owner reference to source configmap
	sourceConfigMap.SetLabels(map[string]string{
		labelKey: fmt.Sprintf("%v.%v", req.Namespace, req.Name),
	})
	if err := r.Update(ctx, sourceConfigMap); err != nil {
		return ctrl.Result{}, err
	}

	for _, controlledConfigMap := range controlledConfigMaps.Items {
		if controlledConfigMap.Namespace != configMapSync.Spec.SourceNamespace && controlledConfigMap.Namespace != configMapSync.Spec.DestinationNamespace {
			// delete uncontrolled namespace
			if err := r.Delete(ctx, &controlledConfigMap); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// replicate source configmap to destination namespace
	destinationConfigMap := &kcorev1.ConfigMap{}
	destinationConfigMapName := types.NamespacedName{
		Namespace: configMapSync.Spec.DestinationNamespace,
		Name:      configMapSync.Spec.ConfigMapName,
	}
	if err := r.Get(ctx, destinationConfigMapName, destinationConfigMap); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating ConfigMap in destination namespace", "Namespace", configMapSync.Spec.DestinationNamespace)
			destinationConfigMap = &kcorev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapSync.Spec.ConfigMapName,
					Namespace: configMapSync.Spec.DestinationNamespace,
				},
				Data: sourceConfigMap.Data, // Copy data from source to destination
			}
			if err := r.Create(ctx, destinationConfigMap); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, err
		}
	} else {
		log.Info("Updating ConfigMap in destination namespace", "Namespace", configMapSync.Spec.DestinationNamespace)
		destinationConfigMap.Data = sourceConfigMap.Data // Update data from source to destination
		if err := r.Update(ctx, destinationConfigMap); err != nil {
			return ctrl.Result{}, err
		}
	}
	// set owner reference to destination configmap
	destinationConfigMap.SetLabels(map[string]string{
		labelKey: fmt.Sprintf("%v.%v", req.Namespace, req.Name),
	})
	if err := r.Update(ctx, destinationConfigMap); err != nil {
		return ctrl.Result{}, err
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
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Complete(r)
}

func (r *ConfigMapSyncReconciler) mapDestinationConfigMapsToReconcile(ctx context.Context, object client.Object) []reconcile.Request {
	configmap := object.(*kcorev1.ConfigMap)
	val, ok := configmap.Labels[labelKey]
	if !ok {
		return nil
	}

	configmapsyncNamespace, configmapysyncName := strings.Split(val, ".")[0], strings.Split(val, ".")[1]

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{Namespace: configmapsyncNamespace, Name: configmapysyncName},
		},
	}
}
