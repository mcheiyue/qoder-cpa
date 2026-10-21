package main

import (
	"encoding/json"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// qoderAuthOwnedKeys 是插件 StorageJSON 自有（或由插件权威决定）的顶层键。
// 写回时一律以新载荷为准，绝不从旧文件回填 —— 否则会把已轮换的
// access_token/refresh_token 复活，或把上游已不认的 email 又写回去。
var qoderAuthOwnedKeys = map[string]bool{
	"type": true, "access_token": true, "refresh_token": true, "expires_at": true,
	"user_id": true, "user_type": true, "machine_id": true, "email": true,
	"label": true, "organization_id": true, "organization_tags": true,
	"runtime_info": true, "runtime_key": true, "transport_profile": true,
}

// mergeForeignAuthMetadata 把旧物理文件里非插件自有的顶层键（priority、
// disabled、note、weight 等宿主侧键）合并进新载荷。
// 宿主 saveAuthFile 是裸写（os.WriteFile，零合并），不合并就会抹掉它们 ——
// workbuddy 2026-10 Phase B 与 qoder 2026-10-21 各实证一次。
func mergeForeignAuthMetadata(oldJSON, freshJSON []byte) ([]byte, error) {
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(freshJSON, &merged); err != nil {
		return nil, fmt.Errorf("decode fresh auth payload: %w", err)
	}
	if len(oldJSON) > 0 {
		var old map[string]json.RawMessage
		if err := json.Unmarshal(oldJSON, &old); err != nil {
			return nil, fmt.Errorf("decode old auth metadata: %w", err)
		}
		for k, v := range old {
			if qoderAuthOwnedKeys[k] {
				continue
			}
			merged[k] = v
		}
	}
	return json.Marshal(merged)
}

// saveAuthPreservingHostKeys 合并外来键后写回。oldJSON 取写回前的物理文件内容；
// 合并失败时退化为直接写新载荷，不让合并逻辑阻塞登录/配额写回。
func (s *managementService) saveAuthPreservingHostKeys(name string, oldJSON, freshJSON []byte) error {
	payload := freshJSON
	if merged, err := mergeForeignAuthMetadata(oldJSON, freshJSON); err == nil {
		payload = merged
	}
	_, err := s.hostCall(pluginabi.MethodHostAuthSave, pluginapi.HostAuthSaveRequest{Name: name, JSON: payload})
	return err
}
