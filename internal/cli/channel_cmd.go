package cli

import (
	"github.com/spf13/cobra"

	"github.com/patchkit-net/patchkit-tools-go/internal/config"
	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/patchkit-net/patchkit-tools-go/internal/workflow"
)

func newChannelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "Manage channel versions",
	}
	cmd.AddCommand(
		newChannelPushCmd(),
		newChannelLinkCmd(),
	)
	return cmd
}

func newChannelPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Create channel version linked to a group version",
		Long: `Creates a channel version linked to a group application version.
Resolves the group version, creates a draft, links it, and optionally publishes.`,
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

			changelog, _ := cmd.Flags().GetString("changelog")
			changelog, err = config.ReadChangelog(changelog)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitcode.InvalidArguments)
			}

			groupVersion, _ := cmd.Flags().GetInt("group-version")
			useLatest, _ := cmd.Flags().GetBool("latest")
			overwrite, _ := cmd.Flags().GetBool("overwrite-draft")
			publish, _ := cmd.Flags().GetBool("publish")
			wait, _ := cmd.Flags().GetBool("wait")

			lockTimeout, err := resolveLockTimeout(cmd, ac.cfg)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitcode.InvalidArguments)
			}

			if groupVersion == 0 && !useLatest {
				ac.out.Error(errorf("--group-version or --latest is required"), "")
				return exitError(exitcode.InvalidArguments)
			}

			if ac.cfg.DryRun {
				if useLatest {
					ac.out.Infof("[Dry run] Would create channel version with label %q using latest group version", label)
				} else {
					ac.out.Infof("[Dry run] Would create channel version with label %q linked to group v%d", label, groupVersion)
				}
				return nil
			}

			result, err := workflow.ChannelPush(ac.ctx, &workflow.ChannelPushConfig{
				Client:       ac.client,
				AppSecret:    appSecret,
				Label:        label,
				Changelog:    changelog,
				GroupVersion: groupVersion,
				UseLatest:    useLatest,
				Overwrite:    overwrite,
				Publish:      publish,
				Wait:         wait,
				LockTimeout:  lockTimeout,
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
				ac.out.Infof("Channel version v%d created (linked to group v%d).", result.VersionID, result.GroupVersion)
				if result.Published {
					ac.out.Info("Version is published.")
				}
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Channel application secret (env: PATCHKIT_APP)")
	cmd.Flags().StringP("label", "l", "", "Version label")
	cmd.Flags().IntP("group-version", "g", 0, "Group version ID")
	cmd.Flags().Bool("latest", false, "Use latest published group version")
	cmd.Flags().StringP("changelog", "c", "", "Changelog text or @path/to/file")
	cmd.Flags().Bool("overwrite-draft", false, "Overwrite existing draft")
	cmd.Flags().BoolP("publish", "p", false, "Publish when done")
	cmd.Flags().BoolP("wait", "w", false, "Wait for publish (implies --publish)")
	cmd.Flags().String("lock-timeout", config.DefaultLockTimeout.String(), "Max time to wait for global lock")
	return cmd
}

func newChannelLinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Link channel version to group version",
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

			groupApp, _ := cmd.Flags().GetString("group-app")
			if groupApp == "" {
				ac.out.Error(errorf("--group-app is required"), "")
				return exitError(exitcode.InvalidArguments)
			}

			groupVersion, _ := cmd.Flags().GetInt("group-version")
			if groupVersion == 0 {
				ac.out.Error(errorf("--group-version is required"), "")
				return exitError(exitcode.InvalidArguments)
			}

			if ac.cfg.DryRun {
				ac.out.Infof("[Dry run] Would link channel version v%d to group version v%d", versionID, groupVersion)
				return nil
			}

			resp, err := ac.client.LinkVersion(ac.ctx, appSecret, versionID, groupApp, groupVersion)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			if ac.cfg.Format == "json" {
				ac.out.Result(map[string]interface{}{
					"version_id":    versionID,
					"group_version": groupVersion,
					"job_guid":      resp.JobGUID,
				})
			} else {
				ac.out.Infof("Linked channel version v%d to group version v%d.", versionID, groupVersion)
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Channel application secret (env: PATCHKIT_APP)")
	cmd.Flags().IntP("version", "v", 0, "Channel version ID")
	cmd.Flags().String("group-app", "", "Group application secret")
	cmd.Flags().IntP("group-version", "g", 0, "Group version ID")
	return cmd
}
