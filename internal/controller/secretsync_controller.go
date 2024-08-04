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

	kcorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

// SecretSyncReconciler reconciles a SecretSync object
type SecretSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=apps.replicator,resources=secretsyncs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps.replicator,resources=secretsyncs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps.replicator,resources=secretsyncs/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets/finalizers,verbs=update

func (r *SecretSyncReconciler) getAllNamespaces(ctx context.Context) (kcorev1.NamespaceList, error) {
	var allNamespaces kcorev1.NamespaceList
	if err := r.List(ctx, &allNamespaces, &client.ListOptions{}); err != nil {
		return allNamespaces, err
	}

	return allNamespaces, nil
}

func (r *SecretSyncReconciler) upsertDestinationSecret(
	ctx context.Context,
	req ctrl.Request,
	destinationSecretName types.NamespacedName,
	sourceSecret *kcorev1.Secret,
	secretSync *appsv1.SecretSync,
) error {
	log := log.FromContext(ctx)
	destinationSecret := &kcorev1.Secret{}
	if err := r.Get(ctx, destinationSecretName, destinationSecret); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Secret in destination namespace", "Namespace", destinationSecretName.Namespace)
			destinationSecret = &kcorev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretSync.Spec.SecretName,
					Namespace: destinationSecretName.Namespace,
				},
				Data: sourceSecret.Data, // Copy data from source to destination
			}
			if err := r.Create(ctx, destinationSecret); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		log.Info("Updating Secret in destination namespace", "Namespace", destinationSecretName.Namespace)
		destinationSecret.Data = sourceSecret.Data // Update data from source to destination
		if err := r.Update(ctx, destinationSecret); err != nil {
			return err
		}
	}
	// set owner labels to destination secret
	destinationSecret.SetLabels(map[string]string{
		labelKey: fmt.Sprintf("%v.%v", req.Namespace, req.Name),
	})
	if err := r.Update(ctx, destinationSecret); err != nil {
		return err
	}
	return nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the SecretSync object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *SecretSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Get the SecretSync object
	secretSync := &appsv1.SecretSync{}
	if err := r.Get(ctx, req.NamespacedName, secretSync); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	allNamespaces, err := r.getAllNamespaces(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	destinationNamespaces, err := utils.GetReplicateNamespaces(&allNamespaces, secretSync.Spec.DestinationNamespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// List all secrets that are controlled by this secretSync object
	var controlledSecrets kcorev1.SecretList
	labels, err := labels.Parse(fmt.Sprintf("%v=%v.%v", labelKey, req.Namespace, req.Name))
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.List(ctx, &controlledSecrets, &client.ListOptions{
		LabelSelector: labels,
	}); err != nil {
		return ctrl.Result{}, err
	}

	// Get source secret
	sourceSecret := &kcorev1.Secret{}
	sourceSecretName := types.NamespacedName{
		Namespace: secretSync.Spec.SourceNamespace,
		Name:      secretSync.Spec.SecretName,
	}
	if err := r.Get(ctx, sourceSecretName, sourceSecret); err != nil {
		return ctrl.Result{}, err
	}
	// Set owner labels to source secret
	sourceSecret.SetLabels(map[string]string{
		labelKey: fmt.Sprintf("%v.%v", req.Namespace, req.Name),
	})
	if err := r.Update(ctx, sourceSecret); err != nil {
		return ctrl.Result{}, err
	}

	// delete all uncontrolled secrets
	// this can occur when the SecretSync.Spec.destinationNamespace is patched
	for _, controlledSecret := range controlledSecrets.Items {
		if controlledSecret.Namespace != secretSync.Spec.SourceNamespace && !utils.NameInStringArray(controlledSecret.Namespace, destinationNamespaces) {
			if err := r.Delete(ctx, &controlledSecret); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// replicate source secret to destination namespace
	for _, destinationNamespace := range destinationNamespaces {
		if destinationNamespace != secretSync.Spec.SourceNamespace {
			destinationSecretName := types.NamespacedName{
				Namespace: destinationNamespace,
				Name:      secretSync.Spec.SecretName,
			}
			err := r.upsertDestinationSecret(ctx, req, destinationSecretName, sourceSecret, secretSync)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.SecretSync{}).
		Owns(&kcorev1.Secret{}).
		Watches(
			&kcorev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.mapDestinationSecretsToReconcile)).
		Watches(
			&kcorev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(r.mapNamespaceToReconcile)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Complete(r)
}

func (r *SecretSyncReconciler) mapDestinationSecretsToReconcile(ctx context.Context, object client.Object) []reconcile.Request {
	secret := object.(*kcorev1.Secret)
	val, ok := secret.Labels[labelKey]
	if !ok {
		return nil
	}

	secretSyncNamespace, secretSyncName := strings.Split(val, ".")[0], strings.Split(val, ".")[1]

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{Namespace: secretSyncNamespace, Name: secretSyncName},
		},
	}
}

// trigger reconcile on CUD of namespaces
// for instance, syncing secrets to new namespaces if destination namespace regex matches
func (r *SecretSyncReconciler) mapNamespaceToReconcile(ctx context.Context, object client.Object) []reconcile.Request {
	log := log.FromContext(ctx)

	// Get the SecretSync object
	var secretSyncList appsv1.SecretSyncList
	if err := r.List(ctx, &secretSyncList, &client.ListOptions{}); err != nil {
		log.Error(err, "error listing all secretSyncs")
		return nil
	}
	var reconcileRequests []reconcile.Request

	for _, secretSync := range secretSyncList.Items {
		reconcileRequests = append(reconcileRequests, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: secretSync.Namespace, Name: secretSync.Name},
		})
	}
	return reconcileRequests
}
