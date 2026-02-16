/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var (
	rolesAllNamespaces bool
	rolesCapability    string
)

var rolesCmd = &cobra.Command{
	Use:   "roles",
	Short: "Manage agent roles",
}

var rolesListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List agent roles",
	Long: `List all ClusterAgentRoles and AgentRoles.

Examples:
  hortator roles list
  hortator roles list -A
  hortator roles list --capability shell,spawn
  hortator roles list --json`,
	RunE: runRolesList,
}

var rolesDescribeCmd = &cobra.Command{
	Use:   "describe <name>",
	Short: "Describe an agent role",
	Long: `Show full details for a single agent role.

Looks up namespace-scoped roles first, then falls back to cluster-scoped.

Examples:
  hortator roles describe endpoint-coder
  hortator roles describe endpoint-coder --json`,
	Args: cobra.ExactArgs(1),
	RunE: runRolesDescribe,
}

func init() {
	rolesListCmd.Flags().BoolVarP(&rolesAllNamespaces, "all-namespaces", "A", false, "Include namespace-scoped roles from all namespaces")
	rolesListCmd.Flags().StringVar(&rolesCapability, "capability", "", "Filter by capabilities (comma-separated, role must have ALL)")
	rolesCmd.AddCommand(rolesListCmd)
	rolesCmd.AddCommand(rolesDescribeCmd)
	rootCmd.AddCommand(rolesCmd)
}

// roleEntry is a unified view of both cluster and namespace-scoped roles.
type roleEntry struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace,omitempty"`
	Scope        string   `json:"scope"`
	TierAffinity string   `json:"tierAffinity,omitempty"`
	Model        string   `json:"model,omitempty"`
	Description  string   `json:"description,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	Rules        []string `json:"rules,omitempty"`
	AntiPatterns []string `json:"antiPatterns,omitempty"`
}

func runRolesList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	var entries []roleEntry

	// List ClusterAgentRoles
	clusterRoles := &corev1alpha1.ClusterAgentRoleList{}
	if err := k8sClient.List(ctx, clusterRoles); err != nil {
		return fmt.Errorf("failed to list cluster roles: %w", err)
	}
	for _, r := range clusterRoles.Items {
		entries = append(entries, roleEntry{
			Name:         r.Name,
			Scope:        "Cluster",
			TierAffinity: r.Spec.TierAffinity,
			Model:        r.Spec.DefaultModel,
			Description:  r.Spec.Description,
			Tools:        r.Spec.Tools,
			Rules:        r.Spec.Rules,
			AntiPatterns: r.Spec.AntiPatterns,
		})
	}

	// List namespace-scoped AgentRoles
	nsRoles := &corev1alpha1.AgentRoleList{}
	var listOpts []client.ListOption
	if !rolesAllNamespaces {
		listOpts = append(listOpts, client.InNamespace(getNamespace()))
	}
	if err := k8sClient.List(ctx, nsRoles, listOpts...); err != nil {
		return fmt.Errorf("failed to list roles: %w", err)
	}
	for _, r := range nsRoles.Items {
		entries = append(entries, roleEntry{
			Name:         r.Name,
			Namespace:    r.Namespace,
			Scope:        "Namespaced",
			TierAffinity: r.Spec.TierAffinity,
			Model:        r.Spec.DefaultModel,
			Description:  r.Spec.Description,
			Tools:        r.Spec.Tools,
			Rules:        r.Spec.Rules,
			AntiPatterns: r.Spec.AntiPatterns,
		})
	}

	// Filter by capability
	if rolesCapability != "" {
		required := strings.Split(rolesCapability, ",")
		var filtered []roleEntry
		for _, e := range entries {
			if hasAllTools(e.Tools, required) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	if outputFormat == "json" {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(entries) == 0 {
		fmt.Println("No roles found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tTIER AFFINITY\tDESCRIPTION")
	for _, e := range entries {
		desc := e.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", e.Name, e.TierAffinity, desc)
	}
	return w.Flush()
}

func runRolesDescribe(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	name := args[0]

	var entry *roleEntry

	// Try namespace-scoped first
	nsRole := &corev1alpha1.AgentRole{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: getNamespace()}, nsRole)
	if err == nil {
		entry = &roleEntry{
			Name:         nsRole.Name,
			Namespace:    nsRole.Namespace,
			Scope:        "Namespaced",
			TierAffinity: nsRole.Spec.TierAffinity,
			Model:        nsRole.Spec.DefaultModel,
			Description:  nsRole.Spec.Description,
			Tools:        nsRole.Spec.Tools,
			Rules:        nsRole.Spec.Rules,
			AntiPatterns: nsRole.Spec.AntiPatterns,
		}
	} else if apierrors.IsNotFound(err) {
		// Fall back to cluster-scoped
		clusterRole := &corev1alpha1.ClusterAgentRole{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: name}, clusterRole)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("role %q not found", name)
			}
			return fmt.Errorf("failed to get cluster role: %w", err)
		}
		entry = &roleEntry{
			Name:         clusterRole.Name,
			Scope:        "Cluster",
			TierAffinity: clusterRole.Spec.TierAffinity,
			Model:        clusterRole.Spec.DefaultModel,
			Description:  clusterRole.Spec.Description,
			Tools:        clusterRole.Spec.Tools,
			Rules:        clusterRole.Spec.Rules,
			AntiPatterns: clusterRole.Spec.AntiPatterns,
		}
	} else {
		return fmt.Errorf("failed to get role: %w", err)
	}

	if outputFormat == "json" {
		data, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Human-friendly output
	fmt.Printf("Name:          %s\n", entry.Name)
	fmt.Printf("Scope:         %s\n", entry.Scope)
	if entry.TierAffinity != "" {
		fmt.Printf("Tier Affinity: %s\n", entry.TierAffinity)
	}
	if entry.Model != "" {
		fmt.Printf("Model:         %s\n", entry.Model)
	}
	if entry.Description != "" {
		fmt.Printf("Description:   %s\n", entry.Description)
	}

	if len(entry.Tools) > 0 {
		fmt.Println("\nTools:")
		for _, t := range entry.Tools {
			fmt.Printf("  - %s\n", t)
		}
	}

	if len(entry.Rules) > 0 {
		fmt.Println("\nRules:")
		for _, r := range entry.Rules {
			fmt.Printf("  - %s\n", r)
		}
	}

	if len(entry.AntiPatterns) > 0 {
		fmt.Println("\nAnti-Patterns:")
		for _, a := range entry.AntiPatterns {
			fmt.Printf("  - %s\n", a)
		}
	}

	return nil
}

func hasAllTools(tools []string, required []string) bool {
	set := make(map[string]struct{}, len(tools))
	for _, t := range tools {
		set[t] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[strings.TrimSpace(r)]; !ok {
			return false
		}
	}
	return true
}
