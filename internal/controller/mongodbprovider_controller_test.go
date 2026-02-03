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
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	crdv1alpha1 "github.com/uselagoon/dbaas-controller/api/v1alpha1"
	"github.com/uselagoon/dbaas-controller/internal/database/mongodb"
)

var _ = Describe("MongoDBProvider Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		mongodbprovider := &crdv1alpha1.MongoDBProvider{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind MongoDBProvider")
			secret := &v1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-mongo-db-provider-secret",
				Namespace: "default",
			}, secret)
			if err != nil && errors.IsNotFound(err) {
				secret = &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-mongo-db-provider-secret",
						Namespace: "default",
					},
					StringData: map[string]string{
						"password": "test-password",
					},
				}
				err = k8sClient.Create(ctx, secret)
				Expect(err).NotTo(HaveOccurred())
			}

			err = k8sClient.Get(ctx, typeNamespacedName, mongodbprovider)
			if err != nil && errors.IsNotFound(err) {
				resource := &crdv1alpha1.MongoDBProvider{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: crdv1alpha1.MongoDBProviderSpec{
						Selector: "development",
						Connections: []crdv1alpha1.MongoDBConnection{
							{
								Name: "test-mongodb-connection",
								Auth: crdv1alpha1.MongoDBAuth{
									Mechanism: "SCRAM-SHA-256",
								},
								PasswordSecretRef: v1.SecretReference{
									Name:      secret.Name,
									Namespace: secret.Namespace,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &crdv1alpha1.MongoDBProvider{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance MongoDBProvider")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			fakeRecorder := events.NewFakeRecorder(100)
			controllerReconciler := &MongoDBProviderReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				Recorder:      fakeRecorder,
				MongoDBClient: &mongodb.MongoDBMock{},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			Expect(err).NotTo(HaveOccurred())
			// fetch the resource to check if the status has been updated
			err = k8sClient.Get(ctx, typeNamespacedName, mongodbprovider)
			Expect(err).NotTo(HaveOccurred())
			// check the port of the connection is having the default value
			Expect(mongodbprovider.Spec.Connections[0].Port).To(Equal(27017))
			Expect(mongodbprovider.Status.ObservedGeneration).To(Equal(mongodbprovider.Generation))

			// check status of the resource
			Expect(mongodbprovider.Status.ConnectionStatus).To(HaveLen(1))
			Expect(mongodbprovider.Status.ConnectionStatus[0].Name).To(Equal("test-mongodb-connection"))

			// check the conditions
			err = k8sClient.Get(ctx, typeNamespacedName, mongodbprovider)
			Expect(err).NotTo(HaveOccurred())
			Expect(mongodbprovider.Status.Conditions).To(HaveLen(1))
			Expect(mongodbprovider.Status.Conditions[0].Type).To(Equal("Ready"))

		})
	})
})
