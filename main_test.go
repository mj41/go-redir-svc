package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handleHealth(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %s", result["status"])
	}
}

func TestHandleRedirect(t *testing.T) {
	// Setup temporary log directory
	tempDir := t.TempDir()

	// Initialize test config
	config = Config{
		Groups: map[string]GroupConfig{
			"test-group": {
				LogFile:        "test.jsonl",
				TargetBase:     "https://example.com",
				AllowedSchemes: []string{"http", "https"},
				Domains: []DomainConfig{
					{Name: "test.example.com", AllowedSchemes: []string{"http", "https"}},
					{Name: "www.test.example.com", AllowedSchemes: nil},
				},
			},
		},
	}

	// Initialize domain map
	domainToInfo = map[string]DomainInfo{
		"test.example.com":     {GroupName: "test-group", AllowedSchemes: []string{"http", "https"}},
		"www.test.example.com": {GroupName: "test-group", AllowedSchemes: nil},
	}

	// Initialize log files
	logFiles = make(map[string]*os.File)
	f, err := os.Create(filepath.Join(tempDir, "test.jsonl"))
	if err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}
	defer f.Close()
	logFiles["test-group"] = f

	tests := []struct {
		name           string
		host           string
		path           string
		scheme         string
		expectedStatus int
		expectedTarget string
	}{
		{
			name:           "redirect with path",
			host:           "test.example.com",
			path:           "/old-page",
			scheme:         "https",
			expectedStatus: http.StatusMovedPermanently,
			expectedTarget: "https://example.com/old-page",
		},
		{
			name:           "redirect with www subdomain",
			host:           "www.test.example.com",
			path:           "/",
			scheme:         "https",
			expectedStatus: http.StatusMovedPermanently,
			expectedTarget: "https://example.com/",
		},
		{
			name:           "unknown domain returns 404",
			host:           "unknown.example.com",
			path:           "/",
			scheme:         "https",
			expectedStatus: http.StatusNotFound,
			expectedTarget: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Host = tt.host
			req.Header.Set("X-Forwarded-Proto", tt.scheme)
			w := httptest.NewRecorder()

			handleRedirect(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			if tt.expectedTarget != "" {
				location := resp.Header.Get("Location")
				if location != tt.expectedTarget {
					t.Errorf("expected location %s, got %s", tt.expectedTarget, location)
				}
			}
		})
	}
}

func TestGetIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expectedIP string
	}{
		{
			name:       "X-Forwarded-For single IP",
			headers:    map[string]string{"X-Forwarded-For": "1.2.3.4"},
			remoteAddr: "5.6.7.8:1234",
			expectedIP: "1.2.3.4",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			headers:    map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"},
			remoteAddr: "9.10.11.12:1234",
			expectedIP: "1.2.3.4",
		},
		{
			name:       "X-Real-IP",
			headers:    map[string]string{"X-Real-IP": "1.2.3.4"},
			remoteAddr: "5.6.7.8:1234",
			expectedIP: "1.2.3.4",
		},
		{
			name:       "RemoteAddr fallback",
			headers:    map[string]string{},
			remoteAddr: "1.2.3.4:1234",
			expectedIP: "1.2.3.4:1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip := getIP(req)
			if ip != tt.expectedIP {
				t.Errorf("expected IP %s, got %s", tt.expectedIP, ip)
			}
		})
	}
}
