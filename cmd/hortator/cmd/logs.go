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
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	logsFollow    bool
	logsTail      int64
	logsContainer string
)

var logsCmd = &cobra.Command{
	Use:   "logs <task-name>",
	Short: "Get logs from an agent task",
	Long: `Get logs from the agent pod associated with a task.

Examples:
  # Get logs from a task
  hortator logs my-task

  # Follow logs in real-time
  hortator logs my-task -f

  # Get last N lines
  hortator logs my-task --tail 100`,
	Args: cobra.ExactArgs(1),
	RunE: runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().Int64Var(&logsTail, "tail", -1, "Number of lines to show from the end")
	logsCmd.Flags().StringVar(&logsContainer, "container", "agent", "Container name")
}

func runLogs(cmd *cobra.Command, args []string) error {
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

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	k8sClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	ns := getNamespace()

	gvr := schema.GroupVersionResource{
		Group:    "core.hortator.io",
		Version:  "v1alpha1",
		Resource: "agenttasks",
	}

	task, err := dynamicClient.Resource(gvr).Namespace(ns).Get(ctx, taskName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	status, found, _ := unstructured.NestedMap(task.Object, "status")
	if !found {
		return fmt.Errorf("task has no status yet")
	}

	podName, found, _ := unstructured.NestedString(status, "podName")
	if !found || podName == "" {
		return fmt.Errorf("task has no associated pod yet")
	}

	logOptions := &corev1.PodLogOptions{
		Container: logsContainer,
		Follow:    logsFollow,
	}
	if logsTail > 0 {
		logOptions.TailLines = &logsTail
	}

	req := k8sClientset.CoreV1().Pods(ns).GetLogs(podName, logOptions)
	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}
	defer stream.Close()

	if logsFollow {
		reader := bufio.NewReader(stream)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			os.Stdout.Write(line)
		}
	} else {
		_, err = io.Copy(os.Stdout, stream)
		if err != nil {
			return fmt.Errorf("failed to read logs: %w", err)
		}
	}

	return nil
}
