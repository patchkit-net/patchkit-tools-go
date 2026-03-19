package workflow

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/patchkit-net/patchkit-tools-go/internal/api"
	"github.com/patchkit-net/patchkit-tools-go/internal/content"
	"github.com/patchkit-net/patchkit-tools-go/internal/diff"
	"github.com/patchkit-net/patchkit-tools-go/internal/lock"
	"github.com/patchkit-net/patchkit-tools-go/internal/native"
	"github.com/patchkit-net/patchkit-tools-go/internal/pack1"
	"github.com/patchkit-net/patchkit-tools-go/internal/upload"
)

// PushConfig contains all configuration for a version push workflow.
type PushConfig struct {
	Client      *api.Client
	AppSecret   string
	Label       string
	FilesDir    string
	Changelog   string
	Mode        string // "auto", "content", "diff", "diff-encrypted", "diff-fast"
	Publish     bool
	Wait        bool
	Overwrite   bool
	SkipProcess bool
	Retries     int
	LockTimeout time.Duration
	DiffThreads int
}

// PushResult contains the result of a push workflow.
type PushResult struct {
	VersionID int    `json:"version_id"`
	Label     string `json:"label"`
	Mode      string `json:"mode"`
	JobGUID   string `json:"job_guid,omitempty"`
	Published bool   `json:"published"`
}

// StatusFn is called with status updates during the workflow.
type StatusFn func(msg string)

// Push executes the full version push workflow.
func Push(ctx context.Context, cfg *PushConfig, statusFn StatusFn) (*PushResult, error) {
	if cfg.Wait {
		cfg.Publish = true
	}
	if cfg.Retries <= 0 {
		cfg.Retries = 5
	}
	if cfg.DiffThreads <= 0 {
		cfg.DiffThreads = 4
	}

	status := func(msg string) {
		if statusFn != nil {
			statusFn(msg)
		}
	}

	// Validate files path before acquiring lock to avoid holding the lock
	// for up to 30 minutes only to discover the path doesn't exist.
	if _, err := os.Stat(cfg.FilesDir); err != nil {
		return nil, fmt.Errorf("files path: %w", err)
	}

	// Step 1: Acquire lock
	status("Acquiring lock...")
	gl, err := lock.AcquireForApp(ctx, cfg.Client, cfg.AppSecret, cfg.LockTimeout, func(msg string) {
		status(msg)
	})
	if err != nil {
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}
	defer gl.Release()

	// Step 2: Get app info and find/create draft
	status("Checking application...")
	app, err := cfg.Client.GetApp(ctx, cfg.AppSecret)
	if err != nil {
		return nil, fmt.Errorf("getting app info: %w", err)
	}

	// Block direct uploads to channel apps unless the API grants permission.
	// Channel apps normally receive content via linking to a group version.
	if app.IsChannel && !app.AllowChannelDirectPublish {
		return nil, fmt.Errorf("cannot upload directly to a channel app. " +
			"Use 'pkt channel push' to link to a group version, or contact support to enable direct channel uploads")
	}

	versions, err := cfg.Client.GetVersions(ctx, cfg.AppSecret)
	if err != nil {
		return nil, fmt.Errorf("listing versions: %w", err)
	}

	// Find existing draft or create new one
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
		draftVersion.Label = cfg.Label
	}

	// Step 3: Detect mode
	mode := cfg.Mode
	if mode == "auto" {
		mode = detectMode(versions)
		status(fmt.Sprintf("Mode: %s (auto-detected)", mode))
	}

	// For pack1 modes (diff-encrypted, diff-fast), set publish_when_processed
	// before uploading, since the server auto-publishes after processing
	encrypted := mode == "diff-encrypted" || mode == "diff-fast"
	if encrypted && cfg.Publish {
		status("Setting publish_when_processed...")
		if err := cfg.Client.UpdateVersion(ctx, cfg.AppSecret, draftVersion.ID, map[string]string{
			"publish_when_processed": "true",
		}); err != nil {
			return nil, fmt.Errorf("setting publish_when_processed: %w", err)
		}
	}

	// Step 4: Build and upload
	tmpDir, err := os.MkdirTemp("", "pkt-push-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	var jobGUID string

	switch mode {
	case "content":
		jobGUID, err = pushContent(ctx, cfg, draftVersion.ID, tmpDir, statusFn)
	case "diff":
		jobGUID, err = pushDiff(ctx, cfg, app, draftVersion.ID, tmpDir, false, statusFn)
	case "diff-encrypted", "diff-fast":
		jobGUID, err = pushDiff(ctx, cfg, app, draftVersion.ID, tmpDir, true, statusFn)
	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}

	if err != nil {
		return nil, err
	}

	// Step 5: Wait for processing
	if !cfg.SkipProcess && jobGUID != "" {
		status("Waiting for server processing...")
		if err := cfg.Client.WaitForJob(ctx, jobGUID, func(progress float64, message string) {
			status(fmt.Sprintf("Processing: %s (%.0f%%)", message, progress*100))
		}); err != nil {
			return nil, fmt.Errorf("processing failed: %w", err)
		}
		status("Processing complete.")
	}

	// Step 5.5: Check for processing errors before publishing
	if cfg.Publish && !cfg.SkipProcess {
		ver, err := cfg.Client.GetVersion(ctx, cfg.AppSecret, draftVersion.ID)
		if err == nil && len(ver.ProcessingMessages) > 0 {
			hasBlockingError := false
			for _, msg := range ver.ProcessingMessages {
				if msg.Severity == "error" {
					status(fmt.Sprintf("Processing error: %s", strings.TrimSpace(msg.Message)))
					hasBlockingError = true
				} else if msg.Severity == "warning" {
					status(fmt.Sprintf("Processing warning: %s", strings.TrimSpace(msg.Message)))
				}
			}
			if hasBlockingError {
				return nil, fmt.Errorf("version has blocking processing errors — cannot publish. Check messages above or visit panel.patchkit.net")
			}
		}
	}

	// Step 6: Publish
	published := false
	if cfg.Publish {
		if encrypted {
			// For pack1 modes, server auto-publishes via publish_when_processed.
			// Wait for the version to become published.
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
		} else {
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
	}

	return &PushResult{
		VersionID: draftVersion.ID,
		Label:     cfg.Label,
		Mode:      mode,
		JobGUID:   jobGUID,
		Published: published,
	}, nil
}

func detectMode(versions []api.Version) string {
	for _, v := range versions {
		if !v.Draft {
			return "diff"
		}
	}
	return "content"
}

func pushContent(ctx context.Context, cfg *PushConfig, versionID int, tmpDir string, statusFn StatusFn) (string, error) {
	status := func(msg string) {
		if statusFn != nil {
			statusFn(msg)
		}
	}

	archivePath := filepath.Join(tmpDir, "content.zip")
	status("Packaging content...")
	packager := content.NewPackager()
	if err := packager.PackDir(cfg.FilesDir, archivePath); err != nil {
		return "", fmt.Errorf("packaging content: %w", err)
	}

	status("Uploading content...")
	uploadID, err := uploadFile(ctx, cfg, archivePath, statusFn)
	if err != nil {
		return "", fmt.Errorf("uploading content: %w", err)
	}

	status("Submitting for processing...")
	resp, err := cfg.Client.UploadContent(ctx, cfg.AppSecret, versionID, uploadID)
	if err != nil {
		return "", fmt.Errorf("submitting content: %w", err)
	}

	return resp.JobGUID, nil
}

func pushDiff(ctx context.Context, cfg *PushConfig, app *api.App, versionID int, tmpDir string, encrypted bool, statusFn StatusFn) (string, error) {
	status := func(msg string) {
		if statusFn != nil {
			statusFn(msg)
		}
	}

	// Find previous published version
	versions, err := cfg.Client.GetVersions(ctx, cfg.AppSecret)
	if err != nil {
		return "", fmt.Errorf("listing versions: %w", err)
	}

	var prevVersion *api.Version
	for i := len(versions) - 1; i >= 0; i-- {
		if !versions[i].Draft {
			prevVersion = &versions[i]
			break
		}
	}

	if prevVersion == nil {
		return "", fmt.Errorf("no previous published version found for diff mode")
	}

	// Download signatures
	status(fmt.Sprintf("Downloading signatures from v%d...", prevVersion.ID))
	sigZipPath := filepath.Join(tmpDir, "signatures.zip")
	sigFile, err := os.Create(sigZipPath)
	if err != nil {
		return "", err
	}
	err = cfg.Client.DownloadSignatures(ctx, cfg.AppSecret, prevVersion.ID, sigFile, nil)
	sigFile.Close()
	if err != nil {
		return "", fmt.Errorf("downloading signatures: %w", err)
	}

	sigsDir := filepath.Join(tmpDir, "signatures")
	if err := ExtractZip(sigZipPath, sigsDir); err != nil {
		return "", fmt.Errorf("extracting signatures: %w", err)
	}

	// Determine delta algorithm
	algorithm := native.AlgorithmLibrsync
	if app.DiffAlgorithm == "turbopatch" {
		algorithm = native.AlgorithmTurbopatch
	}

	// Get previous hashes for unchanged file detection
	var prevHashes map[string]string
	summary, err := cfg.Client.GetContentSummary(ctx, cfg.AppSecret, prevVersion.ID)
	if err == nil && summary != nil {
		prevHashes = summary.Files
	}

	// Run diff pipeline
	status("Computing diff...")
	diffResult, err := diff.Run(ctx, &diff.Config{
		ContentDir:     cfg.FilesDir,
		SignaturesDir:  sigsDir,
		TempDir:        tmpDir,
		Algorithm:      algorithm,
		Workers:        cfg.DiffThreads,
		PreviousHashes: prevHashes,
	})
	if err != nil {
		return "", fmt.Errorf("diff pipeline: %w", err)
	}

	status(fmt.Sprintf("Diff: %d added, %d modified, %d removed, %d unchanged",
		len(diffResult.Summary.AddedFiles),
		len(diffResult.Summary.ModifiedFiles),
		len(diffResult.Summary.RemovedFiles),
		len(diffResult.Summary.UnchangedFiles),
	))

	// Calculate uncompressed size (content directory size, matching Ruby behavior)
	uncompressedSize, err := dirSize(cfg.FilesDir)
	if err != nil {
		return "", fmt.Errorf("calculating content dir size: %w", err)
	}

	// Convert diff entries for packers
	pack1Entries := make(map[string]pack1.DeltaEntry, len(diffResult.DeltaFiles))
	contentEntries := make(map[string]content.DeltaEntry, len(diffResult.DeltaFiles))
	for name, de := range diffResult.DeltaFiles {
		pack1Entries[name] = pack1.DeltaEntry{FilePath: de.FilePath, Data: de.Data, Mode: de.Mode}
		contentEntries[name] = content.DeltaEntry{FilePath: de.FilePath, Data: de.Data, Mode: de.Mode}
	}

	// Package diff
	var archivePaths []string
	var pack1SHA1 string
	if encrypted {
		encKey := pack1.EncryptionKey(cfg.AppSecret, versionID)

		archivePath := filepath.Join(tmpDir, "diff.pack1")
		metaPath := filepath.Join(tmpDir, "diff.pack1.meta")

		packer, err := pack1.NewPacker(encKey)
		if err != nil {
			return "", fmt.Errorf("creating packer: %w", err)
		}

		_, err = packer.PackDeltaEntries(pack1Entries, archivePath, metaPath)
		if err != nil {
			return "", fmt.Errorf("pack1 packaging: %w", err)
		}

		// Compute SHA1 of the pack1 file (required by server for fast_diff processing)
		pack1SHA1, err = fileSHA1(archivePath)
		if err != nil {
			return "", fmt.Errorf("computing pack1 sha1: %w", err)
		}

		archivePaths = []string{archivePath, metaPath}

		diffResult.Summary.CompressionMethod = "pack1"
		diffResult.Summary.EncryptionMethod = "none"
	} else {
		archivePath := filepath.Join(tmpDir, "diff.zip")
		packager := content.NewPackager()
		if err := packager.PackDeltaEntries(contentEntries, archivePath); err != nil {
			return "", fmt.Errorf("zip packaging: %w", err)
		}
		archivePaths = []string{archivePath}

		diffResult.Summary.CompressionMethod = "zip"
		diffResult.Summary.EncryptionMethod = "none"
	}

	// Set archive size on summary (only first file = pack1 file, matching Ruby behavior)
	fi, err := os.Stat(archivePaths[0])
	if err != nil {
		return "", fmt.Errorf("stat archive: %w", err)
	}
	diffResult.Summary.Size = fi.Size()
	diffResult.Summary.UncompressedSize = uncompressedSize

	// Upload files
	status("Uploading diff...")
	var uploadIDs []string
	for _, path := range archivePaths {
		uid, err := uploadFile(ctx, cfg, path, statusFn)
		if err != nil {
			return "", fmt.Errorf("uploading diff: %w", err)
		}
		uploadIDs = append(uploadIDs, uid)
	}

	summaryJSON, err := diffResult.Summary.JSON()
	if err != nil {
		return "", err
	}

	// Submit to server
	status("Submitting diff for processing...")
	var resp *api.UploadResponse
	if len(uploadIDs) == 1 {
		resp, err = cfg.Client.UploadDiff(ctx, cfg.AppSecret, versionID, uploadIDs[0], string(summaryJSON))
	} else {
		resp, err = cfg.Client.UploadDiffMulti(ctx, cfg.AppSecret, versionID, uploadIDs, string(summaryJSON), pack1SHA1)
	}
	if err != nil {
		return "", fmt.Errorf("submitting diff: %w", err)
	}

	return resp.JobGUID, nil
}

func uploadFile(ctx context.Context, cfg *PushConfig, filePath string, statusFn StatusFn) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	uploader := upload.NewS3Uploader(cfg.Client, cfg.Retries)
	uploadID, err := uploader.Upload(ctx, filePath, info.Size(), func(bytesUploaded, totalBytes int64) {
		if statusFn != nil {
			statusFn(fmt.Sprintf("Uploading %.1f MB / %.1f MB",
				float64(bytesUploaded)/(1024*1024),
				float64(totalBytes)/(1024*1024)))
		}
	})
	if err != nil {
		return "", fmt.Errorf("uploading: %w", err)
	}

	return uploadID, nil
}

// ExtractZip extracts a ZIP file to a destination directory.
func ExtractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}

	return nil
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func fileSHA1(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
