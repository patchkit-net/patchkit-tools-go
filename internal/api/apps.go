package api

import (
	"context"
	"fmt"
)

// GetApp retrieves application details by secret.
func (c *Client) GetApp(ctx context.Context, secret string) (*App, error) {
	var app App
	if err := c.Get(ctx, fmt.Sprintf("1/apps/%s", secret), &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// ListApps lists all applications accessible to the API key.
func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	var apps []App
	if err := c.Get(ctx, "1/apps", &apps); err != nil {
		return nil, err
	}
	return apps, nil
}
