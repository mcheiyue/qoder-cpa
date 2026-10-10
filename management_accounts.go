package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// fallbackQoderLabel 判断标签是否为空或 provider 兜底名（保活写回路径的重复注册
// 记录 metadata 无 email，label 落 provider）——去重时用于把兜底名升级为真实昵称。
func fallbackQoderLabel(label string) bool {
	label = strings.TrimSpace(label)
	return label == "" || strings.EqualFold(label, qoderauth.Provider)
}

// accounts 处理 GET /qoder/accounts。
// 同一文件被宿主双注册（扫描 AuthParse 与保活写回路径各一条、auth_index 相同）时，
// 行按 auth_index 塌缩并升级兜底标签/空邮箱；quota 处理不跳过——快照按 file.ID=
// 记录 ID 写入 scheduler，exhausted 排除依赖每记录一份，重复记录仍各自处理。
func (s *managementService) accounts(ctx context.Context) (pluginapi.ManagementResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := s.hostCall(pluginabi.MethodHostAuthList, nil)
	if err != nil {
		return pluginapi.ManagementResponse{}, err
	}
	var result struct {
		Files []pluginapi.HostAuthFileEntry `json:"files"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return pluginapi.ManagementResponse{}, fmt.Errorf("decode auth list: %w", err)
	}
	accounts := make([]managementAccount, 0, len(result.Files))
	seenAuth := make(map[string]int)
	for _, file := range result.Files {
		if !strings.EqualFold(file.Provider, qoderauth.Provider) && !strings.EqualFold(file.Type, qoderauth.Provider) {
			continue
		}
		key := file.AuthIndex
		if key == "" {
			key = file.Name
		}
		idx, dup := seenAuth[key]
		if dup {
			if fallbackQoderLabel(accounts[idx].Label) && !fallbackQoderLabel(file.Label) {
				accounts[idx].Label = file.Label
			}
			if accounts[idx].Email == "" && file.Email != "" {
				accounts[idx].Email = file.Email
			}
		} else {
			idx = len(accounts)
			seenAuth[key] = idx
			accounts = append(accounts, managementAccount{
				AuthIndex: file.AuthIndex, Name: file.Name, Label: file.Label,
				Email: file.Email, Profile: string(qoderauth.TransportProfileCosyAPI2),
				Status: file.Status, Disabled: file.Disabled, Priority: file.Priority,
			})
		}
		if file.AuthIndex != "" {
			if rawAuth, getErr := s.hostCall(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: file.AuthIndex}); getErr == nil {
				var auth pluginapi.HostAuthGetResponse
				var storage qoderauth.StorageJSON
				if json.Unmarshal(rawAuth, &auth) == nil && json.Unmarshal(auth.JSON, &storage) == nil {
					if qoderauth.IsValidProfile(storage.Profile) {
						accounts[idx].Profile = string(storage.Profile)
					}
					accounts[idx].NeedsAuth = storage.MachineID == ""
					if s.fetchQuota != nil && storage.AccessToken != "" {
						quota, quotaErr := s.fetchQuota(ctxOrBackground(ctx), storage.ToCredential())
						applyManagementQuota(&accounts[idx], quota, quotaErr)
						if quotaErr == nil && quota != nil {
							noteQuotaSnapshot(file.ID, quota.Remaining, quota.Exhausted)
						}
					}
				}
			}
		}
	}
	return jsonManagementResponse(http.StatusOK, managementAccountsResponse{Accounts: accounts})
}
