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
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

var (
	listAllNamespaces bool
	listPhase         string
	listLimit         int64
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List agent tasks",
	Long: `List agent tasks in the cluster.

Examples:
  # List all tasks in current namespace
  hortator list

  # List tasks in all namespaces
  hortator list -A

  # List only running tasks
  hortator list --phase Running`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().BoolVarP(&listAllNamespaces, "all-namespaces", "A", false, "List tasks in all namespaces")
	listCmd.Flags().StringVar(&listPhase, "phase", "", "Filter by phase")
	listCmd.Flags().Int64Var(&listLimit, "limit", 0, "Maximum number of tasks to list")
}

func runList(cmd *cobra.Command, args []string) error {
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
	if listAllNamespaces {
		ns = ""
	}

	listOpts := metav1.ListOptions{}
	if listLimit > 0 {
		listOpts.Limit = listLimit
	}

	gvr := schema.GroupVersionResource{
		Group:    "core.hortator.io",
		Version:  "v1alpha1",
		Resource: "agenttasks",
	}

	var tasks *unstructured.UnstructuredList
	if ns == "" {
		tasks, err = client.Resource(gvr).List(ctx, listOpts)
	} else {
		tasks, err = client.Resource(gvr).Namespace(ns).List(ctx, listOpts)
	}
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	if listPhase != "" {
		filtered := make([]unstructured.Unstructured, 0)
		for _, task := range tasks.Items {
			status, _, _ := unstructured.NestedMap(task.Object, "status")
			phase, _, _ := unstructured.NestedString(status, "phase")
			if phase == listPhase {
				filtered = append(filtered, task)
			}
		}
		tasks.Items = filtered
	}

	switch outputFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tasks.Items)
	case "yaml":
		data, err := yaml.Marshal(tasks.Items)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	default:
		return printList(tasks.Items, listAllNamespaces)
	}
}

func printList(tasks []unstructured.Unstructured, showNamespace bool) error {
	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if showNamespace {
		fmt.Fprintf(w, "NAMESPACE\tNAME\tPHASE\tAGE\n")
	} else {
		fmt.Fprintf(w, "NAME\tPHASE\tAGE\n")
	}

	for _, task := range tasks {
		name := task.GetName()
		ns := task.GetNamespace()
		age := formatTaskAge(task.GetCreationTimestamp().Time)

		status, _, _ := unstructured.NestedMap(task.Object, "status")
		phase, _, _ := unstructured.NestedString(status, "phase")
		if phase == "" {
			phase = "Unknown"
		}

		if showNamespace {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ns, name, phase, age)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n", name, phase, age)
		}
	}

	return w.Flush()
}

func formatTaskAge(t time.Time) string {
	d := time.Since(t)
	if d.Hours() >= 24 {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	if d.Hours() >= 1 {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if d.Minutes() >= 1 {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
