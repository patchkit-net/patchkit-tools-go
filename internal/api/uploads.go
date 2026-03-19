package api

import (
	"context"
	"fmt"
)

// CreateUpload creates a new upload session for S3 multi-part upload.
func (c *Client) CreateUpload(ctx context.Context, totalSize int64) (*Upload, error) {
	params := map[string]string{
		"storage_type":     "s3",
		"total_size_bytes": fmt.Sprintf("%d", totalSize),
	}
	var upload Upload
	if err := c.Post(ctx, "1/uploads", params, &upload); err != nil {
		return nil, err
	}
	return &upload, nil
}

// GenChunkURL generates a presigned URL for uploading a chunk.
func (c *Client) GenChunkURL(ctx context.Context, uploadID string, contentRange string) (*ChunkURL, error) {
	params := map[string]string{}
	headers := map[string]string{}
	if contentRange != "" {
		headers["Content-Range"] = contentRange
	}

	path := fmt.Sprintf("1/uploads/%s/gen_chunk_url", uploadID)

	// We need to do a POST with headers, so build it manually
	var chunkURL ChunkURL
	fullURL := c.buildURL(path)

	// Use doRequest directly since we need custom headers on POST
	err := c.doWithRetry(ctx, "POST", path, headers, params, &chunkURL)
	if err != nil {
		return nil, fmt.Errorf("failed to generate chunk URL (upload: %s, url: %s): %w", uploadID, fullURL, err)
	}
	return &chunkURL, nil
}
