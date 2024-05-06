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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	crdv1alpha1 "github.com/uselagoon/dbaas-controller/api/v1alpha1"
	"github.com/uselagoon/dbaas-controller/internal/database"
)

var _ = Describe("RelationalDatabaseProvider Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "relational-database-provider-test"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		relationaldatabaseprovider := &crdv1alpha1.RelationalDatabaseProvider{}

		BeforeEach(func() {
			By("creating the custom resource for the RelationalDatabaseProvider")
			secret := &v1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-rel-db-provider-secret",
				Namespace: "default",
			}, secret)
			if err != nil && errors.IsNotFound(err) {
				secret = &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rel-db-provider-secret",
						Namespace: "default",
					},
					StringData: map[string]string{
						"password": "test-password",
					},
				}
				err = k8sClient.Create(ctx, secret)
				Expect(err).NotTo(HaveOccurred())
			}

			err = k8sClient.Get(ctx, typeNamespacedName, relationaldatabaseprovider)
			if err != nil && errors.IsNotFound(err) {
				resource := &crdv1alpha1.RelationalDatabaseProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: crdv1alpha1.RelationalDatabaseProviderSpec{
						Type:  "mysql",
						Scope: "custom",
						Connections: []crdv1alpha1.Connection{
							{
								Name:     "test-connection",
								Hostname: "test-hostname",
								PasswordSecretRef: v1.SecretReference{
									Name:      secret.Name,
									Namespace: secret.Namespace,
								},
								Port:     3306,
								Username: "test-username",
								Enabled:  true,
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &crdv1alpha1.RelationalDatabaseProvider{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			secret := &v1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resource.Spec.Connections[0].PasswordSecretRef.Name,
				Namespace: resource.Spec.Connections[0].PasswordSecretRef.Namespace,
			}, secret)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RelationalDatabaseProvider")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			fakeRecorder := record.NewFakeRecorder(100)
			controllerReconciler := &RelationalDatabaseProviderReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				Recorder:    fakeRecorder,
				RelDBClient: &database.RelationalDatabaseMock{},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// check status of the resource
			err = k8sClient.Get(ctx, typeNamespacedName, relationaldatabaseprovider)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(relationaldatabaseprovider.Status.Conditions)).To(Equal(1))
			Expect(relationaldatabaseprovider.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(relationaldatabaseprovider.Status.Conditions[0].Type).To(Equal("Ready"))
		})
	})
})
