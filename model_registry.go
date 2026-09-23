package main

import (
	"strings"
	"sync"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

// ModelMeta carries per-model catalog metadata for COSY request semantics.
type ModelMeta struct {
	IsReasoning    bool
	MaxInputTokens int
}

type modelRecord struct {
	internalID string
	meta       ModelMeta
	hasMeta    bool
}

// modelRegistry stores one atomic catalog snapshot for each CPA auth record.
type modelRegistry struct {
	mu     sync.RWMutex
	byAuth map[string]map[string]modelRecord
}

var defaultModelRegistry = newModelRegistry()

func newModelRegistry() *modelRegistry {
	return &modelRegistry{byAuth: make(map[string]map[string]modelRecord)}
}

func (r *modelRegistry) store(authID string, mapping map[string]string) {
	r.storeCatalog(authID, mapping, nil)
}

func (r *modelRegistry) storeCatalog(authID string, mapping map[string]string, meta map[string]ModelMeta) {
	if r == nil {
		return
	}
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return
	}
	snapshot := make(map[string]modelRecord, len(mapping))
	for publicID, internalID := range mapping {
		publicID = strings.TrimSpace(publicID)
		internalID = strings.TrimSpace(internalID)
		if publicID == "" || internalID == "" {
			continue
		}
		modelMeta, hasMeta := meta[publicID]
		snapshot[publicID] = modelRecord{internalID: internalID, meta: modelMeta, hasMeta: hasMeta}
	}
	r.mu.Lock()
	r.byAuth[authID] = snapshot
	r.mu.Unlock()
}

func (r *modelRegistry) resolve(authID, publicID string) string {
	record, _ := r.resolveRecord(authID, publicID)
	return record.internalID
}

func (r *modelRegistry) resolveModel(authID, publicID string) (string, ModelMeta) {
	record, _ := r.resolveRecord(authID, publicID)
	return record.internalID, record.meta
}

func (r *modelRegistry) resolveRecord(authID, publicID string) (modelRecord, bool) {
	if r == nil {
		return modelRecord{}, false
	}
	authID = strings.TrimSpace(authID)
	publicID = strings.TrimSpace(publicID)
	if authID == "" || publicID == "" {
		return modelRecord{}, false
	}
	r.mu.RLock()
	mapping := r.byAuth[authID]
	record := mapping[publicID]
	if record.internalID == "" {
		if strings.HasPrefix(strings.ToLower(publicID), qoderauth.Provider+"/") {
			record = mapping[publicID[len(qoderauth.Provider)+1:]]
		} else {
			record = mapping[qoderauth.Provider+"/"+publicID]
		}
	}
	r.mu.RUnlock()
	return record, record.internalID != ""
}

func (r *modelRegistry) resolveMeta(authID, publicID string) (ModelMeta, bool) {
	record, ok := r.resolveRecord(authID, publicID)
	return record.meta, ok && record.hasMeta
}
