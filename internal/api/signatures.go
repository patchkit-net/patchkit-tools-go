package api

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// GetSignaturesInfo retrieves the signatures download URL and size.
func (c *Client) GetSignaturesInfo(ctx context.Context, appSecret string, versionID int) (*SignaturesInfo, error) {
	var info SignaturesInfo
	if err := c.Get(ctx, fmt.Sprintf("1/apps/%s/versions/%d/signatures/url", appSecret, versionID), &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// DownloadSignatures downloads version signatures to the writer.
// It supports three download paths:
//  1. S3 presigned URL (url contains "//s3.")
//  2. CDN multi-part download (url with .0, .1, etc. suffixes)
//  3. Direct API fallback (when size is 0)
func (c *Client) DownloadSignatures(ctx context.Context, appSecret string, versionID int, w io.Writer, progressFn func(bytesRead int64, totalBytes int64)) error {
	info, err := c.GetSignaturesInfo(ctx, appSecret, versionID)
	if err != nil {
		return err
	}

	if info.Size == 0 {
		// Fallback to direct API download
		return c.downloadSignaturesDirect(ctx, appSecret, versionID, w, progressFn)
	}

	if strings.Contains(info.URL, "//s3.") {
		// S3 presigned URL - single file download
		return c.downloadSignaturesS3(ctx, info.URL, info.Size, w, progressFn)
	}

	// CDN multi-part download
	return c.downloadSignaturesCDN(ctx, info.URL, info.Size, w, progressFn)
}

func (c *Client) downloadSignaturesS3(ctx context.Context, url string, size int64, w io.Writer, progressFn func(int64, int64)) error {
	wrappedProgress := func(bytesRead int64) {
		if progressFn != nil {
			progressFn(bytesRead, size)
		}
	}
	return c.GetStream(ctx, url, nil, w, wrappedProgress)
}

func (c *Client) downloadSignaturesCDN(ctx context.Context, baseURL string, totalSize int64, w io.Writer, progressFn func(int64, int64)) error {
	const partSize int64 = 512 * 1024 * 1024 // 512 MB

	parts := totalSize / partSize
	if totalSize%partSize != 0 {
		parts++
	}

	var totalRead int64
	for part := int64(0); part < parts; part++ {
		url := baseURL
		if part > 0 {
			url = fmt.Sprintf("%s.%d", baseURL, part)
		}

		wrappedProgress := func(bytesRead int64) {
			if progressFn != nil {
				progressFn(totalRead+bytesRead, totalSize)
			}
		}

		partWriter := &countingWriter{w: w, count: &totalRead}
		if err := c.GetStream(ctx, url, nil, partWriter, wrappedProgress); err != nil {
			return fmt.Errorf("failed to download signatures part %d: %w", part, err)
		}
	}

	return nil
}

func (c *Client) downloadSignaturesDirect(ctx context.Context, appSecret string, versionID int, w io.Writer, progressFn func(int64, int64)) error {
	path := fmt.Sprintf("1/apps/%s/versions/%d/signatures", appSecret, versionID)
	fullURL := c.buildURL(path)

	wrappedProgress := func(bytesRead int64) {
		if progressFn != nil {
			progressFn(bytesRead, 0)
		}
	}
	return c.GetStream(ctx, fullURL, nil, w, wrappedProgress)
}

// countingWriter wraps a writer and tracks total bytes written.
type countingWriter struct {
	w     io.Writer
	count *int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	*cw.count += int64(n)
	return n, err
}
