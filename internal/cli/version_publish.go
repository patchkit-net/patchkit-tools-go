package cli

import (
	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/spf13/cobra"
)

func newVersionPublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish a draft version",
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

			versionID, _ := cmd.Flags().GetInt("version")
			wait, _ := cmd.Flags().GetBool("wait")

			// If no version specified, find the current draft
			if versionID == 0 {
				versions, err := ac.client.GetVersions(ac.ctx, appSecret)
				if err != nil {
					ac.out.Error(err, "")
					return exitError(exitCodeFromError(err))
				}
				var drafts []int
				for _, v := range versions {
					if v.Draft {
						drafts = append(drafts, v.ID)
					}
				}
				if len(drafts) == 0 {
					ac.out.Error(errorf("no draft version found"), "Use pkt version create to create a draft first")
					return exitError(exitcode.NotFound)
				}
				if len(drafts) > 1 {
					ac.out.Error(errorf("multiple draft versions found, specify one with --version"), "")
					return exitError(exitcode.InvalidArguments)
				}
				versionID = drafts[0]
			}

			if ac.cfg.DryRun {
				ac.out.Infof("[Dry run] Would publish version v%d", versionID)
				return nil
			}

			ac.out.Infof("Publishing version v%d...", versionID)
			if err := ac.client.PublishVersion(ac.ctx, appSecret, versionID); err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			if wait {
				ac.out.StartProgress("Publishing", 100)
				err := ac.client.WaitForPublish(ac.ctx, appSecret, versionID, func(progress float64, msg string) {
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
					"published":  true,
				})
			} else {
				ac.out.Infof("Version v%d published successfully.", versionID)
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Application secret (env: PATCHKIT_APP)")
	cmd.Flags().IntP("version", "v", 0, "Version ID (omit to publish current draft)")
	cmd.Flags().BoolP("wait", "w", false, "Wait for publish to complete")
	return cmd
}

func errorf(format string, args ...interface{}) error {
	return &simpleError{msg: format}
}

type simpleError struct {
	msg string
}

func (e *simpleError) Error() string {
	return e.msg
}
