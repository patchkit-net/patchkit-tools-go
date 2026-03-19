package api

import (
	"context"
	"fmt"
)

// GetVersions lists versions for an application.
func (c *Client) GetVersions(ctx context.Context, appSecret string) ([]Version, error) {
	var versions []Version
	if err := c.Get(ctx, fmt.Sprintf("1/apps/%s/versions", appSecret), &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

// GetVersion retrieves a specific version.
func (c *Client) GetVersion(ctx context.Context, appSecret string, versionID int) (*Version, error) {
	var version Version
	if err := c.Get(ctx, fmt.Sprintf("1/apps/%s/versions/%d", appSecret, versionID), &version); err != nil {
		return nil, err
	}
	return &version, nil
}

// CreateVersion creates a new draft version. Returns version ID.
func (c *Client) CreateVersion(ctx context.Context, appSecret string, label string) (*VersionCreateResponse, error) {
	params := map[string]string{
		"label": label,
	}
	var resp VersionCreateResponse
	if err := c.Post(ctx, fmt.Sprintf("1/apps/%s/versions", appSecret), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateVersion updates version metadata.
func (c *Client) UpdateVersion(ctx context.Context, appSecret string, versionID int, updates map[string]string) error {
	return c.Patch(ctx, fmt.Sprintf("1/apps/%s/versions/%d", appSecret, versionID), updates, nil)
}

// PublishVersion publishes a version.
func (c *Client) PublishVersion(ctx context.Context, appSecret string, versionID int) error {
	return c.Put(ctx, fmt.Sprintf("1/apps/%s/versions/%d/publish", appSecret, versionID), nil, nil)
}

// UploadContent submits a content upload for a version.
func (c *Client) UploadContent(ctx context.Context, appSecret string, versionID int, uploadID string) (*UploadResponse, error) {
	params := map[string]string{
		"upload_id": uploadID,
	}
	var resp UploadResponse
	if err := c.Put(ctx, fmt.Sprintf("1/apps/%s/versions/%d/content_file", appSecret, versionID), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UploadDiff submits a diff upload for a version.
func (c *Client) UploadDiff(ctx context.Context, appSecret string, versionID int, uploadID string, diffSummary string) (*UploadResponse, error) {
	params := map[string]string{
		"upload_id":    uploadID,
		"diff_summary": diffSummary,
	}
	var resp UploadResponse
	if err := c.Put(ctx, fmt.Sprintf("1/apps/%s/versions/%d/diff_file", appSecret, versionID), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UploadDiffMulti submits a multi-file diff upload (for Pack1 format).
func (c *Client) UploadDiffMulti(ctx context.Context, appSecret string, versionID int, uploadIDs []string, diffSummary string, sha1 string) (*UploadResponse, error) {
	fields := []KeyValue{
		{Key: "diff_summary", Value: diffSummary},
	}
	for _, id := range uploadIDs {
		fields = append(fields, KeyValue{Key: "upload_id[]", Value: id})
	}
	if sha1 != "" {
		fields = append(fields, KeyValue{Key: "sha1", Value: sha1})
	}
	var resp UploadResponse
	if err := c.PutMulti(ctx, fmt.Sprintf("1/apps/%s/versions/%d/diff_file", appSecret, versionID), fields, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetPack1Key retrieves the pack1 encryption key for a version.
func (c *Client) GetPack1Key(ctx context.Context, appSecret string, versionID int) (string, error) {
	var resp Pack1KeyResponse
	if err := c.Get(ctx, fmt.Sprintf("1/apps/%s/versions/%d/pack1_key", appSecret, versionID), &resp); err != nil {
		return "", err
	}
	return resp.Key, nil
}

// GetContentSummary retrieves the content file hashes for a version.
func (c *Client) GetContentSummary(ctx context.Context, appSecret string, versionID int) (*ContentSummary, error) {
	var summary ContentSummary
	if err := c.Get(ctx, fmt.Sprintf("1/apps/%s/versions/%d/content_summary", appSecret, versionID), &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

// ImportVersion imports a version from another application.
func (c *Client) ImportVersion(ctx context.Context, appSecret string, versionID int, sourceAppSecret string, sourceVersionID int) (*ImportResponse, error) {
	params := map[string]string{
		"source_app_secret": sourceAppSecret,
		"source_vid":        fmt.Sprintf("%d", sourceVersionID),
	}
	var resp ImportResponse
	if err := c.Post(ctx, fmt.Sprintf("1/apps/%s/versions/%d/import", appSecret, versionID), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// LinkVersion links a channel version to a group version.
func (c *Client) LinkVersion(ctx context.Context, appSecret string, versionID int, sourceAppSecret string, sourceVersionID int) (*LinkResponse, error) {
	params := map[string]string{
		"source_app_secret":      sourceAppSecret,
		"source_app_version_id":  fmt.Sprintf("%d", sourceVersionID),
	}
	var resp LinkResponse
	if err := c.Post(ctx, fmt.Sprintf("1/apps/%s/versions/%d/link", appSecret, versionID), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
