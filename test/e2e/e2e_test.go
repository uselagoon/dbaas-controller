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

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/uselagoon/dbaas-controller/test/utils"
)

const (
	namespace = "dbaas-controller-system"
	timeout   = "300s"
)

var _ = Describe("controller", Ordered, func() {
	BeforeAll(func() {
		By("installing prometheus operator")
		Expect(utils.InstallPrometheusOperator()).To(Succeed())

		By("installing the cert-manager")
		Expect(utils.InstallCertManager()).To(Succeed())

		By("installing relational databases pods")
		Expect(utils.InstallRelationalDatabases()).To(Succeed())

		By("installing mongodb pods")
		Expect(utils.InstallMongoDB()).To(Succeed())

		By("creating manager namespace")
		cmd := exec.Command(utils.Kubectl(), "create", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	AfterAll(func() {
		By("uninstalling the Prometheus manager bundle")
		utils.UninstallPrometheusOperator()

		By("uninstalling the cert-manager bundle")
		utils.UninstallCertManager()

		By("removing the RelationalDatabaseProvider resource")
		for _, name := range []string{"mysql", "mysql-scope", "postgres", "mongodb"} {
			cmd := exec.Command(
				utils.Kubectl(),
				"patch",
				"relationaldatabaseprovider",
				fmt.Sprintf("relationaldatabaseprovider-%s-sample", name),
				"-p",
				`{"metadata":{"finalizers":[]}}`,
				"--type=merge",
			)
			_, _ = utils.Run(cmd)
			cmd = exec.Command(
				utils.Kubectl(), "delete", "--force", "relationaldatabaseprovider", fmt.Sprintf(
					"relationaldatabaseprovider-%s-sample", name))
			_, _ = utils.Run(cmd)
		}
		By("removing the DatabaseRequest resource")
		for _, name := range []string{"mysql", "mysql-scope", "postgres", "seed"} {
			cmd := exec.Command(
				utils.Kubectl(),
				"patch",
				"databaserequest",
				fmt.Sprintf("databaserequest-%s-sample", name),
				"-p",
				`{"metadata":{"finalizers":[]}}`,
				"--type=merge",
			)
			_, _ = utils.Run(cmd)
			cmd = exec.Command(
				utils.Kubectl(), "delete", "--force", "databaserequest", fmt.Sprintf(
					"databaserequest-%s-sample", name))
			_, _ = utils.Run(cmd)
		}
		By("removing manager namespace")
		cmd := exec.Command(utils.Kubectl(), "delete", "ns", namespace)
		_, _ = utils.Run(cmd)

		By("uninstalling relational databases pods")
		utils.UninstallRelationalDatabases()

		By("uninstalling mongodb pods")
		utils.UninstallMongoDB()

		By("removing service and secret")
		for _, name := range []string{"mysql", "mysql-scope", "postgres", "mongodb"} {
			cmd = exec.Command(
				utils.Kubectl(), "delete", "service", "-n", "default", "-l", "app.kubernetes.io/instance=databaserequest-"+name+"-sample")
			_, _ = utils.Run(cmd)
			cmd = exec.Command(
				utils.Kubectl(), "delete", "secret", "-n", "default", "-l", "app.kubernetes.io/instance=databaserequest-"+name+"-sample")
			_, _ = utils.Run(cmd)
		}
	})

	Context("Operator", func() {
		It("should run successfully", func() {
			var controllerPodName string
			var err error

			// projectimage stores the name of the image used in the example
			var projectimage = "example.com/dbaas-controller:v0.0.1"

			By("building the manager(Operator) image")
			cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectimage))
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("loading the the manager(Operator) image on Kind")
			err = utils.LoadImageToKindClusterWithName(projectimage)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("installing CRDs")
			cmd = exec.Command("make", "install")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("deploying the controller-manager")
			cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectimage))
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func() error {
				// Get pod name

				cmd = exec.Command(utils.Kubectl(), "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				ExpectWithOffset(2, err).NotTo(HaveOccurred())
				podNames := utils.GetNonEmptyLines(string(podOutput))
				if len(podNames) != 1 {
					return fmt.Errorf("expect 1 controller pods running, but got %d", len(podNames))
				}
				controllerPodName = podNames[0]
				ExpectWithOffset(2, controllerPodName).Should(ContainSubstring("controller-manager"))

				// Validate pod status
				cmd = exec.Command(utils.Kubectl(), "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				status, err := utils.Run(cmd)
				ExpectWithOffset(2, err).NotTo(HaveOccurred())
				if string(status) != "Running" {
					return fmt.Errorf("controller pod in %s status", status)
				}
				return nil
			}
			EventuallyWithOffset(1, verifyControllerUp, time.Minute, time.Second).Should(Succeed())

			By("validating that all database providers and database requests are working")
			for _, name := range []string{"mysql", "mysql-scope", "postgres", "seed"} {
				if name != "seed" {
					By("creating a RelationalDatabaseProvider resource")
					cmd = exec.Command(
						utils.Kubectl(),
						"apply",
						"-f",
						fmt.Sprintf("config/samples/crd_v1alpha1_relationaldatabaseprovider_%s.yaml", name),
					)
					_, err = utils.Run(cmd)
					ExpectWithOffset(1, err).NotTo(HaveOccurred())

					By("validating that the RelationalDatabaseProvider resource is created")
					cmd = exec.Command(
						utils.Kubectl(),
						"wait",
						"--for=condition=Ready",
						"relationaldatabaseprovider",
						fmt.Sprintf("relationaldatabaseprovider-%s-sample", name),
						fmt.Sprintf("--timeout=%s", timeout),
					)
					_, err = utils.Run(cmd)
					ExpectWithOffset(1, err).NotTo(HaveOccurred())
				} else {
					By("creating seed secret for the DatabaseRequest resource")
					cmd = exec.Command(utils.Kubectl(), "apply", "-f", "test/e2e/testdata/seed-secret.yaml")
					_, err = utils.Run(cmd)
					ExpectWithOffset(1, err).NotTo(HaveOccurred())

					By("creating mysql pod to create seed credentials")
					cmd = exec.Command(utils.Kubectl(), "apply", "-f", "test/e2e/testdata/mysql-client-pod.yaml")
					_, err = utils.Run(cmd)
					ExpectWithOffset(1, err).NotTo(HaveOccurred())

					// We need to wait here a bit because we changed the code to not retry on specific failures in the seed.
					// As we can see this is problematic... before it would just have automatically retried and worked after the mysql
					// pod was fully up. Now we need to sleep some arbitrary time to make sure the seed database is up
					time.Sleep(10 * time.Second)
				}

				By("creating a DatabaseRequest resource")
				cmd = exec.Command(
					utils.Kubectl(),
					"-n", "default",
					"apply",
					"-f",
					fmt.Sprintf("config/samples/crd_v1alpha1_databaserequest_%s.yaml", name),
				)
				_, err = utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				By("validating that the DatabaseRequest resource is created")
				cmd = exec.Command(
					utils.Kubectl(),
					"-n", "default",
					"wait",
					"--for=condition=Ready",
					"databaserequest",
					fmt.Sprintf("databaserequest-%s-sample", name),
					fmt.Sprintf("--timeout=%s", timeout),
				)
				_, err = utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				// verify that the service and secret got created
				By("validating that the service is created")
				cmd = exec.Command(
					utils.Kubectl(),
					"get",
					"service",
					"-n", "default",
					"-l", fmt.Sprintf("app.kubernetes.io/instance=databaserequest-%s-sample", name),
				)
				serviceOutput, err := utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				serviceNames := utils.GetNonEmptyLines(string(serviceOutput))
				ExpectWithOffset(1, serviceNames).Should(HaveLen(2))
				ExpectWithOffset(1, serviceNames[1]).Should(ContainSubstring(fmt.Sprintf("first-%s-db", name)))

				By("validating that the secret is created")
				cmd = exec.Command(
					utils.Kubectl(),
					"get",
					"secret",
					"-n", "default",
					"-l", fmt.Sprintf("app.kubernetes.io/instance=databaserequest-%s-sample", name),
				)
				secretOutput, err := utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				secretNames := utils.GetNonEmptyLines(string(secretOutput))
				ExpectWithOffset(1, secretNames).Should(HaveLen(2))

				if name == "seed" {
					By("checking that the seed secret is deleted")
					cmd = exec.Command(utils.Kubectl(), "get", "secret", "seed-mysql-secret")
					_, err := utils.Run(cmd)
					// expect error to occurred
					ExpectWithOffset(1, err).To(HaveOccurred())
				}

				By("deleting the DatabaseRequest resource the database is getting deprovisioned")
				cmd = exec.Command(
					utils.Kubectl(),
					"-n", "default",
					"delete",
					"databaserequest",
					fmt.Sprintf("databaserequest-%s-sample", name),
				)
				_, err = utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				By("validating that the service is deleted")
				cmd = exec.Command(
					utils.Kubectl(),
					"get",
					"service",
					"-n", "default",
					"-l", fmt.Sprintf("app.kubernetes.io/instance=databaserequest-%s-sample", name),
				)
				serviceOutput, err = utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				serviceNames = utils.GetNonEmptyLines(string(serviceOutput))
				ExpectWithOffset(1, serviceNames).Should(HaveLen(1))

				By("validating that the secret is deleted")
				cmd = exec.Command(
					utils.Kubectl(),
					"get",
					"secret",
					"-n", "default",
					"-l", fmt.Sprintf("app.kubernetes.io/instance=databaserequest-%s-sample", name),
				)
				secretOutput, err = utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				secretNames = utils.GetNonEmptyLines(string(secretOutput))
				ExpectWithOffset(1, secretNames).Should(HaveLen(1))
			}

			By("validating that broken seed database request are failing in exptected way")
			for _, name := range []string{"credential-broken-seed", "non-existing-database-seed"} {
				By("creating seed secret for the DatabaseRequest resource")
				cmd = exec.Command(utils.Kubectl(), "apply", "-f", fmt.Sprintf("test/e2e/testdata/%s-secret.yaml", name))
				_, err = utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				By("creating a DatabaseRequest resource")
				// replace - with _
				dbrName := strings.ReplaceAll(name, "-", "_")
				cmd = exec.Command(
					utils.Kubectl(),
					"-n", "default",
					"apply",
					"-f",
					fmt.Sprintf("test/e2e/testdata/crd_v1alpha1_%s_databaserequest.yaml", dbrName),
				)
				_, err = utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				By("validating that the DatabaseRequest resource is created but fails")
				cmd = exec.Command(
					utils.Kubectl(),
					"-n", "default",
					"get",
					"databaserequest",
					fmt.Sprintf("%s-databaserequest-sample", name),
					"-o=jsonpath={.status.conditions[?(@.type=='Ready')].status}",
				)
				for i := 0; i < 3; i++ {
					output, err := utils.Run(cmd)
					if err != nil {
						ExpectWithOffset(1, err).NotTo(HaveOccurred())
					}
					if strings.TrimSpace(string(output)) == "False" {
						break
					} else if i == 2 {
						Expect(strings.TrimSpace(string(output))).To(Equal("False"))
					}
					// give it a bit of time to fail
					time.Sleep(time.Second)
				}

				// verify that the service and secret got created
				By("validating that the service is not created")
				cmd = exec.Command(
					utils.Kubectl(),
					"get",
					"service",
					"-n", "default",
					"-l", fmt.Sprintf("app.kubernetes.io/instance=%s-databaserequest-sample", name),
				)
				output, err := utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				Expect(strings.TrimSpace(string(output))).To(Equal("No resources found in default namespace."))

				By("validating that the secret is not created")
				cmd = exec.Command(
					utils.Kubectl(),
					"get",
					"secret",
					"-n", "default",
					"-l", fmt.Sprintf("app.kubernetes.io/instance=%s-databaserequest-sample", name),
				)
				serviceOutput, err := utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				Expect(strings.TrimSpace(string(serviceOutput))).To(Equal("No resources found in default namespace."))

				By("validating that the seed secret is not deleted")
				cmd = exec.Command(utils.Kubectl(), "get", "secret", fmt.Sprintf("%s-secret", name))
				_, err = utils.Run(cmd)
				// expect no error to have occurred because the secret should not get deleted
				// if the database request is not successfully created
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				By("deleting the DatabaseRequest resource the database is getting deprovisioned")
				cmd = exec.Command(
					utils.Kubectl(),
					"-n", "default",
					"delete",
					"databaserequest",
					fmt.Sprintf("%s-databaserequest-sample", name),
				)
				_, err = utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}
		})

		// uncomment to debug ...
		// time.Sleep(15 * time.Minute)

	})

})
