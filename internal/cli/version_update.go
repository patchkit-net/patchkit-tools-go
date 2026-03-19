package cli

import (
	"fmt"

	"github.com/patchkit-net/patchkit-tools-go/internal/config"
	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/spf13/cobra"
)

func newVersionUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update version metadata",
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

			label, _ := cmd.Flags().GetString("label")
			changelog, _ := cmd.Flags().GetString("changelog")

			if label == "" && changelog == "" {
				ac.out.Error(fmt.Errorf("at least one of --label or --changelog is required"), "")
				return exitError(exitcode.InvalidArguments)
			}

			updates := make(map[string]string)
			if label != "" {
				updates["label"] = label
			}
			if changelog != "" {
				changelogText, err := config.ReadChangelog(changelog)
				if err != nil {
					ac.out.Error(err, "")
					return exitError(exitcode.GeneralError)
				}
				updates["changelog"] = changelogText
			}

			if ac.cfg.DryRun {
				ac.out.Infof("[Dry run] Would update version v%d: %v", versionID, updates)
				return nil
			}

			if err := ac.client.UpdateVersion(ac.ctx, appSecret, versionID, updates); err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			if ac.cfg.Format == "json" {
				ac.out.Result(map[string]interface{}{
					"version_id": versionID,
					"updated":    updates,
				})
			} else {
				ac.out.Infof("Updated version v%d", versionID)
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Application secret (env: PATCHKIT_APP)")
	cmd.Flags().IntP("version", "v", 0, "Version ID")
	cmd.Flags().StringP("label", "l", "", "New label")
	cmd.Flags().StringP("changelog", "c", "", "New changelog text or @path/to/file")
	return cmd
}
