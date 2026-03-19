package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/patchkit-net/patchkit-tools-go/internal/api"
	"github.com/patchkit-net/patchkit-tools-go/internal/lock"
)

func init() {
	// Speed up lock timings for tests
	lock.PollInterval = 10 * time.Millisecond
	lock.RefreshInterval = time.Hour // effectively disable refresh
	lock.SafetyCheckPause = 10 * time.Millisecond
	lock.AcquireRetryPause = 10 * time.Millisecond
}

func newTestServer(app api.App) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/1/apps/") && !strings.Contains(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode(app)

		case r.Method == "POST" && r.URL.Path == "/1/global_locks/acquire":
			json.NewEncoder(w).Encode(api.GlobalLock{Status: "allow"})

		default:
			// Return 400 for anything beyond the channel check to stop the workflow quickly
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"message": "mock: not implemented"})
		}
	}))
}

func TestPush_blocksChannelWithoutPermission(t *testing.T) {
	server := newTestServer(api.App{
		ID:                        1,
		Name:                      "Test Channel",
		IsChannel:                 true,
		AllowChannelDirectPublish: false,
	})
	defer server.Close()

	client := api.NewClient(server.URL, "testkey")
	client.RetryPause = time.Millisecond

	_, err := Push(context.Background(), &PushConfig{
		Client:      client,
		AppSecret:   "channel-secret",
		Label:       "1.0.0",
		FilesDir:    t.TempDir(),
		Mode:        "content",
		LockTimeout: 5 * time.Second,
	}, nil)

	if err == nil {
		t.Fatal("expected error for channel app without permission, got nil")
	}
	if !strings.Contains(err.Error(), "cannot upload directly to a channel app") {
		t.Errorf("expected channel-blocked error, got: %v", err)
	}
}

func TestPush_allowsChannelWithPermission(t *testing.T) {
	server := newTestServer(api.App{
		ID:                        1,
		Name:                      "Test Channel",
		IsChannel:                 true,
		AllowChannelDirectPublish: true,
	})
	defer server.Close()

	client := api.NewClient(server.URL, "testkey")
	client.RetryPause = time.Millisecond

	_, err := Push(context.Background(), &PushConfig{
		Client:      client,
		AppSecret:   "channel-secret",
		Label:       "1.0.0",
		FilesDir:    t.TempDir(),
		Mode:        "content",
		LockTimeout: 5 * time.Second,
	}, nil)

	// Workflow will fail later (mock returns 400 for version listing etc.)
	// but it must NOT fail with the channel-blocked error.
	if err != nil && strings.Contains(err.Error(), "cannot upload directly to a channel app") {
		t.Errorf("channel app with AllowChannelDirectPublish=true was blocked: %v", err)
	}
}

func TestPush_allowsNonChannelApp(t *testing.T) {
	server := newTestServer(api.App{
		ID:                        1,
		Name:                      "Regular App",
		IsChannel:                 false,
		AllowChannelDirectPublish: false,
	})
	defer server.Close()

	client := api.NewClient(server.URL, "testkey")
	client.RetryPause = time.Millisecond

	_, err := Push(context.Background(), &PushConfig{
		Client:      client,
		AppSecret:   "regular-secret",
		Label:       "1.0.0",
		FilesDir:    t.TempDir(),
		Mode:        "content",
		LockTimeout: 5 * time.Second,
	}, nil)

	// Must NOT fail with the channel-blocked error.
	if err != nil && strings.Contains(err.Error(), "cannot upload directly to a channel app") {
		t.Errorf("non-channel app was blocked by channel check: %v", err)
	}
}
