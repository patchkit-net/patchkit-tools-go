package cli

import (
	"github.com/spf13/cobra"

	"github.com/patchkit-net/patchkit-tools-go/internal/config"
	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/patchkit-net/patchkit-tools-go/internal/workflow"
)

func newVersionPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Build, upload, and optionally publish a new version",
		Long: `The primary workflow command. Creates a draft version, uploads content or diff,
waits for server processing, and optionally publishes.`,
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

			label, _ := cmd.Flags().GetString("label")
			if label == "" {
				ac.out.Error(errorf("--label is required"), "")
				return exitError(exitcode.InvalidArguments)
			}

			filesDir, _ := cmd.Flags().GetString("files")
			if filesDir == "" {
				ac.out.Error(errorf("--files is required"), "")
				return exitError(exitcode.InvalidArguments)
			}

			changelog, _ := cmd.Flags().GetString("changelog")
			changelog, err = config.ReadChangelog(changelog)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitcode.InvalidArguments)
			}

			mode, _ := cmd.Flags().GetString("mode")
			publish, _ := cmd.Flags().GetBool("publish")
			wait, _ := cmd.Flags().GetBool("wait")
			overwrite, _ := cmd.Flags().GetBool("overwrite-draft")
			skipProcess, _ := cmd.Flags().GetBool("skip-processing")
			retries, _ := cmd.Flags().GetInt("retries")

			lockTimeout, err := resolveLockTimeout(cmd, ac.cfg)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitcode.InvalidArguments)
			}

			if ac.cfg.DryRun {
				ac.out.Infof("[Dry run] Would push version with label %q from %s (mode: %s)", label, filesDir, mode)
				return nil
			}

			result, err := workflow.Push(ac.ctx, &workflow.PushConfig{
				Client:      ac.client,
				AppSecret:   appSecret,
				Label:       label,
				FilesDir:    filesDir,
				Changelog:   changelog,
				Mode:        mode,
				Publish:     publish,
				Wait:        wait,
				Overwrite:   overwrite,
				SkipProcess: skipProcess,
				Retries:     retries,
				LockTimeout: lockTimeout,
			}, func(msg string) {
				ac.out.Info(msg)
			})

			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			if ac.cfg.Format == "json" {
				ac.out.Result(result)
			} else {
				ac.out.Infof("Version v%d pushed successfully (mode: %s).", result.VersionID, result.Mode)
				if result.Published {
					ac.out.Info("Version is published.")
				}
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Application secret (env: PATCHKIT_APP)")
	cmd.Flags().StringP("label", "l", "", "Version label")
	cmd.Flags().StringP("files", "f", "", "Path to files directory or APK")
	cmd.Flags().StringP("changelog", "c", "", "Changelog text or @path/to/file")
	cmd.Flags().StringP("mode", "m", "auto", "Upload mode: auto, content, diff, diff-encrypted, diff-fast")
	cmd.Flags().BoolP("publish", "p", false, "Publish after processing")
	cmd.Flags().BoolP("wait", "w", false, "Wait for publish (implies --publish)")
	cmd.Flags().Bool("overwrite-draft", false, "Overwrite existing draft")
	cmd.Flags().Bool("skip-processing", false, "Don't wait for server processing")
	cmd.Flags().Int("retries", 5, "Total upload attempts including initial")
	cmd.Flags().String("lock-timeout", config.DefaultLockTimeout.String(), "Max time to wait for global lock")
	return cmd
}
