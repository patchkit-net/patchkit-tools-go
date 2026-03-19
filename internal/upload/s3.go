package upload

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/patchkit-net/patchkit-tools-go/internal/api"
)

const (
	chunkSize = 32 * 1024 * 1024 // 32 MB
)

// S3Uploader handles multi-part uploads via PatchKit's S3 broker.
type S3Uploader struct {
	client  *api.Client
	retries int
}

// NewS3Uploader creates a new S3 uploader.
func NewS3Uploader(client *api.Client, retries int) *S3Uploader {
	if retries <= 0 {
		retries = 5
	}
	return &S3Uploader{
		client:  client,
		retries: retries,
	}
}

// Upload uploads a file to S3 via the PatchKit upload broker.
// Returns the upload ID on success.
func (u *S3Uploader) Upload(ctx context.Context, filePath string, totalSize int64, progressFn func(uploaded int64, total int64)) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	// Create upload session
	upload, err := u.client.CreateUpload(ctx, totalSize)
	if err != nil {
		return "", fmt.Errorf("failed to create upload: %w", err)
	}

	// Upload chunks
	var offset int64
	for offset < totalSize {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		end := offset + chunkSize
		if end > totalSize {
			end = totalSize
		}
		currentChunkSize := end - offset

		contentRange := fmt.Sprintf("bytes %d-%d/%d", offset, end-1, totalSize)

		err := u.uploadChunkWithRetry(ctx, upload.ID.String(), f, offset, currentChunkSize, totalSize, contentRange, progressFn)
		if err != nil {
			return "", fmt.Errorf("failed to upload chunk at offset %d: %w", offset, err)
		}

		offset = end
	}

	return upload.ID.String(), nil
}

func (u *S3Uploader) uploadChunkWithRetry(ctx context.Context, uploadID string, f *os.File, offset int64, size int64, totalSize int64, contentRange string, progressFn func(int64, int64)) error {
	var lastErr error

	for attempt := 1; attempt <= u.retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = u.uploadChunk(ctx, uploadID, f, offset, size, totalSize, contentRange, progressFn)
		if lastErr == nil {
			return nil
		}

		if attempt < u.retries {
			backoff := retryBackoff(attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return lastErr
}

func (u *S3Uploader) uploadChunk(ctx context.Context, uploadID string, f *os.File, offset int64, size int64, totalSize int64, contentRange string, progressFn func(int64, int64)) error {
	// Get presigned URL
	chunkURL, err := u.client.GenChunkURL(ctx, uploadID, contentRange)
	if err != nil {
		return err
	}

	// Seek to offset
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	// Create limited reader for chunk
	reader := io.LimitReader(f, size)

	// Wrap with progress tracking
	if progressFn != nil {
		reader = &uploadProgressReader{
			reader:     reader,
			offset:     offset,
			totalSize:  totalSize,
			progressFn: progressFn,
		}
	}

	// Determine headers
	headers := map[string]string{}
	if strings.Contains(chunkURL.URL, "localhost") || strings.Contains(chunkURL.URL, "127.0.0.1") {
		headers["Content-Type"] = "application/octet-stream"
	} else {
		// S3 requires explicit empty Content-Type
		headers["Content-Type"] = ""
	}
	if strings.Contains(chunkURL.URL, "s3-accelerate.amazonaws.com") {
		headers["x-amz-acl"] = "bucket-owner-full-control"
	}

	return u.client.PutRawBody(ctx, chunkURL.URL, reader, size, headers)
}

// retryBackoff calculates exponential backoff with jitter.
// Base: 2s, multiplier: 2x, cap: 60s, jitter: +/- 20%
func retryBackoff(attempt int) time.Duration {
	base := 2.0
	multiplier := math.Pow(2, float64(attempt-1))
	seconds := base * multiplier

	if seconds > 60 {
		seconds = 60
	}

	// Add jitter: +/- 20%
	jitter := seconds * 0.2 * (2*rand.Float64() - 1)
	seconds += jitter

	return time.Duration(seconds * float64(time.Second))
}

// uploadProgressReader wraps a reader to report upload progress.
type uploadProgressReader struct {
	reader     io.Reader
	offset     int64
	bytesRead  int64
	totalSize  int64
	progressFn func(uploaded int64, total int64)
}

func (r *uploadProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += int64(n)
	if r.progressFn != nil {
		r.progressFn(r.offset+r.bytesRead, r.totalSize)
	}
	return n, err
}
