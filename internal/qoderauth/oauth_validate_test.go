package qoderauth

import "testing"

func TestValidateVerifyHost(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		insecure bool
		wantErr  bool
	}{
		{"https qoder.com", "https://qoder.com/activate", false, false},
		{"https qoder.sh", "https://api.qoder.sh/activate", false, false},
		{"https subdomain qoder.com", "https://auth.qoder.com/activate", false, false},
		{"http qoder.com blocked", "http://qoder.com/activate", false, true},
		{"http loopback allowed", "http://localhost/activate", true, false},
		{"http loopback 127 allowed", "http://127.0.0.1/activate", true, false},
		{"http loopback ::1 allowed", "http://[::1]/activate", true, false},
		{"http loopback blocked without insecure", "http://localhost/activate", false, true},
		{"non-qoder host blocked", "https://evil.com/activate", false, true},
		{"empty host blocked", "://", false, true},
		{"ftp scheme blocked", "ftp://qoder.com/activate", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateVerifyHost(tc.url, tc.insecure)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateVerifyHost(%q, %v) error=%v, wantErr=%v", tc.url, tc.insecure, err, tc.wantErr)
			}
		})
	}
}
