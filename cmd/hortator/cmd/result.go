/*
Copyright 2026 Hortator Authors.

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

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var resultCmd = &cobra.Command{
	Use:   "result <task-name>",
	Short: "Get the output result of a completed task",
	Long: `Get the output result from a completed agent task.

Examples:
  # Get result from a completed task
  hortator result my-task

  # Get result as JSON
  hortator result my-task -o json`,
	Args: cobra.ExactArgs(1),
	RunE: runResult,
}

func init() {
	rootCmd.AddCommand(resultCmd)
}

func runResult(cmd *cobra.Command, args []string) error {
	taskName := args[0]
	ctx := context.Background()

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ns := getNamespace()

	gvr := schema.GroupVersionResource{
		Group:    "core.hortator.io",
		Version:  "v1alpha1",
		Resource: "agenttasks",
	}

	task, err := client.Resource(gvr).Namespace(ns).Get(ctx, taskName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	status, found, _ := unstructured.NestedMap(task.Object, "status")
	if !found {
		return fmt.Errorf("task has no status yet")
	}

	phase, _, _ := unstructured.NestedString(status, "phase")
	if phase != "Succeeded" && phase != "Failed" && phase != "Timeout" {
		return fmt.Errorf("task is still %s, no result available yet", phase)
	}

	output, found, _ := unstructured.NestedString(status, "output")
	if !found || output == "" {
		fmt.Println("(no output)")
		return nil
	}

	switch outputFormat {
	case "json":
		result := map[string]interface{}{
			"name":      task.GetName(),
			"namespace": task.GetNamespace(),
			"phase":     phase,
			"output":    output,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	default:
		fmt.Println(output)
		return nil
	}
}
