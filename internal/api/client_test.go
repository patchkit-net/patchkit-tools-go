package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_buildURL(t *testing.T) {
	c := NewClient("https://api.patchkit.net", "mykey123")

	tests := []struct {
		path string
		want string
	}{
		{"/1/apps/abc", "https://api.patchkit.net/1/apps/abc?api_key=mykey123"},
		{"1/apps/abc", "https://api.patchkit.net/1/apps/abc?api_key=mykey123"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := c.buildURL(tt.path)
			if got != tt.want {
				t.Errorf("buildURL(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestClient_buildURL_noAPIKey(t *testing.T) {
	c := NewClient("https://api.patchkit.net", "")
	got := c.buildURL("1/apps/abc")
	want := "https://api.patchkit.net/1/apps/abc"
	if got != want {
		t.Errorf("buildURL() = %q, want %q", got, want)
	}
}

func TestClient_buildURL_trailingSlash(t *testing.T) {
	c := NewClient("https://api.patchkit.net/", "key")
	got := c.buildURL("/1/apps/abc")
	want := "https://api.patchkit.net/1/apps/abc?api_key=key"
	if got != want {
		t.Errorf("buildURL() = %q, want %q", got, want)
	}
}

func TestClient_Get_success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"name": "TestApp"})
	}))
	defer server.Close()

	c := NewClient(server.URL, "testkey")
	var result map[string]string
	err := c.Get(context.Background(), "1/apps/abc", &result)
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if result["name"] != "TestApp" {
		t.Errorf("result[name] = %q, want %q", result["name"], "TestApp")
	}
}

func TestClient_Get_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
	}))
	defer server.Close()

	c := NewClient(server.URL, "testkey")
	var result map[string]string
	err := c.Get(context.Background(), "1/apps/notexist", &result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
	if apiErr.Message != "not found" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "not found")
	}
}

func TestClient_Get_retries5xxOnGET(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count <= 2 {
			w.WriteHeader(502)
			w.Write([]byte(`{"message":"bad gateway"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	c := NewClient(server.URL, "testkey")
	c.RetryPause = time.Millisecond // fast retries for test
	var result map[string]string
	err := c.Get(context.Background(), "1/test", &result)
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("result[status] = %q, want %q", result["status"], "ok")
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("attempts = %d, want 3", atomic.LoadInt32(&attempts))
	}
}

func TestClient_Post_noRetryOn5xx(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(500)
		w.Write([]byte(`{"message":"server error"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, "testkey")
	c.RetryPause = time.Millisecond
	err := c.Post(context.Background(), "1/test", map[string]string{"key": "val"}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// POST should NOT retry on 5xx - only 1 attempt
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("attempts = %d, want 1 (POST should not retry on 5xx)", atomic.LoadInt32(&attempts))
	}
}

func TestClient_Get_contextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	c := NewClient(server.URL, "testkey")
	err := c.Get(ctx, "1/test", nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestClient_Post_multipartForm(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct == "" {
			t.Error("expected Content-Type header")
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		if r.FormValue("label") != "1.0.0" {
			t.Errorf("label = %q, want %q", r.FormValue("label"), "1.0.0")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"id": 14})
	}))
	defer server.Close()

	c := NewClient(server.URL, "testkey")
	var result map[string]int
	err := c.Post(context.Background(), "1/apps/abc/versions", map[string]string{"label": "1.0.0"}, &result)
	if err != nil {
		t.Fatalf("Post() returned error: %v", err)
	}
	if result["id"] != 14 {
		t.Errorf("result[id] = %d, want 14", result["id"])
	}
}
