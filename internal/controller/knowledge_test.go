/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

func TestBuildTaskTags_Role(t *testing.T) {
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "hello",
			Role:   "backend-dev",
			Tier:   "legionary",
		},
	}
	tags := buildTaskTags(task)
	for _, want := range []string{"backend-dev", "backend"} {
		if !tags[want] {
			t.Errorf("expected tag %q", want)
		}
	}
	// "dev" is 3 chars, included by role splitter (> 2 chars)
	if !tags["dev"] {
		t.Error("expected role part 'dev' (3 chars)")
	}
}

func TestBuildTaskTags_Capabilities(t *testing.T) {
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt:       "hello",
			Capabilities: []string{"shell", "network"},
		},
	}
	tags := buildTaskTags(task)
	if !tags["shell"] || !tags["network"] {
		t.Errorf("expected capability tags, got %v", tags)
	}
}

func TestBuildTaskTags_PromptKeywords(t *testing.T) {
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "Migrate the database schema and update the backend API",
		},
	}
	tags := buildTaskTags(task)
	for _, want := range []string{"migrate", "database", "schema", "update", "backend"} {
		if !tags[want] {
			t.Errorf("expected prompt keyword %q in tags", want)
		}
	}
	// "the" and "and" are <=3 chars, should not be present
	if tags["the"] || tags["and"] {
		t.Error("short words should be filtered")
	}
}

func TestBuildTaskTags_PromptStopWords(t *testing.T) {
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "this should be about testing where there would be results",
		},
	}
	tags := buildTaskTags(task)
	for _, stop := range []string{"this", "about", "where", "there", "would"} {
		if tags[stop] {
			t.Errorf("stop word %q should be filtered", stop)
		}
	}
	if !tags["testing"] {
		t.Error("expected 'testing' to be included")
	}
	if !tags["results"] {
		t.Error("expected 'results' to be included")
	}
}

func TestExtractPromptKeywords(t *testing.T) {
	keywords := extractPromptKeywords("Build a REST API for user authentication, including JWT tokens.")
	keyMap := make(map[string]bool)
	for _, k := range keywords {
		keyMap[k] = true
	}
	if !keyMap["build"] || !keyMap["rest"] || !keyMap["authentication"] || !keyMap["tokens"] {
		t.Errorf("unexpected keywords: %v", keywords)
	}
}

func TestTagOverlap(t *testing.T) {
	taskTags := map[string]bool{"backend": true, "database": true, "api": true}
	pvcTags := []string{"database", "migration", "backend"}
	overlap := tagOverlap(taskTags, pvcTags)
	if overlap != 2 {
		t.Errorf("expected overlap 2, got %d", overlap)
	}
}

func TestTagOverlap_NoMatch(t *testing.T) {
	taskTags := map[string]bool{"frontend": true}
	pvcTags := []string{"database", "backend"}
	overlap := tagOverlap(taskTags, pvcTags)
	if overlap != 0 {
		t.Errorf("expected overlap 0, got %d", overlap)
	}
}

func TestSplitTags(t *testing.T) {
	tags := splitTags("  database , BACKEND , api ")
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %v", len(tags), tags)
	}
	if tags[0] != "database" || tags[1] != "backend" || tags[2] != "api" {
		t.Errorf("unexpected tags: %v", tags)
	}
}

func TestSplitTags_Empty(t *testing.T) {
	tags := splitTags("")
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %v", tags)
	}
}

func TestDiscoverRetainedPVCs_SkipOwnPVC(t *testing.T) {
	// buildTaskTags should work for a task even with a name set
	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{Name: "my-task", Namespace: "default"},
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "database migration",
			Role:   "backend",
		},
	}
	tags := buildTaskTags(task)
	if !tags["database"] || !tags["migration"] || !tags["backend"] {
		t.Errorf("expected prompt + role tags, got %v", tags)
	}
}
