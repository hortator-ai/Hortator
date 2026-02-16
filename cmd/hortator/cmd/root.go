/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var (
	kubeconfig   string
	namespace    string
	outputFormat string
	k8sClient    client.Client
	clientset    *kubernetes.Clientset
	scheme       = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1alpha1.AddToScheme(scheme))
}

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
	Version: Version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "version" || cmd.Name() == "help" {
			return nil
		}
		return initClient()
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Target namespace")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format: table, json, yaml")
}

func initClient() error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Set namespace from kubeconfig if not specified
	if namespace == "" {
		ns, _, err := kubeConfig.Namespace()
		if err == nil && ns != "" {
			namespace = ns
		} else {
			namespace = "default"
		}
	}

	k8sClient, err = client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	return nil
}

func getNamespace() string {
	if namespace != "" {
		return namespace
	}
	if ns := os.Getenv("HORTATOR_NAMESPACE"); ns != "" {
		return ns
	}
	return "default"
}
