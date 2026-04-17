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

// newVersionAwareTestServer extends newTestServer to return the given version
// list from GET /1/apps/:secret/versions, so the workflow can reach mode
// detection without crashing earlier.
func newVersionAwareTestServer(app api.App, versions []api.Version) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode(versions)

		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/1/apps/") && !strings.Contains(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode(app)

		case r.Method == "POST" && r.URL.Path == "/1/global_locks/acquire":
			json.NewEncoder(w).Encode(api.GlobalLock{Status: "allow"})

		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/versions"):
			json.NewEncoder(w).Encode(map[string]int{"id": 999})

		default:
			// 400 to stop the workflow before it tries actual uploads
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"message": "mock: not implemented"})
		}
	}))
}

// When the caller pins a diff mode but the app has no previously published
// version, the workflow must fall back to content mode instead of failing with
// "no previous published version found for diff mode".
func TestPush_fallsBackToContentOnFirstVersion(t *testing.T) {
	server := newVersionAwareTestServer(
		api.App{ID: 1, Name: "Fresh app", IsChannel: false},
		[]api.Version{}, // no versions yet — first push
	)
	defer server.Close()

	client := api.NewClient(server.URL, "testkey")
	client.RetryPause = time.Millisecond

	for _, mode := range []string{"diff", "diff-encrypted", "diff-fast"} {
		t.Run(mode, func(t *testing.T) {
			var messages []string
			_, _ = Push(context.Background(), &PushConfig{
				Client:      client,
				AppSecret:   "fresh-secret",
				Label:       "1.0.0",
				FilesDir:    t.TempDir(),
				Mode:        mode,
				LockTimeout: 5 * time.Second,
			}, func(msg string) { messages = append(messages, msg) })

			fallbackSeen := false
			for _, m := range messages {
				if strings.Contains(m, "falling back from "+mode+" to content mode") {
					fallbackSeen = true
					break
				}
			}
			if !fallbackSeen {
				t.Errorf("expected fallback status message for mode=%s, got: %v", mode, messages)
			}
			for _, m := range messages {
				if strings.Contains(m, "Setting publish_when_processed") {
					t.Errorf("publish_when_processed must not be set after fallback to content (mode=%s)", mode)
				}
			}
		})
	}
}

// If the app already has a published version, a pinned diff mode should be
// respected (no fallback).
func TestPush_keepsDiffModeWhenPublishedVersionExists(t *testing.T) {
	server := newVersionAwareTestServer(
		api.App{ID: 1, Name: "App", IsChannel: false},
		[]api.Version{{ID: 1, Draft: false, Published: true}},
	)
	defer server.Close()

	client := api.NewClient(server.URL, "testkey")
	client.RetryPause = time.Millisecond

	var messages []string
	_, _ = Push(context.Background(), &PushConfig{
		Client:      client,
		AppSecret:   "secret",
		Label:       "1.0.1",
		FilesDir:    t.TempDir(),
		Mode:        "diff-fast",
		LockTimeout: 5 * time.Second,
	}, func(msg string) { messages = append(messages, msg) })

	for _, m := range messages {
		if strings.Contains(m, "falling back") {
			t.Errorf("fallback fired unexpectedly when a published version exists; messages=%v", messages)
		}
	}
}
