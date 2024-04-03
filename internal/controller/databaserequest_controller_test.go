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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	crdv1alpha1 "github.com/uselagoon/dbaas-controller/api/v1alpha1"
	"github.com/uselagoon/dbaas-controller/internal/database/mysql"
)

var _ = Describe("DatabaseRequest Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			dbRequestResource             = "dbr-database-request-test-resource"
			dbMySQLProviderSecretResource = "dbr-database-request-test-secret"
			dbMySQLProviderResource       = "dbr-database-mysql-provider-test-resource"
		)
		ctx := context.Background()

		databaserequest := &crdv1alpha1.DatabaseRequest{}
		databaseMysqlProvider := &crdv1alpha1.DatabaseMySQLProvider{}
		databaseMysqlProviderSecret := &v1.Secret{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind DatabaseMySQLProvider")
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      dbMySQLProviderSecretResource,
				Namespace: "default",
			}, databaseMysqlProviderSecret)
			if err != nil && errors.IsNotFound(err) {
				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbMySQLProviderSecretResource,
						Namespace: "default",
					},
					StringData: map[string]string{
						"password": "test-password",
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: dbMySQLProviderResource,
			}, databaseMysqlProvider)
			if err != nil && errors.IsNotFound(err) {
				resource := &crdv1alpha1.DatabaseMySQLProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name: dbMySQLProviderResource,
					},
					Spec: crdv1alpha1.DatabaseMySQLProviderSpec{
						Scope: "development",
						MySQLConnections: []crdv1alpha1.MySQLConnection{
							{
								Name:     "test-connection",
								Hostname: "test-hostname",
								PasswordSecretRef: v1.SecretReference{
									Name:      dbMySQLProviderSecretResource,
									Namespace: "default",
								},
								Username: "test-username",
								Port:     3306,
								Enabled:  true,
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("creating the custom resource for the Kind DatabaseRequest")
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      dbRequestResource,
				Namespace: "default",
			}, databaserequest)
			if err != nil && errors.IsNotFound(err) {
				resource := &crdv1alpha1.DatabaseRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbRequestResource,
						Namespace: "default",
					},
					Spec: crdv1alpha1.DatabaseRequestSpec{
						Scope:                "development",
						Type:                 "mysql",
						DropDatabaseOnDelete: false,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance DatabaseMySQLProvider")
			databaseMysqlProvider = &crdv1alpha1.DatabaseMySQLProvider{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name: dbMySQLProviderResource,
			}, databaseMysqlProvider)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, databaseMysqlProvider)).To(Succeed())

			By("Cleanup the specific resource instance Secret")
			databaseMysqlProviderSecret = &v1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      dbMySQLProviderSecretResource,
				Namespace: "default",
			}, databaseMysqlProviderSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, databaseMysqlProviderSecret)).To(Succeed())

			By("Cleanup the specific resource instance DatabaseRequest")
			databaserequest := &crdv1alpha1.DatabaseRequest{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      dbRequestResource,
				Namespace: "default",
			}, databaserequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, databaserequest)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			fakeRecoder := record.NewFakeRecorder(100)
			controllerReconciler := &DatabaseRequestReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				Recorder:    fakeRecoder,
				MySQLClient: &mysql.MySQLMock{},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      dbRequestResource,
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking state of secret and service")
			secret := &v1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      dbRequestResource,
				Namespace: "default",
			}, secret)
			Expect(err).NotTo(HaveOccurred())

			// use label selector to get the service
			serviceList := &v1.ServiceList{}
			err = k8sClient.List(ctx, serviceList, &client.ListOptions{
				Namespace: "default",
				LabelSelector: labels.SelectorFromSet(
					map[string]string{
						"app.kubernetes.io/instance": dbRequestResource,
					},
				),
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
