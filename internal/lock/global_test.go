package lock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patchkit-net/patchkit-tools-go/internal/api"
)

func TestAcquire_immediateAllow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.GlobalLock{Status: "allow", QueuePosition: 0})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "testkey")
	gl, err := Acquire(context.Background(), client, "test-resource", 5*time.Second, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer gl.Release()

	if gl.resource != "test-resource" {
		t.Errorf("resource = %q, want %q", gl.resource, "test-resource")
	}
}

func TestAcquire_denyThenAllow(t *testing.T) {
	// Override intervals for test speed
	origPoll := PollInterval
	origRetry := AcquireRetryPause
	PollInterval = 10 * time.Millisecond
	AcquireRetryPause = time.Millisecond
	defer func() {
		PollInterval = origPoll
		AcquireRetryPause = origRetry
	}()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&calls, 1)
		if count <= 2 {
			json.NewEncoder(w).Encode(api.GlobalLock{Status: "deny", QueuePosition: int(3 - count)})
			return
		}
		json.NewEncoder(w).Encode(api.GlobalLock{Status: "allow", QueuePosition: 0})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "testkey")

	var messages []string
	gl, err := Acquire(context.Background(), client, "res", 10*time.Second, func(msg string) {
		messages = append(messages, msg)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer gl.Release()

	if len(messages) == 0 {
		t.Error("expected status messages during wait")
	}
}

func TestAcquire_timeout(t *testing.T) {
	origPoll := PollInterval
	origRetry := AcquireRetryPause
	PollInterval = 10 * time.Millisecond
	AcquireRetryPause = time.Millisecond
	defer func() {
		PollInterval = origPoll
		AcquireRetryPause = origRetry
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.GlobalLock{Status: "deny", QueuePosition: 5})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "testkey")
	_, err := Acquire(context.Background(), client, "res", 50*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}

	_, ok := err.(*LockTimeoutError)
	if !ok {
		t.Errorf("expected *LockTimeoutError, got %T: %v", err, err)
	}
}

func TestAcquire_contextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.GlobalLock{Status: "deny", QueuePosition: 1})
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := api.NewClient(server.URL, "testkey")
	_, err := Acquire(ctx, client, "res", 10*time.Second, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRelease_stopsRefresh(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(api.GlobalLock{Status: "allow", QueuePosition: 0})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "testkey")
	gl, err := Acquire(context.Background(), client, "res", 5*time.Second, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gl.Release()
	callsAfterRelease := atomic.LoadInt32(&calls)

	// Wait a bit and verify no more calls happen
	time.Sleep(50 * time.Millisecond)
	callsLater := atomic.LoadInt32(&calls)

	if callsLater != callsAfterRelease {
		t.Errorf("refresh continued after Release(): calls went from %d to %d", callsAfterRelease, callsLater)
	}
}

func TestLockTimeoutError(t *testing.T) {
	err := &LockTimeoutError{Resource: "myapp", Timeout: 30 * time.Minute}
	want := "timeout waiting for lock on myapp after 30m0s"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}
