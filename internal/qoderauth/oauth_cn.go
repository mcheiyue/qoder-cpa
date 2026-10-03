package qoderauth

// DefaultConfigCN returns the official Qoder CN (qoder.com.cn) Device OAuth
// endpoints. Path suffixes mirror the intl DefaultConfig; only hosts differ.
func DefaultConfigCN() OAuthConfig {
	return OAuthConfig{
		BaseURL:    "https://qoder.com.cn",
		APIBaseURL: "https://openapi.qoder.com.cn",
		DevicePath: "/device/selectAccounts",
		PollPath:   "/api/v1/deviceToken/poll",
		ClientID:   "qoder-cpa",
	}
}
