package util

import (
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestGetClientHelloID(t *testing.T) {
	tests := []struct {
		name        string
		fingerprint TLSFingerprint
		expectStr   string
	}{
		{"Chrome 120", FingerprintChrome120, "Chrome-120"},
		{"Chrome Latest", FingerprintChromeLatest, "Chrome-133"},
		{"Firefox 120", FingerprintFirefox120, "Firefox-120"},
		{"Safari Latest", FingerprintSafariLatest, "Safari-16.0"},
		{"Edge Latest", FingerprintEdgeLatest, "Edge-85"},
		{"iOS Latest", FingerprintiOSLatest, "iOS-14"},
		{"Android Latest", FingerprintAndroidLatest, "Android-11"},
		{"None", FingerprintNone, "Golang-0"},
		{"Unknown", TLSFingerprint("unknown"), "Chrome-133"}, // Should fallback to Chrome Auto
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helloID := GetClientHelloID(tt.fingerprint)
			str := helloID.Str()
			if str != tt.expectStr {
				t.Errorf("GetClientHelloID(%v) = %s, want %s", tt.fingerprint, str, tt.expectStr)
			}
		})
	}
}

func TestCreateUTLSTransport(t *testing.T) {
	tests := []struct {
		name          string
		fingerprint   TLSFingerprint
		baseTransport *http.Transport
		expectNil     bool
	}{
		{"With Chrome fingerprint", FingerprintChromeLatest, nil, false},
		{"With base transport", FingerprintFirefoxLatest, &http.Transport{}, false},
		{"Without fingerprint", FingerprintNone, nil, false},
		{"Empty fingerprint", TLSFingerprint(""), nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := CreateUTLSTransport(tt.fingerprint, tt.baseTransport)
			if (transport == nil) != tt.expectNil {
				t.Errorf("CreateUTLSTransport() returned %v, want nil=%v", transport, tt.expectNil)
			}
		})
	}
}

func TestApplyUTLSToClient(t *testing.T) {
	tests := []struct {
		name        string
		client      *http.Client
		fingerprint TLSFingerprint
		expectNil   bool
	}{
		{"New client with Chrome", nil, FingerprintChromeLatest, false},
		{"Existing client with Firefox", &http.Client{}, FingerprintFirefoxLatest, false},
		{"Client with no fingerprint", &http.Client{}, FingerprintNone, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := ApplyUTLSToClient(tt.client, tt.fingerprint)
			if (client == nil) != tt.expectNil {
				t.Errorf("ApplyUTLSToClient() returned %v, want nil=%v", client, tt.expectNil)
			}
			if client != nil && tt.fingerprint != FingerprintNone && tt.fingerprint != "" {
				if client.Transport == nil {
					t.Error("ApplyUTLSToClient() did not set Transport")
				}
			}
		})
	}
}

func TestSetProxyWithUTLS(t *testing.T) {
	tests := []struct {
		name           string
		proxyURL       string
		tlsFingerprint string
		expectNil      bool
	}{
		{"No proxy, no fingerprint", "", "", false},
		{"No proxy, with fingerprint", "", "chrome_latest", false},
		{"With HTTP proxy, no fingerprint", "http://proxy.example.com:8080", "", false},
		{"With HTTP proxy, with fingerprint", "http://proxy.example.com:8080", "firefox_latest", false},
		{"With SOCKS5 proxy, with fingerprint", "socks5://user:pass@proxy.example.com:1080", "safari_latest", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.SDKConfig{
				ProxyURL:       tt.proxyURL,
				TLSFingerprint: tt.tlsFingerprint,
			}
			client := &http.Client{}
			result := SetProxy(cfg, client)

			if (result == nil) != tt.expectNil {
				t.Errorf("SetProxy() returned %v, want nil=%v", result, tt.expectNil)
			}

			// Check if transport was set when fingerprint is enabled
			if tt.tlsFingerprint != "" && result != nil {
				if result.Transport == nil {
					t.Error("SetProxy() did not set Transport when fingerprint was specified")
				}
			}
		})
	}
}
