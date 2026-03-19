package cli

import (
	"fmt"
	"os"

	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/patchkit-net/patchkit-tools-go/internal/upload"
	"github.com/spf13/cobra"
)

func newVersionUploadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload content or diff file (low-level)",
		Long: `Low-level plumbing command. Uploads a pre-built content or diff file to an existing draft version.

For the high-level workflow, use "pkt version push" instead.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ac, err := newAppContext(cmd)
			if err != nil {
				return err
			}
			defer ac.cancel()

			if err := ac.cfg.RequireAPIKey(); err != nil {
				ac.out.Error(err, "")
				return exitError(exitcode.InvalidArguments)
			}

			appSecret, err := requireApp(cmd, ac.cfg)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitcode.InvalidArguments)
			}

			versionID, err := requireVersion(cmd)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitcode.InvalidArguments)
			}

			mode, _ := cmd.Flags().GetString("mode")
			if mode == "" {
				ac.out.Error(fmt.Errorf("upload mode is required"), "Use --mode content or --mode diff")
				return exitError(exitcode.InvalidArguments)
			}

			files, _ := cmd.Flags().GetStringSlice("file")
			if len(files) == 0 {
				ac.out.Error(fmt.Errorf("at least one file is required"), "Use --file <path>")
				return exitError(exitcode.InvalidArguments)
			}

			diffSummary, _ := cmd.Flags().GetString("diff-summary")
			if mode == "diff" && diffSummary == "" {
				ac.out.Error(fmt.Errorf("--diff-summary is required for diff mode"), "")
				return exitError(exitcode.InvalidArguments)
			}

			retries, _ := cmd.Flags().GetInt("retries")
			wait, _ := cmd.Flags().GetBool("wait")

			if ac.cfg.DryRun {
				ac.out.Infof("[Dry run] Would upload %d file(s) to version v%d in %s mode", len(files), versionID, mode)
				return nil
			}

			// Upload each file
			var uploadIDs []string
			for _, filePath := range files {
				fi, err := os.Stat(filePath)
				if err != nil {
					ac.out.Error(fmt.Errorf("cannot access file %s: %w", filePath, err), "")
					return exitError(exitcode.InvalidArguments)
				}

				ac.out.Infof("Uploading %s...", filePath)
				uploader := upload.NewS3Uploader(ac.client, retries)
				uploadID, err := uploader.Upload(ac.ctx, filePath, fi.Size(), func(uploaded int64, total int64) {
					ac.out.UpdateProgress(uploaded)
				})
				if err != nil {
					ac.out.Error(err, fmt.Sprintf("Upload failed. Draft v%d exists but upload is incomplete.", versionID))
					return exitError(exitcode.UploadFailed)
				}
				uploadIDs = append(uploadIDs, uploadID)
			}

			// Submit to API
			var jobGUID string
			switch mode {
			case "content":
				resp, err := ac.client.UploadContent(ac.ctx, appSecret, versionID, uploadIDs[0])
				if err != nil {
					ac.out.Error(err, "")
					return exitError(exitCodeFromError(err))
				}
				jobGUID = resp.JobGUID
			case "diff":
				summaryData, err := os.ReadFile(diffSummary)
				if err != nil {
					ac.out.Error(fmt.Errorf("cannot read diff summary: %w", err), "")
					return exitError(exitcode.GeneralError)
				}
				if len(uploadIDs) == 1 {
					resp, err := ac.client.UploadDiff(ac.ctx, appSecret, versionID, uploadIDs[0], string(summaryData))
					if err != nil {
						ac.out.Error(err, "")
						return exitError(exitCodeFromError(err))
					}
					jobGUID = resp.JobGUID
				} else {
					resp, err := ac.client.UploadDiffMulti(ac.ctx, appSecret, versionID, uploadIDs, string(summaryData), "")
					if err != nil {
						ac.out.Error(err, "")
						return exitError(exitCodeFromError(err))
					}
					jobGUID = resp.JobGUID
				}
			}

			// Wait for processing job
			if wait && jobGUID != "" {
				ac.out.StartProgress("Processing", 100)
				err := ac.client.WaitForJob(ac.ctx, jobGUID, func(progress float64, msg string) {
					ac.out.UpdateProgress(int64(progress * 100))
					ac.out.UpdateProgressMessage(msg)
				})
				ac.out.EndProgress()
				if err != nil {
					ac.out.Error(err, "")
					return exitError(exitCodeFromError(err))
				}
			}

			if ac.cfg.Format == "json" {
				ac.out.Result(map[string]interface{}{
					"version_id": versionID,
					"upload_ids": uploadIDs,
					"job_guid":   jobGUID,
				})
			} else {
				ac.out.Infof("Upload complete for version v%d.", versionID)
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Application secret (env: PATCHKIT_APP)")
	cmd.Flags().IntP("version", "v", 0, "Version ID (must be draft)")
	cmd.Flags().StringP("mode", "m", "", "Upload mode: content, diff")
	cmd.Flags().StringSlice("file", nil, "File to upload (repeatable)")
	cmd.Flags().String("diff-summary", "", "Diff summary file (required for diff mode)")
	cmd.Flags().String("sha1", "", "SHA1 digest for integrity verification")
	cmd.Flags().BoolP("wait", "w", false, "Wait for processing job")
	cmd.Flags().Int("retries", 5, "Total upload attempts including initial")
	return cmd
}
