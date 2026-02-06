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
	"text/tabwriter"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

var statusCmd = &cobra.Command{
	Use:   "status <task-name>",
	Short: "Get the status of an agent task",
	Long: `Get detailed status information about an agent task.

Examples:
  # Get status of a task
  hortator status my-task

  # Get status in JSON format
  hortator status my-task -o json

  # Get status in YAML format
  hortator status my-task -o yaml`,
	Args: cobra.ExactArgs(1),
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
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

	switch outputFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(task.Object)
	case "yaml":
		data, err := yaml.Marshal(task.Object)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	default:
		return printStatus(task)
	}
}

func printStatus(task *unstructured.Unstructured) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	name := task.GetName()
	namespace := task.GetNamespace()
	createdAt := task.GetCreationTimestamp().Format("2006-01-02 15:04:05")

	spec, _, _ := unstructured.NestedMap(task.Object, "spec")
	prompt, _, _ := unstructured.NestedString(spec, "prompt")
	timeout, _, _ := unstructured.NestedString(spec, "timeout")
	image, _, _ := unstructured.NestedString(spec, "image")
	model, _, _ := unstructured.NestedString(spec, "model")

	status, _, _ := unstructured.NestedMap(task.Object, "status")
	phase, _, _ := unstructured.NestedString(status, "phase")
	podName, _, _ := unstructured.NestedString(status, "podName")
	message, _, _ := unstructured.NestedString(status, "message")
	startTime, _, _ := unstructured.NestedString(status, "startTime")
	completionTime, _, _ := unstructured.NestedString(status, "completionTime")

	fmt.Fprintf(w, "Name:\t%s\n", name)
	fmt.Fprintf(w, "Namespace:\t%s\n", namespace)
	fmt.Fprintf(w, "Created:\t%s\n", createdAt)
	fmt.Fprintf(w, "Phase:\t%s\n", phase)
	fmt.Fprintln(w)

	if len(prompt) > 80 {
		prompt = prompt[:77] + "..."
	}
	fmt.Fprintf(w, "Prompt:\t%s\n", prompt)
	fmt.Fprintf(w, "Timeout:\t%s\n", timeout)
	if image != "" {
		fmt.Fprintf(w, "Image:\t%s\n", image)
	}
	if model != "" {
		fmt.Fprintf(w, "Model:\t%s\n", model)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Pod:\t%s\n", podName)
	if startTime != "" {
		fmt.Fprintf(w, "Started:\t%s\n", startTime)
	}
	if completionTime != "" {
		fmt.Fprintf(w, "Completed:\t%s\n", completionTime)
	}
	if message != "" {
		fmt.Fprintf(w, "Message:\t%s\n", message)
	}

	return w.Flush()
}
