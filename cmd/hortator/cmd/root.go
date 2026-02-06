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
	"os"

	"github.com/spf13/cobra"
)

var (
	// kubeconfig is the path to the kubeconfig file
	kubeconfig string
	// namespace is the target namespace
	namespace string
	// outputFormat is the output format (json, yaml, table)
	outputFormat string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "hortator",
	Short: "CLI for Hortator - AI Agent Orchestration for Kubernetes",
	Long: `Hortator is a Kubernetes operator that enables AI agents to spawn other agents.

It provides a CLI interface for agents to create, monitor, and collect results
from worker tasks running in the cluster.

Examples:
  # Spawn a new agent task and wait for completion
  hortator spawn --prompt "Analyze the logs in /var/log" --wait

  # Check status of a running task
  hortator status my-task

  # Get logs from a task
  hortator logs my-task

  # Get the result output from a completed task
  hortator result my-task

  # List all tasks
  hortator list`,
	Version: "0.1.0",
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (defaults to $KUBECONFIG or ~/.kube/config)")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Target namespace (defaults to current context namespace)")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format: table, json, yaml")
}

// getNamespace returns the namespace to use, falling back to default
func getNamespace() string {
	if namespace != "" {
		return namespace
	}
	if ns := os.Getenv("HORTATOR_NAMESPACE"); ns != "" {
		return ns
	}
	return "default"
}
