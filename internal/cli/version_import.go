package cli

import (
	"github.com/spf13/cobra"

	"github.com/patchkit-net/patchkit-tools-go/internal/config"
	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/patchkit-net/patchkit-tools-go/internal/lock"
)

func newVersionImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import version from another application",
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

			fromApp, _ := cmd.Flags().GetString("from-app")
			fromVersion, _ := cmd.Flags().GetInt("from-version")
			label, _ := cmd.Flags().GetString("label")
			copyLabel, _ := cmd.Flags().GetBool("copy-label")
			copyChangelog, _ := cmd.Flags().GetBool("copy-changelog")
			overwrite, _ := cmd.Flags().GetBool("overwrite-draft")
			publish, _ := cmd.Flags().GetBool("publish")
			wait, _ := cmd.Flags().GetBool("wait")
			skipProcess, _ := cmd.Flags().GetBool("skip-processing")

			lockTimeout, err := resolveLockTimeout(cmd, ac.cfg)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitcode.InvalidArguments)
			}

			if fromApp == "" {
				ac.out.Error(errorf("--from-app is required"), "")
				return exitError(exitcode.InvalidArguments)
			}
			if fromVersion == 0 {
				ac.out.Error(errorf("--from-version is required"), "")
				return exitError(exitcode.InvalidArguments)
			}
			if label == "" && !copyLabel {
				ac.out.Error(errorf("--label or --copy-label is required"), "")
				return exitError(exitcode.InvalidArguments)
			}

			// Fetch source version metadata if copying label or changelog
			var changelog string
			if copyLabel || copyChangelog {
				srcVersion, err := ac.client.GetVersion(ac.ctx, fromApp, fromVersion)
				if err != nil {
					ac.out.Error(err, "")
					return exitError(exitCodeFromError(err))
				}
				if copyLabel && label == "" {
					label = srcVersion.Label
				}
				if copyChangelog {
					changelog = srcVersion.Changelog
				}
			}

			if wait {
				publish = true
			}

			if ac.cfg.DryRun {
				ac.out.Infof("[Dry run] Would import version from %s v%d to %s", fromApp, fromVersion, appSecret)
				return nil
			}

			// Acquire lock
			ac.out.Info("Acquiring lock...")
			gl, err := lock.AcquireForApp(ac.ctx, ac.client, appSecret, lockTimeout, func(msg string) {
				ac.out.Info(msg)
			})
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitcode.LockTimeout)
			}
			defer gl.Release()

			// Find or create draft
			versions, err := ac.client.GetVersions(ac.ctx, appSecret)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			var draftID int
			for _, v := range versions {
				if v.Draft {
					if !overwrite {
						ac.out.Error(errorf("draft version v%d already exists (use --overwrite-draft)", v.ID), "")
						return exitError(exitcode.Conflict)
					}
					draftID = v.ID
					break
				}
			}

			if draftID == 0 {
				ac.out.Info("Creating draft version...")
				resp, err := ac.client.CreateVersion(ac.ctx, appSecret, label)
				if err != nil {
					ac.out.Error(err, "")
					return exitError(exitCodeFromError(err))
				}
				draftID = resp.ID

				if changelog != "" {
					if err := ac.client.UpdateVersion(ac.ctx, appSecret, draftID, map[string]string{"changelog": changelog}); err != nil {
						ac.out.Error(err, "")
						return exitError(exitCodeFromError(err))
					}
				}
			} else {
				updates := map[string]string{"label": label}
				if changelog != "" {
					updates["changelog"] = changelog
				}
				if err := ac.client.UpdateVersion(ac.ctx, appSecret, draftID, updates); err != nil {
					ac.out.Error(err, "")
					return exitError(exitCodeFromError(err))
				}
			}

			// Import
			ac.out.Infof("Importing from %s v%d...", fromApp, fromVersion)
			importResp, err := ac.client.ImportVersion(ac.ctx, appSecret, draftID, fromApp, fromVersion)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			// Wait for processing
			if !skipProcess && importResp.JobGUID != "" {
				ac.out.Info("Waiting for processing...")
				if err := ac.client.WaitForJob(ac.ctx, importResp.JobGUID, func(progress float64, message string) {
					ac.out.Infof("Processing: %s (%.0f%%)", message, progress*100)
				}); err != nil {
					ac.out.Error(err, "")
					return exitError(exitcode.ProcessingError)
				}
				ac.out.Info("Processing complete.")
			}

			// Publish
			if publish {
				ac.out.Info("Publishing...")
				if err := ac.client.PublishVersion(ac.ctx, appSecret, draftID); err != nil {
					ac.out.Error(err, "")
					return exitError(exitCodeFromError(err))
				}
				if wait {
					ac.out.Info("Waiting for publish...")
					if err := ac.client.WaitForPublish(ac.ctx, appSecret, draftID, func(progress float64, message string) {
						ac.out.Infof("Publishing: %s (%.0f%%)", message, progress*100)
					}); err != nil {
						ac.out.Error(err, "")
						return exitError(exitcode.ProcessingError)
					}
				}
				ac.out.Info("Published.")
			}

			if ac.cfg.Format == "json" {
				ac.out.Result(map[string]interface{}{
					"version_id":   draftID,
					"label":        label,
					"from_app":     fromApp,
					"from_version": fromVersion,
					"job_guid":     importResp.JobGUID,
					"published":    publish,
				})
			} else {
				ac.out.Infof("Version v%d imported successfully.", draftID)
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Target application secret (env: PATCHKIT_APP)")
	cmd.Flags().String("from-app", "", "Source application secret")
	cmd.Flags().Int("from-version", 0, "Source version ID")
	cmd.Flags().StringP("label", "l", "", "Version label (or use --copy-label)")
	cmd.Flags().Bool("copy-label", false, "Copy label from source")
	cmd.Flags().Bool("copy-changelog", false, "Copy changelog from source")
	cmd.Flags().Bool("overwrite-draft", false, "Overwrite existing draft")
	cmd.Flags().BoolP("publish", "p", false, "Publish after import")
	cmd.Flags().BoolP("wait", "w", false, "Wait for publish (implies --publish)")
	cmd.Flags().Bool("skip-processing", false, "Don't wait for server processing")
	cmd.Flags().String("lock-timeout", config.DefaultLockTimeout.String(), "Max time to wait for global lock")
	return cmd
}
