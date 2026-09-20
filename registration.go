package main

import (
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

const (
	pluginID = "qoder"
	version  = "0.1.7"
	author   = "mcheiyue"
	repoURL  = "https://github.com/mcheiyue/qoder-cpa"
)

func registration() map[string]any {
	return map[string]any{
		"schema_version": pluginabi.SchemaVersion,
		"metadata": map[string]any{
			"Name":             pluginID,
			"Version":          version,
			"Author":           author,
			"GitHubRepository": repoURL,
			"ConfigFields":     []map[string]any{},
		},
		"capabilities": map[string]any{
			"auth_provider":           true,
			"model_provider":          true,
			"executor":                true,
			"management_api":          true,
			"executor_model_scope":    "oauth",
			"executor_input_formats":  []string{"chat-completions"},
			"executor_output_formats": []string{"chat-completions"},
		},
	}
}
