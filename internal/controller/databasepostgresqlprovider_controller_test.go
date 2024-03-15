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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	crdv1alpha1 "github.com/uselagoon/dbaas-controller/api/v1alpha1"
)

var _ = Describe("DatabasePostgreSQLProvider Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		databasepostgresqlprovider := &crdv1alpha1.DatabasePostgreSQLProvider{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind DatabasePostgreSQLProvider")
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret-postgresql",
					Namespace: "default",
				},
				StringData: map[string]string{
					"password": "test-password",
				},
			}
			err := k8sClient.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, typeNamespacedName, databasepostgresqlprovider)
			if err != nil && errors.IsNotFound(err) {
				resource := &crdv1alpha1.DatabasePostgreSQLProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: crdv1alpha1.DatabasePostgreSQLProviderSpec{
						Scope: "custom",
						PostgreSQLConnections: []crdv1alpha1.PostgreSQLConnection{
							{
								Name:     "test-connection",
								Hostname: "test-hostname",
								PasswordSecretRef: v1.ObjectReference{
									Kind:            secret.Kind,
									Name:            secret.Name,
									Namespace:       secret.Namespace,
									APIVersion:      secret.APIVersion,
									ResourceVersion: secret.ResourceVersion,
								},
								Port: 5432,
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &crdv1alpha1.DatabasePostgreSQLProvider{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			secret := &v1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resource.Spec.PostgreSQLConnections[0].PasswordSecretRef.Name,
				Namespace: resource.Spec.PostgreSQLConnections[0].PasswordSecretRef.Namespace,
			}, secret)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance DatabasePostgreSQLProvider")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DatabasePostgreSQLProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
