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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "github.com/jason-cky/resource-replicator-operator/api/v1"
	kcorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ConfigMapSync Controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		ConfigMapSyncName      = "test-configmapsync"
		ConfigMapSyncNamespace = "source"
		ConfigMapName          = "test-cm"

		SourceNamespace      = "source"
		DestinationNamespace = "test"
	)
	configmapData := map[string]string{
		"key": "value",
		"foo": "bar",
	}
	Context("When reconciling a resource", func() {
		ctx := context.Background()

		// setup: create ns (default and test)
		sourceNamespace := &kcorev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: SourceNamespace}}
		destinationNamespace := &kcorev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: DestinationNamespace}}

		// setup: create configmap with test data
		configmap := &kcorev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConfigMapName,
				Namespace: SourceNamespace,
			},
			Data: configmapData,
		}

		typeNamespacedName := types.NamespacedName{
			Name:      ConfigMapSyncName,
			Namespace: ConfigMapSyncNamespace, // (user):Modify as needed
		}
		configmapsync := &appsv1.ConfigMapSync{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ConfigMapSync")
			err := k8sClient.Get(ctx, types.NamespacedName{Name: SourceNamespace}, sourceNamespace)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, sourceNamespace)).To(Succeed())
			}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: DestinationNamespace}, destinationNamespace)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, destinationNamespace)).To(Succeed())
			}
			Expect(k8sClient.Create(ctx, configmap)).To(Succeed())
			err = k8sClient.Get(ctx, typeNamespacedName, configmapsync)
			if err != nil && errors.IsNotFound(err) {
				resource := &appsv1.ConfigMapSync{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ConfigMapSyncName,
						Namespace: ConfigMapSyncNamespace,
					},
					// (user): Specify other spec details if needed.
					Spec: appsv1.ConfigMapSyncSpec{
						SourceNamespace:      SourceNamespace,
						DestinationNamespace: DestinationNamespace,
						ConfigMapName:        ConfigMapName,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// (user): Cleanup logic after each test, like removing the resource instance.
			resource := &appsv1.ConfigMapSync{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ConfigMapSync")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ConfigMapSyncReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// (user): Add more specific assertions depending on your controller's reconciliation logic.
			// assert that configmap is created in the destination namespace
			replicatedConfigMap := &kcorev1.ConfigMap{}
			err = k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      ConfigMapName,
					Namespace: DestinationNamespace, // (user):Modify as needed
				},
				replicatedConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(replicatedConfigMap.Data).Should(Equal(configmapData))
		})
	})
})
