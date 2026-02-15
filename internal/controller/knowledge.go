/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

// RetainedPVC represents a retained PVC that may be relevant to a new task.
type RetainedPVC struct {
	Name        string
	TaskName    string   // original task name (from annotation)
	Tags        []string // from hortator.ai/retain-tags
	Reason      string   // from hortator.ai/retain-reason
	CompletedAt string   // from hortator.ai/completed-at
	TagOverlap  int      // number of matching tags
}

// discoverRetainedPVCs finds retained PVCs in the namespace that match the
// task's role and tags. Returns matched PVCs sorted by relevance (tag overlap).
func (r *AgentTaskReconciler) discoverRetainedPVCs(ctx context.Context,
	task *corev1alpha1.AgentTask, cfg StorageRetainedConfig) ([]RetainedPVC, error) {

	if cfg.Discovery == "none" || !cfg.AutoMount {
		return nil, nil
	}

	logger := log.FromContext(ctx)

	// List all PVCs in the namespace
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcList, client.InNamespace(task.Namespace)); err != nil {
		return nil, err
	}

	// Build the set of tags to match against.
	// Include the task's role name and any tags from the prompt keywords.
	taskTags := buildTaskTags(task)

	var matches []RetainedPVC

	for _, pvc := range pvcList.Items {
		// Only consider retained PVCs
		if pvc.Annotations["hortator.ai/retain"] != "true" {
			continue
		}

		// Don't mount our own PVC
		taskPVCName := task.Name + "-storage"
		if pvc.Name == taskPVCName {
			continue
		}

		// Parse tags from PVC annotation
		tagsStr := pvc.Annotations["hortator.ai/retain-tags"]
		if tagsStr == "" {
			continue
		}
		pvcTags := splitTags(tagsStr)

		// Calculate tag overlap
		overlap := tagOverlap(taskTags, pvcTags)
		if overlap == 0 {
			continue
		}

		matches = append(matches, RetainedPVC{
			Name:        pvc.Name,
			TaskName:    pvc.Annotations["hortator.ai/task"],
			Tags:        pvcTags,
			Reason:      pvc.Annotations["hortator.ai/retain-reason"],
			CompletedAt: pvc.Annotations["hortator.ai/completed-at"],
			TagOverlap:  overlap,
		})
	}

	// Sort by relevance (highest tag overlap first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].TagOverlap > matches[j].TagOverlap
	})

	// Limit results
	maxResults := cfg.MaxRetainedPerNS
	if maxResults <= 0 {
		maxResults = 5
	}
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	if len(matches) > 0 {
		logger.Info("Discovered retained PVCs for task",
			"task", task.Name, "matches", len(matches))
	}

	return matches, nil
}

// buildTaskTags extracts tags from the task's role, tier, prompt keywords,
// and capabilities. Prompt words are included so that PVC tags like "database"
// or "migration" can match task prompts that mention those terms.
func buildTaskTags(task *corev1alpha1.AgentTask) map[string]bool {
	tags := make(map[string]bool)

	// Role name is always a tag
	if task.Spec.Role != "" {
		tags[strings.ToLower(task.Spec.Role)] = true
		// Split hyphenated role names too (e.g. "backend-dev" → "backend", "dev")
		for _, part := range strings.Split(task.Spec.Role, "-") {
			if len(part) > 2 {
				tags[strings.ToLower(part)] = true
			}
		}
	}

	// Tier
	if task.Spec.Tier != "" {
		tags[strings.ToLower(task.Spec.Tier)] = true
	}

	// Capabilities as tags
	for _, cap := range task.Spec.Capabilities {
		tags[strings.ToLower(cap)] = true
	}

	// Extract keywords from prompt (words with length > 3 to skip noise)
	if task.Spec.Prompt != "" {
		for _, word := range extractPromptKeywords(task.Spec.Prompt) {
			tags[word] = true
		}
	}

	return tags
}

// extractPromptKeywords splits the prompt into lowercase words, filtering out
// short words (<=3 chars) and common stop words. This enables PVC tag matching
// against prompt content.
func extractPromptKeywords(prompt string) []string {
	stopWords := map[string]bool{
		"this": true, "that": true, "with": true, "from": true,
		"have": true, "will": true, "your": true, "they": true,
		"been": true, "were": true, "then": true, "than": true,
		"what": true, "when": true, "where": true, "which": true,
		"there": true, "their": true, "about": true, "would": true,
		"could": true, "should": true, "these": true, "those": true,
		"into": true, "some": true, "make": true, "like": true,
		"just": true, "also": true, "each": true, "does": true,
	}

	// Replace common separators with spaces
	replacer := strings.NewReplacer(
		",", " ", ".", " ", ";", " ", ":", " ",
		"(", " ", ")", " ", "[", " ", "]", " ",
		"{", " ", "}", " ", "/", " ", "\\", " ",
		"\"", " ", "'", " ", "\n", " ", "\t", " ",
	)
	cleaned := replacer.Replace(prompt)

	words := strings.Fields(cleaned)
	keywords := make([]string, 0, len(words)/2)
	seen := make(map[string]bool)

	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if len(w) <= 3 || stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		keywords = append(keywords, w)
	}
	return keywords
}

// splitTags splits a comma-separated tag string into a slice.
func splitTags(tagsStr string) []string {
	parts := strings.Split(tagsStr, ",")
	tags := make([]string, 0, len(parts))
	for _, t := range parts {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// tagOverlap counts how many PVC tags match the task's tag set.
func tagOverlap(taskTags map[string]bool, pvcTags []string) int {
	count := 0
	for _, tag := range pvcTags {
		if taskTags[tag] {
			count++
		}
	}
	return count
}
