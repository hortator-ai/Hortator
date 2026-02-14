/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"
)

func TestEffectiveCapabilities(t *testing.T) {
	t.Run("legionary gets no auto-inject", func(t *testing.T) {
		caps := effectiveCapabilities("legionary", []string{"shell"})
		if len(caps) != 1 || caps[0] != "shell" {
			t.Errorf("expected [shell], got %v", caps)
		}
	})

	t.Run("tribune gets spawn auto-injected", func(t *testing.T) {
		caps := effectiveCapabilities("tribune", []string{"shell"})
		found := false
		for _, c := range caps {
			if c == "spawn" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected spawn in caps, got %v", caps)
		}
	})

	t.Run("centurion gets spawn auto-injected", func(t *testing.T) {
		caps := effectiveCapabilities("centurion", []string{"shell"})
		found := false
		for _, c := range caps {
			if c == "spawn" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected spawn in caps, got %v", caps)
		}
	})

	t.Run("tribune with spawn already present no duplicate", func(t *testing.T) {
		caps := effectiveCapabilities("tribune", []string{"shell", "spawn"})
		count := 0
		for _, c := range caps {
			if c == "spawn" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected exactly 1 spawn, got %d in %v", count, caps)
		}
	})

	t.Run("does not mutate input slice", func(t *testing.T) {
		input := []string{"shell"}
		_ = effectiveCapabilities("tribune", input)
		if len(input) != 1 {
			t.Errorf("input slice was mutated: %v", input)
		}
	})
}

func TestCapabilitySubsetValidation(t *testing.T) {
	// Helper: checks if childCaps ⊆ effectiveCapabilities(parentTier, parentCaps)
	isSubset := func(parentTier string, parentCaps, childCaps []string) (bool, string) {
		effective := effectiveCapabilities(parentTier, parentCaps)
		allowed := make(map[string]bool, len(effective))
		for _, c := range effective {
			allowed[c] = true
		}
		for _, c := range childCaps {
			if !allowed[c] {
				return false, c
			}
		}
		return true, ""
	}

	t.Run("child subset allowed", func(t *testing.T) {
		ok, _ := isSubset("legionary", []string{"shell", "spawn"}, []string{"shell"})
		if !ok {
			t.Error("expected subset to be allowed")
		}
	})

	t.Run("child equal set allowed", func(t *testing.T) {
		ok, _ := isSubset("legionary", []string{"shell", "spawn"}, []string{"shell", "spawn"})
		if !ok {
			t.Error("expected equal set to be allowed")
		}
	})

	t.Run("child escalation rejected", func(t *testing.T) {
		ok, denied := isSubset("legionary", []string{"shell"}, []string{"shell", "spawn"})
		if ok {
			t.Error("expected escalation to be rejected")
		}
		if denied != "spawn" {
			t.Errorf("expected denied capability 'spawn', got %q", denied)
		}
	})

	t.Run("child of tribune can use spawn via auto-inject", func(t *testing.T) {
		// Tribune parent has ["shell"] but spawn is auto-injected
		ok, _ := isSubset("tribune", []string{"shell"}, []string{"shell", "spawn"})
		if !ok {
			t.Error("expected spawn to be allowed for child of tribune (auto-injected)")
		}
	})

	t.Run("child of centurion can use spawn via auto-inject", func(t *testing.T) {
		ok, _ := isSubset("centurion", []string{"shell"}, []string{"spawn"})
		if !ok {
			t.Error("expected spawn to be allowed for child of centurion (auto-injected)")
		}
	})

	t.Run("empty child caps always allowed", func(t *testing.T) {
		ok, _ := isSubset("legionary", []string{"shell"}, []string{})
		if !ok {
			t.Error("expected empty child caps to be allowed")
		}
	})

	t.Run("child requests cap parent doesn't have even with auto-inject", func(t *testing.T) {
		ok, denied := isSubset("tribune", []string{"shell"}, []string{"network"})
		if ok {
			t.Error("expected escalation to be rejected")
		}
		if denied != "network" {
			t.Errorf("expected denied 'network', got %q", denied)
		}
	})
}
