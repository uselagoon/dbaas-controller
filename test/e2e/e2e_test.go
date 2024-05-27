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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/uselagoon/dbaas-controller/test/utils"
)

const (
	namespace = "dbaas-controller-system"
	timetout  = "500s"
)

var _ = Describe("controller", Ordered, func() {
	BeforeAll(func() {
		By("installing prometheus operator")
		Expect(utils.InstallPrometheusOperator()).To(Succeed())

		By("installing the cert-manager")
		Expect(utils.InstallCertManager()).To(Succeed())

		By("installing MySQL pod")
		Expect(utils.InstallMySQL()).To(Succeed())

		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	AfterAll(func() {
		By("uninstalling the Prometheus manager bundle")
		utils.UninstallPrometheusOperator()

		By("uninstalling the cert-manager bundle")
		utils.UninstallCertManager()

		By("removing the DatabaseMySQLProvider resource")
		// we enforce the deletion by removing the finalizer
		cmd := exec.Command(
			"kubectl",
			"patch",
			"databasemysqlprovider",
			"databasemysqlprovider-sample",
			"-p",
			`{"metadata":{"finalizers":[]}}`,
			"--type=merge",
		)
		_, _ = utils.Run(cmd)

		cmd = exec.Command("kubectl", "delete", "--force", "databasemysqlprovider", "databasemysqlprovider-sample")
		_, _ = utils.Run(cmd)

		By("removing the DatabaseRequest resource")
		for _, name := range []string{"databaserequest-sample", "seed-databaserequest-sample"} {
			// we enforce the deletion by removing the finalizer
			cmd = exec.Command(
				"kubectl",
				"patch",
				"databaserequest",
				name,
				"-p",
				`{"metadata":{"finalizers":[]}}`,
				"--type=merge",
			)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "--force", "databaserequest", name)
			_, _ = utils.Run(cmd)

			By("removing service and secret")
			cmd = exec.Command(
				"kubectl", "delete", "service", "-n", "default", "-l", fmt.Sprintf("app.kubernetes.io/instance=%s", name))
			_, _ = utils.Run(cmd)
			cmd = exec.Command(
				"kubectl", "delete", "secret", "-n", "default", "-l", fmt.Sprintf("app.kubernetes.io/instance=%s", name))
			_, _ = utils.Run(cmd)
		}
		cmd = exec.Command(
			"kubectl", "delete", "secret", "-n", "default", "seed-mysql-secret")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)

		By("uninstalling MySQL pod")
		utils.UninstallMySQLPod()

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

				cmd = exec.Command("kubectl", "get",
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
				cmd = exec.Command("kubectl", "get",
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

			By("creating a DatabaseMySQLProvider resource")
			cmd = exec.Command("kubectl", "apply", "-f", "config/samples/crd_v1alpha1_databasemysqlprovider.yaml")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("validating that the DatabaseMySQLProvider resource is created")
			cmd = exec.Command(
				"kubectl",
				"wait",
				"--for=condition=Ready",
				"databasemysqlprovider",
				"databasemysqlprovider-sample",
				fmt.Sprintf("--timeout=%s", timetout),
			)
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("creating a DatabaseRequest resource")
			cmd = exec.Command("kubectl", "apply", "-f", "config/samples/crd_v1alpha1_databaserequest.yaml")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("validating that the DatabaseRequest resource is created")
			cmd = exec.Command(
				"kubectl",
				"wait",
				"--for=condition=Ready",
				"databaserequest",
				"databaserequest-sample",
				fmt.Sprintf("--timeout=%s", timetout),
			)
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			// verify that the service and secret got created
			By("validating that the service is created")
			cmd = exec.Command(
				"kubectl",
				"get",
				"service",
				"-n", "default",
				"-l", "app.kubernetes.io/instance=databaserequest-sample",
			)
			serviceOutput, err := utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			serviceNames := utils.GetNonEmptyLines(string(serviceOutput))
			ExpectWithOffset(1, serviceNames).Should(HaveLen(2))
			ExpectWithOffset(1, serviceNames[1]).Should(ContainSubstring("first-mysql-db"))

			By("validating that the secret is created")
			cmd = exec.Command(
				"kubectl",
				"get",
				"secret",
				"-n", "default",
				"-l", "app.kubernetes.io/instance=databaserequest-sample",
			)
			secretOutput, err := utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			secretNames := utils.GetNonEmptyLines(string(secretOutput))
			ExpectWithOffset(1, secretNames).Should(HaveLen(2))

			By("deleting the DatabaseRequest resource the database is getting deprovisioned")
			cmd = exec.Command("kubectl", "delete", "databaserequest", "databaserequest-sample")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("validating that the service is deleted")
			cmd = exec.Command(
				"kubectl",
				"get",
				"service",
				"-n", "default",
				"-l", "app.kubernetes.io/instance=databaserequest-sample",
			)
			serviceOutput, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			serviceNames = utils.GetNonEmptyLines(string(serviceOutput))
			ExpectWithOffset(1, serviceNames).Should(HaveLen(1))

			By("validating that the secret is deleted")
			cmd = exec.Command(
				"kubectl",
				"get",
				"secret",
				"-n", "default",
				"-l", "app.kubernetes.io/instance=databaserequest-sample",
			)
			secretOutput, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			secretNames = utils.GetNonEmptyLines(string(secretOutput))
			ExpectWithOffset(1, secretNames).Should(HaveLen(1))

			By("creating seed secret for the DatabaseRequest resource")
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/seed-secret.yaml")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("creating mysql pod to create seed credentials")
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/testdata/mysql-client-pod.yaml")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("creating a seed DatabaseRequest resource")
			cmd = exec.Command("kubectl", "apply", "-f", "config/samples/crd_v1alpha1_seed_databaserequest.yaml")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("validating that the DatabaseRequest resource is created")
			cmd = exec.Command(
				"kubectl",
				"wait",
				"--for=condition=Ready",
				"databaserequest",
				"seed-databaserequest-sample",
				fmt.Sprintf("--timeout=%s", timetout),
			)
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			// verify that the service and secret got created
			By("validating that the service is created")
			cmd = exec.Command(
				"kubectl",
				"get",
				"service",
				"-n", "default",
				"-l", "app.kubernetes.io/instance=seed-databaserequest-sample",
			)
			serviceOutput, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			serviceNames = utils.GetNonEmptyLines(string(serviceOutput))
			ExpectWithOffset(1, serviceNames).Should(HaveLen(2))
			ExpectWithOffset(1, serviceNames[1]).Should(ContainSubstring("seed-mysql-db"))

			By("validating that the secret is created")
			cmd = exec.Command(
				"kubectl",
				"get",
				"secret",
				"-n", "default",
				"-l", "app.kubernetes.io/instance=seed-databaserequest-sample",
			)
			secretOutput, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			secretNames = utils.GetNonEmptyLines(string(secretOutput))
			ExpectWithOffset(1, secretNames).Should(HaveLen(2))

			// uncomment to debug ...
			//time.Sleep(15 * time.Minute)
		})
	})
})
