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
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

var (
	spawnPrompt       string
	spawnCapabilities []string
	spawnTimeout      string
	spawnImage        string
	spawnModel        string
	spawnName         string
	spawnWait         bool
)

var spawnCmd = &cobra.Command{
	Use:   "spawn",
	Short: "Spawn a new agent task",
	Long: `Spawn a new agent task in the cluster.

Examples:
  hortator spawn --prompt "Write a hello world in Python"
  hortator spawn --prompt "Deploy the app" --capabilities exec,kubernetes
  hortator spawn --prompt "Run tests" --image myregistry/agent:v1 --timeout 1h
  hortator spawn --prompt "Quick task" --wait`,
	RunE: runSpawn,
}

func init() {
	spawnCmd.Flags().StringVarP(&spawnPrompt, "prompt", "p", "", "Task prompt (required)")
	spawnCmd.Flags().StringSliceVarP(&spawnCapabilities, "capabilities", "c", nil, "Agent capabilities")
	spawnCmd.Flags().StringVarP(&spawnTimeout, "timeout", "t", "30m", "Task timeout")
	spawnCmd.Flags().StringVarP(&spawnImage, "image", "i", "", "Agent container image")
	spawnCmd.Flags().StringVarP(&spawnModel, "model", "m", "", "LLM model")
	spawnCmd.Flags().StringVar(&spawnName, "name", "", "Task name")
	spawnCmd.Flags().BoolVarP(&spawnWait, "wait", "w", false, "Wait for completion")
	spawnCmd.MarkFlagRequired("prompt")
	rootCmd.AddCommand(spawnCmd)
}

func runSpawn(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	name := spawnName
	if name == "" {
		name = fmt.Sprintf("task-%d", time.Now().Unix())
	}
	name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	// Parse timeout string to seconds
	timeoutDuration, err := time.ParseDuration(spawnTimeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}
	timeoutSec := int(timeoutDuration.Seconds())

	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: getNamespace(),
		},
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt:       spawnPrompt,
			Capabilities: spawnCapabilities,
			Image:        spawnImage,
			Timeout:      &timeoutSec,
		},
	}

	if spawnModel != "" {
		task.Spec.Model = &corev1alpha1.ModelSpec{Name: spawnModel}
	}

	if err := k8sClient.Create(ctx, task); err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	fmt.Printf("✓ Task '%s' created in namespace '%s'\n", name, getNamespace())

	if !spawnWait {
		fmt.Printf("\nUse 'hortator status %s' to check progress\n", name)
		return nil
	}

	fmt.Println("\nWaiting for task completion...")
	return waitForTask(ctx, name)
}

func waitForTask(ctx context.Context, name string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			task := &corev1alpha1.AgentTask{}
			if err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: getNamespace(),
				Name:      name,
			}, task); err != nil {
				return fmt.Errorf("failed to get task: %w", err)
			}

			switch task.Status.Phase {
			case corev1alpha1.AgentTaskPhaseCompleted:
				fmt.Printf("✓ Task completed successfully\n")
				if task.Status.Output != "" {
					fmt.Printf("\nOutput:\n%s\n", task.Status.Output)
				}
				return nil
			case corev1alpha1.AgentTaskPhaseFailed:
				return fmt.Errorf("task failed: %s", task.Status.Message)
			case corev1alpha1.AgentTaskPhaseRunning:
				fmt.Printf("  Running... (pod: %s)\n", task.Status.PodName)
			case corev1alpha1.AgentTaskPhasePending:
				fmt.Println("  Pending...")
			}
		}
	}
}
