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

func TestModelRegistryStoresMetadataPerAccount(t *testing.T) {
	registry := newModelRegistry()
	registry.storeCatalog(
		"auth-a",
		map[string]string{"qoder/r1": "reason-model"},
		map[string]ModelMeta{"qoder/r1": {IsReasoning: true, MaxInputTokens: 131072}},
	)
	registry.store("auth-b", map[string]string{"qoder/r1": "reason-model"})
	// auth-b has no metadata stored.

	meta, ok := registry.resolveMeta("auth-a", "qoder/r1")
	if !ok || !meta.IsReasoning || meta.MaxInputTokens != 131072 {
		t.Fatalf("auth-a metadata: %+v ok=%v", meta, ok)
	}
	meta, ok = registry.resolveMeta("auth-b", "qoder/r1")
	if ok {
		t.Fatalf("auth-b should have no metadata, got %+v", meta)
	}
}

func TestModelRegistryResolveMetaCopiesSnapshot(t *testing.T) {
	registry := newModelRegistry()
	original := map[string]ModelMeta{"qoder/m": {IsReasoning: true, MaxInputTokens: 100}}
	registry.storeCatalog("auth", map[string]string{"qoder/m": "m"}, original)
	// Mutate original after store — snapshot should be independent.
	original["qoder/m"] = ModelMeta{IsReasoning: false, MaxInputTokens: 999}
	meta, ok := registry.resolveMeta("auth", "qoder/m")
	if !ok || !meta.IsReasoning || meta.MaxInputTokens != 100 {
		t.Fatalf("snapshot was mutated: %+v", meta)
	}
}

func TestModelRegistryStoreReplacesMetadata(t *testing.T) {
	registry := newModelRegistry()
	registry.storeCatalog("auth", map[string]string{"qoder/m": "m"}, map[string]ModelMeta{"qoder/m": {IsReasoning: true}})
	registry.storeCatalog("auth", map[string]string{"qoder/m": "m"}, map[string]ModelMeta{"qoder/m": {IsReasoning: false}})
	meta, ok := registry.resolveMeta("auth", "qoder/m")
	if !ok || meta.IsReasoning {
		t.Fatalf("stale metadata: %+v", meta)
	}
}

func TestModelRegistryConcurrentStoreAndResolve(t *testing.T) {
	registry := newModelRegistry()
	var wg sync.WaitGroup
	const n = 8
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			authID := "auth-" + string(rune('a'+i))
			registry.storeCatalog(
				authID,
				map[string]string{"qoder/m": "m"},
				map[string]ModelMeta{"qoder/m": {IsReasoning: i%2 == 0, MaxInputTokens: i * 1000}},
			)
		}(i)
		go func(i int) {
			defer wg.Done()
			authID := "auth-" + string(rune('a'+i))
			registry.resolve(authID, "qoder/m")
			registry.resolveMeta(authID, "qoder/m")
		}(i)
	}
	wg.Wait()
}
