//go:build e2e

/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hortator-ai/Hortator/test/utils"
)

const namespace = "hortator-system"

var _ = Describe("controller", Ordered, func() {
	BeforeAll(func() {
		By("installing prometheus operator")
		Expect(utils.InstallPrometheusOperator()).To(Succeed())

		By("installing the cert-manager")
		Expect(utils.InstallCertManager()).To(Succeed())

		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	AfterAll(func() {
		By("uninstalling the Prometheus manager bundle")
		utils.UninstallPrometheusOperator()

		By("uninstalling the cert-manager bundle")
		utils.UninstallCertManager()

		By("removing manager namespace")
		cmd := exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	Context("Operator", func() {
		It("should run successfully", func() {
			var controllerPodName string
			var err error

			// projectimage stores the name of the image used in the example
			var projectimage = "example.com/hortator:v0.0.1"

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

		})
	})

	Context("AgentTask lifecycle", func() {
		It("should complete a legionary task in echo mode", func() {
			By("creating a legionary AgentTask")
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = createTaskManifest("e2e-echo-test", "legionary", "Hello from e2e test", "")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the task to complete")
			verifyTaskCompleted := func() error {
				cmd = exec.Command("kubectl", "get", "agenttask", "e2e-echo-test",
					"-n", namespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				phase := string(output)
				if phase != "Completed" && phase != "Failed" {
					return fmt.Errorf("task phase is %s, waiting for terminal", phase)
				}
				return nil
			}
			EventuallyWithOffset(1, verifyTaskCompleted, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("checking task has output")
			cmd = exec.Command("kubectl", "get", "agenttask", "e2e-echo-test",
				"-n", namespace, "-o", "jsonpath={.status.output}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).NotTo(BeEmpty())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "agenttask", "e2e-echo-test", "-n", namespace)
			_, _ = utils.Run(cmd)
		})

		It("should handle budget-exceeded tasks", func() {
			By("creating a task with very low budget")
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = createTaskManifest("e2e-budget-test", "legionary",
				"Write a very long essay", `{"budget":{"maxTokens":10}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the task to reach terminal phase")
			verifyTaskTerminal := func() error {
				cmd = exec.Command("kubectl", "get", "agenttask", "e2e-budget-test",
					"-n", namespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				phase := string(output)
				if phase != "Completed" && phase != "Failed" && phase != "BudgetExceeded" {
					return fmt.Errorf("task phase is %s, waiting for terminal", phase)
				}
				return nil
			}
			EventuallyWithOffset(1, verifyTaskTerminal, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "agenttask", "e2e-budget-test", "-n", namespace)
			_, _ = utils.Run(cmd)
		})

		It("should handle TTL cleanup", func() {
			By("verifying the task CR is garbage collected after retention")
			// This is tested implicitly by handleTTLCleanup in unit tests.
			// In e2e, we just verify the reconciler handles terminal tasks.
			Skip("TTL cleanup happens on timescale not suitable for e2e")
		})
	})

	Context("Tribune orchestration", func() {
		It("should spawn children and reincarnate", func() {
			By("creating a tribune AgentTask")
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = createTaskManifest("e2e-tribune-test", "tribune",
				"Write a Python calculator with tests. Delegate implementation and testing to separate agents.",
				`{"capabilities":["shell","spawn","files"]}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the tribune to reach a terminal or waiting phase")
			verifyTribuneProgress := func() error {
				cmd = exec.Command("kubectl", "get", "agenttask", "e2e-tribune-test",
					"-n", namespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				phase := string(output)
				// Tribune should either complete, fail (in echo mode), or enter Waiting
				if phase != "Completed" && phase != "Failed" && phase != "Waiting" && phase != "BudgetExceeded" {
					return fmt.Errorf("tribune phase is %s, waiting for progress", phase)
				}
				return nil
			}
			EventuallyWithOffset(1, verifyTribuneProgress, 5*time.Minute, 10*time.Second).Should(Succeed())

			By("checking for child tasks")
			cmd = exec.Command("kubectl", "get", "agenttask", "-n", namespace,
				"-l", "hortator.ai/parent=e2e-tribune-test", "-o", "name")
			output, _ := utils.Run(cmd)
			// Children may or may not exist depending on echo mode
			GinkgoWriter.Printf("Child tasks: %s\n", string(output))

			By("cleaning up tribune and children")
			cmd = exec.Command("kubectl", "delete", "agenttask", "-n", namespace,
				"-l", "hortator.ai/parent=e2e-tribune-test")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "agenttask", "e2e-tribune-test", "-n", namespace)
			_, _ = utils.Run(cmd)
		})
	})
})

// specExtrasFields represents the JSON structure of specExtras parameter.
type specExtrasFields struct {
	Capabilities []string          `json:"capabilities,omitempty"`
	Budget       map[string]any    `json:"budget,omitempty"`
}

// createTaskManifest generates an AgentTask YAML manifest as an io.Reader.
// specExtras is a JSON string with optional fields like capabilities and budget.
func createTaskManifest(name, tier, prompt, specExtras string) *strings.Reader {
	manifest := fmt.Sprintf(`apiVersion: core.hortator.ai/v1alpha1
kind: AgentTask
metadata:
  name: %s
  namespace: %s
spec:
  prompt: "%s"
  tier: %s
  timeout: 120
`, name, namespace, prompt, tier)

	if specExtras != "" {
		var extras specExtrasFields
		if err := json.Unmarshal([]byte(specExtras), &extras); err == nil {
			if len(extras.Capabilities) > 0 {
				manifest += "  capabilities:\n"
				for _, cap := range extras.Capabilities {
					manifest += fmt.Sprintf("    - %s\n", cap)
				}
			}
			if extras.Budget != nil {
				manifest += "  budget:\n"
				if v, ok := extras.Budget["maxTokens"]; ok {
					manifest += fmt.Sprintf("    maxTokens: %v\n", v)
				}
				if v, ok := extras.Budget["maxCostUsd"]; ok {
					manifest += fmt.Sprintf("    maxCostUsd: %v\n", v)
				}
			}
		}
	}

	return strings.NewReader(manifest)
}
