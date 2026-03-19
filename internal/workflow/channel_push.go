package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/patchkit-net/patchkit-tools-go/internal/api"
	"github.com/patchkit-net/patchkit-tools-go/internal/lock"
)

// ChannelPushConfig contains configuration for the channel push workflow.
type ChannelPushConfig struct {
	Client       *api.Client
	AppSecret    string
	Label        string
	Changelog    string
	GroupVersion int  // 0 means use latest
	UseLatest    bool // Use latest published group version
	Overwrite    bool
	Publish      bool
	Wait         bool
	LockTimeout  time.Duration
}

// ChannelPushResult contains the result of a channel push.
type ChannelPushResult struct {
	VersionID    int    `json:"version_id"`
	Label        string `json:"label"`
	GroupVersion int    `json:"group_version"`
	JobGUID      string `json:"job_guid,omitempty"`
	Published    bool   `json:"published"`
}

// ChannelPush creates a channel version linked to a group version.
func ChannelPush(ctx context.Context, cfg *ChannelPushConfig, statusFn StatusFn) (*ChannelPushResult, error) {
	if cfg.Wait {
		cfg.Publish = true
	}

	status := func(msg string) {
		if statusFn != nil {
			statusFn(msg)
		}
	}

	// Verify app is a channel
	status("Checking application...")
	app, err := cfg.Client.GetApp(ctx, cfg.AppSecret)
	if err != nil {
		return nil, fmt.Errorf("getting app info: %w", err)
	}

	if !app.IsChannel {
		return nil, fmt.Errorf("application is not a channel (use 'version push' for regular apps)")
	}

	if app.ParentGroup == nil || app.ParentGroup.Secret == "" {
		return nil, fmt.Errorf("channel has no group application configured")
	}

	// Acquire lock
	status("Acquiring lock...")
	gl, err := lock.AcquireForApp(ctx, cfg.Client, cfg.AppSecret, cfg.LockTimeout, func(msg string) {
		status(msg)
	})
	if err != nil {
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}
	defer gl.Release()

	// Resolve group version
	groupVersionID := cfg.GroupVersion
	if cfg.UseLatest || groupVersionID == 0 {
		status("Finding latest group version...")
		groupVersions, err := cfg.Client.GetVersions(ctx, app.ParentGroup.Secret)
		if err != nil {
			return nil, fmt.Errorf("listing group versions: %w", err)
		}

		var latest *api.Version
		for i := len(groupVersions) - 1; i >= 0; i-- {
			if !groupVersions[i].Draft {
				latest = &groupVersions[i]
				break
			}
		}

		if latest == nil {
			return nil, fmt.Errorf("no published group version found")
		}
		groupVersionID = latest.ID
		status(fmt.Sprintf("Using group version v%d", groupVersionID))
	}

	// Find or create draft
	versions, err := cfg.Client.GetVersions(ctx, cfg.AppSecret)
	if err != nil {
		return nil, fmt.Errorf("listing versions: %w", err)
	}

	var draftVersion *api.Version
	for i := range versions {
		if versions[i].Draft {
			draftVersion = &versions[i]
			break
		}
	}

	if draftVersion != nil && !cfg.Overwrite {
		return nil, fmt.Errorf("draft version v%d already exists (use --overwrite-draft to replace)", draftVersion.ID)
	}

	if draftVersion == nil {
		status("Creating draft version...")
		resp, err := cfg.Client.CreateVersion(ctx, cfg.AppSecret, cfg.Label)
		if err != nil {
			return nil, fmt.Errorf("creating version: %w", err)
		}
		draftVersion = &api.Version{ID: resp.ID, Label: cfg.Label, Draft: true}

		if cfg.Changelog != "" {
			if err := cfg.Client.UpdateVersion(ctx, cfg.AppSecret, resp.ID, map[string]string{"changelog": cfg.Changelog}); err != nil {
				return nil, fmt.Errorf("setting changelog: %w", err)
			}
		}
	} else {
		status(fmt.Sprintf("Updating draft version v%d...", draftVersion.ID))
		updates := map[string]string{"label": cfg.Label}
		if cfg.Changelog != "" {
			updates["changelog"] = cfg.Changelog
		}
		if err := cfg.Client.UpdateVersion(ctx, cfg.AppSecret, draftVersion.ID, updates); err != nil {
			return nil, fmt.Errorf("updating version: %w", err)
		}
	}

	// Link to group version
	status(fmt.Sprintf("Linking to group version v%d...", groupVersionID))
	linkResp, err := cfg.Client.LinkVersion(ctx, cfg.AppSecret, draftVersion.ID, app.ParentGroup.Secret, groupVersionID)
	if err != nil {
		return nil, fmt.Errorf("linking version: %w", err)
	}

	// Wait for processing
	if linkResp.JobGUID != "" {
		status("Waiting for processing...")
		if err := cfg.Client.WaitForJob(ctx, linkResp.JobGUID, func(progress float64, message string) {
			status(fmt.Sprintf("Processing: %s (%.0f%%)", message, progress*100))
		}); err != nil {
			return nil, fmt.Errorf("processing failed: %w", err)
		}
		status("Processing complete.")
	}

	// Publish
	published := false
	if cfg.Publish {
		status("Publishing...")
		if err := cfg.Client.PublishVersion(ctx, cfg.AppSecret, draftVersion.ID); err != nil {
			return nil, fmt.Errorf("publishing: %w", err)
		}

		if cfg.Wait {
			status("Waiting for publish...")
			if err := cfg.Client.WaitForPublish(ctx, cfg.AppSecret, draftVersion.ID, func(progress float64, message string) {
				status(fmt.Sprintf("Publishing: %s (%.0f%%)", message, progress*100))
			}); err != nil {
				return nil, fmt.Errorf("waiting for publish: %w", err)
			}
		}
		published = true
		status("Published.")
	}

	return &ChannelPushResult{
		VersionID:    draftVersion.ID,
		Label:        cfg.Label,
		GroupVersion: groupVersionID,
		JobGUID:      linkResp.JobGUID,
		Published:    published,
	}, nil
}
