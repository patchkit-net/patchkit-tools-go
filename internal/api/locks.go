package api

import (
	"context"
	"fmt"
)

const (
	lockAcquireMaxRetries = 5
	lockAcquireRetryPause = 5 // seconds
)

// AcquireLock attempts to acquire a global processing lock.
func (c *Client) AcquireLock(ctx context.Context, resource string, owner string) (*GlobalLock, error) {
	params := map[string]string{
		"resource": resource,
		"owner":    owner,
	}
	var lock GlobalLock
	if err := c.Post(ctx, "1/global_locks/acquire", params, &lock); err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	return &lock, nil
}

// RefreshLock refreshes an existing lock (same call as acquire with same owner).
func (c *Client) RefreshLock(ctx context.Context, resource string, owner string) error {
	_, err := c.AcquireLock(ctx, resource, owner)
	return err
}
