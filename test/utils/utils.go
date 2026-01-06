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

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo/v2"
)

const (
	prometheusOperatorVersion = "v0.68.0"
	prometheusOperatorURL     = "https://github.com/prometheus-operator/prometheus-operator/" +
		"releases/download/%s/bundle.yaml"

	certmanagerVersion = "v1.5.3"
	certmanagerURLTmpl = "https://github.com/jetstack/cert-manager/releases/download/%s/cert-manager.yaml"

	mysqlYaml    = "test/e2e/testdata/mysql.yaml"
	postgresYaml = "test/e2e/testdata/postgres.yaml"
)

var kubectlPath, kindPath string

func init() {
	if v, ok := os.LookupEnv("KIND_PATH"); ok {
		kindPath = v
	} else {
		kindPath = "kind"
	}
	if v, ok := os.LookupEnv("KUBECTL_PATH"); ok {
		kubectlPath = v
	} else {
		kubectlPath = "kubectl"
	}
	fmt.Println(kubectlPath, kindPath)
}

func Kubectl() string {
	return kubectlPath
}

func warnError(err error) {
	fmt.Fprintf(ginkgo.GinkgoWriter, "warning: %v\n", err)
}

// InstallRelationalDatabases installs both MySQL and PostgreSQL pods to be used for testing.
func InstallRelationalDatabases() error {
	dir, err := GetProjectDir()
	if err != nil {
		return err
	}
	errChan := make(chan error, 2)
	for _, yaml := range []string{mysqlYaml, postgresYaml} {
		cmd := exec.Command(kubectlPath, "apply", "-f", yaml)
		cmd.Dir = dir
		fmt.Fprintf(ginkgo.GinkgoWriter, "running: %s in directory: %s\n", strings.Join(cmd.Args, " "), dir)
		go func() {
			_, err := Run(cmd)
			errChan <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			return err
		}
	}
	return nil
}

// InstallMongoDB installs a MongoDB pod to be used for testing.
func InstallMongoDB() error {
	dir, err := GetProjectDir()
	if err != nil {
		return err
	}
	cmd := exec.Command(kubectlPath, "apply", "-f", "test/e2e/testdata/mongodb.yaml")
	cmd.Dir = dir
	fmt.Fprintf(ginkgo.GinkgoWriter, "running: %s in directory: %s\n", strings.Join(cmd.Args, " "), dir)
	_, err = Run(cmd)
	return err
}

// UninstallRelationalDatabases uninstalls both MySQL and PostgreSQL pods.
func UninstallRelationalDatabases() {
	dir, err := GetProjectDir()
	if err != nil {
		warnError(err)
	}
	errChan := make(chan error, 2)
	for _, yaml := range []string{mysqlYaml, postgresYaml} {
		cmd := exec.Command(kubectlPath, "delete", "-f", yaml)
		cmd.Dir = dir
		fmt.Fprintf(ginkgo.GinkgoWriter, "running: %s in directory: %s\n", strings.Join(cmd.Args, " "), dir)
		go func() {
			_, err := Run(cmd)
			errChan <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			warnError(err)
		}
	}
}

// UninstallMongoDB uninstalls the MongoDB pod.
func UninstallMongoDB() {
	dir, err := GetProjectDir()
	if err != nil {
		warnError(err)
	}
	cmd := exec.Command(kubectlPath, "delete", "-f", "test/e2e/testdata/mongodb.yaml")
	cmd.Dir = dir
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// InstallMySQL installs a MySQL pod to be used for testing.
func InstallMySQL() error {
	dir, err := GetProjectDir()
	if err != nil {
		return err
	}
	cmd := exec.Command(kubectlPath, "apply", "-f", mysqlYaml)
	cmd.Dir = dir
	fmt.Fprintf(ginkgo.GinkgoWriter, "running: %s in directory: %s\n", strings.Join(cmd.Args, " "), dir)
	_, err = Run(cmd)
	// Note that we don't wait for the pod to be ready here. This is a good test for the controller
	// to see if it can handle mysql server not being ready.
	return err
}

// UninstallMySQLPod uninstalls the MySQL pod.
func UninstallMySQLPod() {
	dir, err := GetProjectDir()
	if err != nil {
		warnError(err)
	}
	cmd := exec.Command(kubectlPath, "delete", "-f", mysqlYaml)
	cmd.Dir = dir
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// InstallPrometheusOperator installs the prometheus Operator to be used to export the enabled metrics.
func InstallPrometheusOperator() error {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command(kubectlPath, "create", "-f", url)
	_, err := Run(cmd)
	return err
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) ([]byte, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "chdir dir: %s\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	fmt.Fprintf(ginkgo.GinkgoWriter, "running: %s\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s failed with error: (%v) %s", command, err, string(output))
	}

	return output, nil
}

// UninstallPrometheusOperator uninstalls the prometheus
func UninstallPrometheusOperator() {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command(kubectlPath, "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// UninstallCertManager uninstalls the cert manager
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command(kubectlPath, "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// InstallCertManager installs the cert manager bundle.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command(kubectlPath, "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}
	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command(kubectlPath, "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)

	_, err := Run(cmd)
	return err
}

// LoadImageToKindCluster loads a local docker image to the kind cluster
func LoadImageToKindClusterWithName(name string) error {
	cluster := kindPath
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindOptions := []string{"load", "docker-image", name, "--name", cluster}
	cmd := exec.Command(kindPath, kindOptions...)
	_, err := Run(cmd)
	return err
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, err
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}
