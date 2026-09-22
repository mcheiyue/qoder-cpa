package main

import (
	"sync"
	"testing"
)

func TestModelRegistryResolvesPerAccount(t *testing.T) {
	registry := newModelRegistry()
	registry.store("auth-a", map[string]string{"qoder/Qwen3.8-Flash": "qfmodel"})
	registry.store("auth-b", map[string]string{"qoder/Qwen3.8-Flash": "other-model"})

	if got := registry.resolve("auth-a", "qoder/Qwen3.8-Flash"); got != "qfmodel" {
		t.Fatalf("auth-a model=%q", got)
	}
	if got := registry.resolve("auth-b", "Qwen3.8-Flash"); got != "other-model" {
		t.Fatalf("auth-b model=%q", got)
	}
	if got := registry.resolve("auth-c", "qoder/Qwen3.8-Flash"); got != "" {
		t.Fatalf("unknown account model=%q", got)
	}
}

func TestModelRegistryStoreReplacesMapping(t *testing.T) {
	registry := newModelRegistry()
	registry.store("auth", map[string]string{"qoder/Old": "old"})
	registry.store("auth", map[string]string{"qoder/New": "new"})

	if got := registry.resolve("auth", "qoder/Old"); got != "" {
		t.Fatalf("stale model=%q", got)
	}
	if got := registry.resolve("auth", "qoder/New"); got != "new" {
		t.Fatalf("new model=%q", got)
	}
}

func TestModelRegistryConcurrentAccess(t *testing.T) {
	registry := newModelRegistry()
	const workers = 8
	var wait sync.WaitGroup
	wait.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wait.Done()
			authID := "auth-" + string(rune('a'+i))
			registry.store(authID, map[string]string{"qoder/model": "model"})
			if got := registry.resolve(authID, "qoder/model"); got != "model" {
				t.Errorf("auth=%q model=%q", authID, got)
			}
		}(i)
	}
	wait.Wait()
}
