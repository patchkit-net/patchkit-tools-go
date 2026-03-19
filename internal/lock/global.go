package lock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/patchkit-net/patchkit-tools-go/internal/api"
)

var (
	PollInterval      = 30 * time.Second
	RefreshInterval   = 30 * time.Second
	SafetyCheckPause  = 60 * time.Second
	AcquireMaxRetries = 5
	AcquireRetryPause = 5 * time.Second
)

// GlobalLock manages a distributed lock with background refresh.
type GlobalLock struct {
	client   *api.Client
	resource string
	owner    string
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// StatusFn is called with lock status updates.
type StatusFn func(msg string)

// Acquire acquires a global lock, polling until allowed or timeout.
// The returned GlobalLock must be released by calling Release().
func Acquire(ctx context.Context, client *api.Client, resource string, timeout time.Duration, statusFn StatusFn) (*GlobalLock, error) {
	owner := uuid.New().String()
	deadline := time.Now().Add(timeout)

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		lock, err := acquireWithRetry(ctx, client, resource, owner)
		if err != nil {
			return nil, err
		}

		if lock.Status == "allow" {
			// Lock acquired - start background refresh
			refreshCtx, cancel := context.WithCancel(context.Background())
			gl := &GlobalLock{
				client:   client,
				resource: resource,
				owner:    owner,
				cancel:   cancel,
			}
			gl.wg.Add(1)
			go gl.refreshLoop(refreshCtx)
			return gl, nil
		}

		if time.Now().After(deadline) {
			return nil, &LockTimeoutError{Resource: resource, Timeout: timeout}
		}

		if statusFn != nil {
			statusFn(fmt.Sprintf("Waiting for lock on %s (queue position: %d)", resource, lock.QueuePosition))
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(PollInterval):
		}
	}
}

// AcquireForApp acquires a lock with app processing/publishing safety checks.
func AcquireForApp(ctx context.Context, client *api.Client, appSecret string, timeout time.Duration, statusFn StatusFn) (*GlobalLock, error) {
	deadline := time.Now().Add(timeout)

	// Safety check: wait until app is not processing/publishing
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		app, err := client.GetApp(ctx, appSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to check app status: %w", err)
		}

		if app.ProcessingVersion.Set {
			if time.Now().After(deadline) {
				return nil, &LockTimeoutError{Resource: appSecret, Timeout: timeout}
			}
			if statusFn != nil {
				statusFn("Application version is currently being processed. Checking again in 60 seconds...")
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(SafetyCheckPause):
			}
			continue
		}

		if app.PublishingVersion.Set {
			if time.Now().After(deadline) {
				return nil, &LockTimeoutError{Resource: appSecret, Timeout: timeout}
			}
			if statusFn != nil {
				statusFn("Application version is currently being published. Checking again in 60 seconds...")
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(SafetyCheckPause):
			}
			continue
		}

		break
	}

	// Acquire the lock
	gl, err := Acquire(ctx, client, appSecret, time.Until(deadline), statusFn)
	if err != nil {
		return nil, err
	}

	// Post-acquire safety check
	app, err := client.GetApp(ctx, appSecret)
	if err != nil {
		gl.Release()
		return nil, fmt.Errorf("post-lock safety check failed: %w", err)
	}

	if app.ProcessingVersion.Set || app.PublishingVersion.Set {
		gl.Release()
		return nil, fmt.Errorf("global lock safety check failed: app is still processing or publishing after lock acquisition")
	}

	return gl, nil
}

// Release stops the background refresh goroutine, allowing the lock to expire server-side.
func (gl *GlobalLock) Release() {
	if gl.cancel != nil {
		gl.cancel()
		gl.wg.Wait()
	}
}

func (gl *GlobalLock) refreshLoop(ctx context.Context) {
	defer gl.wg.Done()
	ticker := time.NewTicker(RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := acquireWithRetry(ctx, gl.client, gl.resource, gl.owner)
			if err != nil {
				// Log but don't crash - the lock will eventually expire
				continue
			}
		}
	}
}

func acquireWithRetry(ctx context.Context, client *api.Client, resource, owner string) (*api.GlobalLock, error) {
	var lastErr error
	for attempt := 0; attempt < AcquireMaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		lock, err := client.AcquireLock(ctx, resource, owner)
		if err == nil {
			return lock, nil
		}
		lastErr = err

		if attempt < AcquireMaxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(AcquireRetryPause):
			}
		}
	}
	return nil, fmt.Errorf("failed to acquire lock after %d attempts: %w", AcquireMaxRetries, lastErr)
}

// LockTimeoutError is returned when lock acquisition times out.
type LockTimeoutError struct {
	Resource string
	Timeout  time.Duration
}

func (e *LockTimeoutError) Error() string {
	return fmt.Sprintf("timeout waiting for lock on %s after %v", e.Resource, e.Timeout)
}
