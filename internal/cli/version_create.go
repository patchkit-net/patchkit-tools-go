package cli

import (
	"fmt"

	"github.com/patchkit-net/patchkit-tools-go/internal/config"
	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/spf13/cobra"
)

func newVersionCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new draft version",
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
				ac.out.Error(fmt.Errorf("version label is required"), "Use --label to specify a version label")
				return exitError(exitcode.InvalidArguments)
			}

			if ac.cfg.DryRun {
				ac.out.Infof("[Dry run] Would create draft version with label %q for app %s", label, appSecret)
				return nil
			}

			resp, err := ac.client.CreateVersion(ac.ctx, appSecret, label)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			// Update changelog if provided
			changelog, _ := cmd.Flags().GetString("changelog")
			if changelog != "" {
				changelogText, err := config.ReadChangelog(changelog)
				if err != nil {
					ac.out.Error(err, "")
					return exitError(exitcode.GeneralError)
				}
				updates := map[string]string{"changelog": changelogText}
				if err := ac.client.UpdateVersion(ac.ctx, appSecret, resp.ID, updates); err != nil {
					ac.out.Warnf("Version created but failed to set changelog: %v", err)
				}
			}

			if ac.cfg.Format == "json" {
				ac.out.Result(map[string]interface{}{
					"version_id": resp.ID,
					"label":      label,
					"status":     "draft",
				})
			} else {
				ac.out.Infof("Created version v%d", resp.ID)
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Application secret (env: PATCHKIT_APP)")
	cmd.Flags().StringP("label", "l", "", "Version label")
	cmd.Flags().StringP("changelog", "c", "", "Changelog text or @path/to/file")
	return cmd
}
