package api

import (
	"context"
	"fmt"
	"time"
)

// GetJobStatus retrieves the status of a background processing job.
func (c *Client) GetJobStatus(ctx context.Context, jobGUID string) (*JobStatus, error) {
	var status JobStatus
	if err := c.Get(ctx, fmt.Sprintf("1/background_jobs/%s", jobGUID), &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// WaitForJob polls a background job until completion, calling progressFn on each update.
func (c *Client) WaitForJob(ctx context.Context, jobGUID string, progressFn func(progress float64, message string)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		start := time.Now()

		status, err := c.GetJobStatus(ctx, jobGUID)
		if err != nil {
			if progressFn != nil {
				progressFn(0, fmt.Sprintf("WARNING: Cannot read job status - %v. Will try again...", err))
			}
		} else {
			msg := status.StatusMessage
			if status.Status == 0 {
				if status.Pending {
					msg = "Pending"
				}
				if status.Finished {
					msg = "Done"
				}
			}

			if progressFn != nil {
				progressFn(status.Progress, msg)
			}

			if status.Finished {
				if status.Status != 0 {
					return &JobError{Message: fmt.Sprintf("%s. Please visit panel.patchkit.net for more information.", msg)}
				}
				return nil
			}
		}

		elapsed := time.Since(start)
		remaining := time.Second - elapsed
		if remaining > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(remaining):
			}
		}
	}
}

// WaitForPublish polls version status until published.
func (c *Client) WaitForPublish(ctx context.Context, appSecret string, versionID int, progressFn func(progress float64, message string)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		start := time.Now()

		version, err := c.GetVersion(ctx, appSecret, versionID)
		if err != nil {
			if progressFn != nil {
				progressFn(0, fmt.Sprintf("WARNING: Cannot read publishing status (%v). Will try again...", err))
			}
		} else {
			if version.Published {
				if progressFn != nil {
					progressFn(1.0, "Version has been published!")
				}
				return nil
			}

			if !version.PendingPublish {
				return &PublishError{Message: "Unable to publish version. Please visit panel.patchkit.net for more information."}
			}

			if progressFn != nil {
				progressFn(version.PublishProgress, "Publishing...")
			}
		}

		elapsed := time.Since(start)
		remaining := time.Second - elapsed
		if remaining > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(remaining):
			}
		}
	}
}
