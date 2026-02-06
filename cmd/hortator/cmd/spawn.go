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
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	spawnPrompt       string
	spawnCapabilities []string
	spawnTimeout      string
	spawnImage        string
	spawnModel        string
	spawnWait         bool
	spawnName         string
	spawnParentTask   string
)

var spawnCmd = &cobra.Command{
	Use:   "spawn",
	Short: "Spawn a new agent task",
	Long: `Spawn a new agent task in the cluster.

The task will be scheduled as a Pod running an agent container.
Use --wait to block until the task completes.

Examples:
  # Spawn a task with a prompt
  hortator spawn --prompt "Analyze the application logs"

  # Spawn with specific capabilities
  hortator spawn --prompt "Search for..." --capabilities web-search,file-access

  # Spawn and wait for completion
  hortator spawn --prompt "Run tests" --wait

  # Spawn with custom timeout
  hortator spawn --prompt "Long running analysis" --timeout 2h`,
	RunE: runSpawn,
}

func init() {
	rootCmd.AddCommand(spawnCmd)

	spawnCmd.Flags().StringVarP(&spawnPrompt, "prompt", "p", "", "The prompt/instruction for the agent (required)")
	spawnCmd.Flags().StringSliceVarP(&spawnCapabilities, "capabilities", "c", nil, "Comma-separated list of capabilities")
	spawnCmd.Flags().StringVarP(&spawnTimeout, "timeout", "t", "30m", "Task timeout (e.g., 30m, 1h)")
	spawnCmd.Flags().StringVar(&spawnImage, "image", "", "Custom agent image")
	spawnCmd.Flags().StringVar(&spawnModel, "model", "", "AI model to use")
	spawnCmd.Flags().BoolVarP(&spawnWait, "wait", "w", false, "Wait for task completion")
	spawnCmd.Flags().StringVar(&spawnName, "name", "", "Custom task name")
	spawnCmd.Flags().StringVar(&spawnParentTask, "parent", "", "Parent task name")

	_ = spawnCmd.MarkFlagRequired("prompt")
}

func runSpawn(cmd *cobra.Command, args []string) error {
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

	taskName := spawnName
	if taskName == "" {
		taskName = fmt.Sprintf("task-%d", time.Now().Unix())
	}
	taskName = strings.ToLower(strings.ReplaceAll(taskName, " ", "-"))

	task := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "core.hortator.io/v1alpha1",
			"kind":       "AgentTask",
			"metadata": map[string]interface{}{
				"name":      taskName,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"prompt":  spawnPrompt,
				"timeout": spawnTimeout,
			},
		},
	}

	spec := task.Object["spec"].(map[string]interface{})
	if len(spawnCapabilities) > 0 {
		spec["capabilities"] = spawnCapabilities
	}
	if spawnImage != "" {
		spec["image"] = spawnImage
	}
	if spawnModel != "" {
		spec["model"] = spawnModel
	}
	if spawnParentTask != "" {
		spec["parentTask"] = spawnParentTask
	}

	gvr := schema.GroupVersionResource{
		Group:    "core.hortator.io",
		Version:  "v1alpha1",
		Resource: "agenttasks",
	}

	created, err := client.Resource(gvr).Namespace(ns).Create(ctx, task, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	fmt.Printf("Created task: %s/%s\n", ns, created.GetName())

	if !spawnWait {
		return nil
	}

	fmt.Println("Waiting for task completion...")

	watcher, err := client.Resource(gvr).Namespace(ns).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", taskName),
	})
	if err != nil {
		return fmt.Errorf("failed to watch task: %w", err)
	}
	defer watcher.Stop()

	for event := range watcher.ResultChan() {
		if event.Type == watch.Modified || event.Type == watch.Added {
			obj := event.Object.(*unstructured.Unstructured)
			status, found, _ := unstructured.NestedMap(obj.Object, "status")
			if !found {
				continue
			}
			phase, _, _ := unstructured.NestedString(status, "phase")
			switch phase {
			case "Succeeded":
				output, _, _ := unstructured.NestedString(status, "output")
				fmt.Println("Task completed successfully!")
				if output != "" {
					fmt.Println("\nOutput:")
					fmt.Println(output)
				}
				return nil
			case "Failed":
				message, _, _ := unstructured.NestedString(status, "message")
				fmt.Printf("Task failed: %s\n", message)
				os.Exit(1)
			case "Timeout":
				fmt.Println("Task timed out")
				os.Exit(1)
			}
		}
	}

	return nil
}
