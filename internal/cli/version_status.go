package cli

import (
	"fmt"

	"github.com/patchkit-net/patchkit-tools-go/internal/api"
	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/spf13/cobra"
)

func newVersionStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check version processing/publish status",
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

			version, err := ac.client.GetVersion(ac.ctx, appSecret, versionID)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			if ac.cfg.Format == "json" {
				ac.out.Result(map[string]interface{}{
					"version_id": versionID,
					"state":      versionState(version),
					"progress":   version.ProcessingProgress,
					"published":  version.Published,
				})
			} else {
				state := versionState(version)
				if state == "processing" {
					ac.out.Infof("Version v%d: %s (%.0f%%)", versionID, state, version.ProcessingProgress*100)
				} else {
					ac.out.Infof("Version v%d: %s", versionID, state)
				}

				if version.HasProcessingError && len(version.ProcessingMessages) > 0 {
					for _, msg := range version.ProcessingMessages {
						ac.out.Warnf("  [%s] %s", msg.Severity, msg.Message)
					}
				}
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Application secret (env: PATCHKIT_APP)")
	cmd.Flags().IntP("version", "v", 0, "Version ID")
	return cmd
}

func versionState(v *api.Version) string {
	if v.Published {
		return "published"
	}
	if v.PendingPublish {
		return fmt.Sprintf("publishing (%.0f%%)", v.PublishProgress*100)
	}
	if v.HasProcessingError {
		return "processing_error"
	}
	if v.Draft {
		if v.ProcessingProgress > 0 && v.ProcessingProgress < 1.0 {
			return "processing"
		}
		return "draft"
	}
	return "unknown"
}
