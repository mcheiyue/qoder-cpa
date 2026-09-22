package main

import (
	"strings"
	"sync"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

// modelRegistry stores the public model ID mapping for each CPA auth record.
type modelRegistry struct {
	mu     sync.RWMutex
	byAuth map[string]map[string]string
}

var defaultModelRegistry = newModelRegistry()

func newModelRegistry() *modelRegistry {
	return &modelRegistry{byAuth: make(map[string]map[string]string)}
}

func (r *modelRegistry) store(authID string, mapping map[string]string) {
	if r == nil {
		return
	}
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return
	}
	copyMapping := make(map[string]string, len(mapping))
	for publicID, internalID := range mapping {
		publicID = strings.TrimSpace(publicID)
		internalID = strings.TrimSpace(internalID)
		if publicID == "" || internalID == "" {
			continue
		}
		copyMapping[publicID] = internalID
	}
	r.mu.Lock()
	r.byAuth[authID] = copyMapping
	r.mu.Unlock()
}

func (r *modelRegistry) resolve(authID, publicID string) string {
	if r == nil {
		return ""
	}
	authID = strings.TrimSpace(authID)
	publicID = strings.TrimSpace(publicID)
	if authID == "" || publicID == "" {
		return ""
	}
	r.mu.RLock()
	mapping := r.byAuth[authID]
	internalID := mapping[publicID]
	if internalID == "" {
		if strings.HasPrefix(strings.ToLower(publicID), qoderauth.Provider+"/") {
			internalID = mapping[publicID[len(qoderauth.Provider)+1:]]
		} else {
			internalID = mapping[qoderauth.Provider+"/"+publicID]
		}
	}
	r.mu.RUnlock()
	return internalID
}
