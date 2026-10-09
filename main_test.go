package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
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
				PreservePath:   true, // For backward compatibility in tests
				Domains: []DomainConfig{
					{Name: "test.example.com", AllowedSchemes: []string{"http", "https"}},
					{Name: "www.test.example.com", AllowedSchemes: nil},
				},
			},
			"test-group-strip": {
				LogFile:        "test-strip.jsonl",
				TargetBase:     "https://example.org",
				AllowedSchemes: []string{"http", "https"},
				PreservePath:   false, // Test path stripping
				Domains: []DomainConfig{
					{Name: "strip.example.com", AllowedSchemes: []string{"http", "https"}},
				},
			},
		},
	}

	// Initialize domain map
	domainToInfo = map[string]DomainInfo{
		"test.example.com":     {GroupName: "test-group", AllowedSchemes: []string{"http", "https"}},
		"www.test.example.com": {GroupName: "test-group", AllowedSchemes: nil},
		"strip.example.com":    {GroupName: "test-group-strip", AllowedSchemes: []string{"http", "https"}},
	}

	// Initialize log files
	logFiles = map[string]*dayLog{
		"test-group":       newDayLog(tempDir, "test.jsonl", "pod-a"),
		"test-group-strip": newDayLog(tempDir, "test-strip.jsonl", "pod-a"),
	}
	defer func() {
		for _, l := range logFiles {
			l.Close()
		}
	}()

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
		{
			name:           "strip path when preserve_path is false",
			host:           "strip.example.com",
			path:           "/some/path",
			scheme:         "https",
			expectedStatus: http.StatusMovedPermanently,
			expectedTarget: "https://example.org",
		},
		{
			name:           "strip path with query string",
			host:           "strip.example.com",
			path:           "/some/path?query=value",
			scheme:         "https",
			expectedStatus: http.StatusMovedPermanently,
			expectedTarget: "https://example.org?query=value",
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

func TestDayLog(t *testing.T) {
	dir := t.TempDir()
	l := newDayLog(dir, "shiftate.jsonl", "pod-a")
	defer l.Close()
	day1 := time.Date(2026, 10, 9, 23, 59, 0, 0, time.UTC)
	day2 := time.Date(2026, 10, 10, 0, 0, 1, 0, time.UTC)
	for _, w := range []struct {
		at   time.Time
		line string
	}{{day1, "a\n"}, {day1, "b\n"}, {day2, "c\n"}} {
		if err := l.write(w.at, []byte(w.line)); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{
		"shiftate-2026-10-09-pod-a.jsonl": "a\nb\n",
		"shiftate-2026-10-10-pod-a.jsonl": "c\n",
	} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(b) != want {
			t.Errorf("%s: %q, %v; want %q", name, b, err, want)
		}
	}
	// A local time zone does not move the day: the day is UTC's.
	prague := time.FixedZone("CEST", 2*3600)
	if err := l.write(time.Date(2026, 10, 10, 1, 30, 0, 0, prague), []byte("d\n")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "shiftate-2026-10-09-pod-a.jsonl")); string(b) != "a\nb\nd\n" {
		t.Errorf("01:30 CEST is 23:30 UTC the day before: %q", b)
	}
}
