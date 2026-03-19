package cli

import (
	"fmt"

	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/spf13/cobra"
)

func newAppCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Inspect applications",
	}
	cmd.AddCommand(newAppInfoCmd(), newAppListCmd())
	return cmd
}

func newAppInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show app details",
		Long:  "Show application details including platform, channel status, and diff algorithm.",
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

			app, err := ac.client.GetApp(ac.ctx, appSecret)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			if ac.cfg.Format == "json" {
				ac.out.Result(app)
			} else {
				processing := "no"
				if app.ProcessingVersion.Set {
					processing = fmt.Sprintf("v%d", app.ProcessingVersion.Value)
				}
				publishing := "no"
				if app.PublishingVersion.Set {
					publishing = fmt.Sprintf("v%d", app.PublishingVersion.Value)
				}
				channel := "no"
				if app.IsChannel {
					channel = "yes"
				}

				ac.out.Infof("App: %s", app.Name)
				ac.out.Infof("Platform: %s", app.Platform)
				ac.out.Infof("Channel: %s", channel)
				ac.out.Infof("Diff algorithm: %s", app.DiffAlgorithm)
				ac.out.Infof("Processing: %s", processing)
				ac.out.Infof("Publishing: %s", publishing)
				ac.out.Infof("Versions: %d published, %d draft", app.PublishedVersions, app.DraftVersions)
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Application secret (env: PATCHKIT_APP)")
	return cmd
}

func newAppListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List apps accessible to the API key",
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

			apps, err := ac.client.ListApps(ac.ctx)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			if ac.cfg.Format == "json" {
				ac.out.Result(apps)
			} else {
				if len(apps) == 0 {
					ac.out.Info("No applications found.")
					return nil
				}
				// Table output
				fmt.Printf("%-30s %-20s %-10s\n", "NAME", "PLATFORM", "CHANNEL")
				fmt.Printf("%-30s %-20s %-10s\n", "----", "--------", "-------")
				for _, app := range apps {
					channel := "no"
					if app.IsChannel {
						channel = "yes"
					}
					fmt.Printf("%-30s %-20s %-10s\n", app.Name, app.Platform, channel)
				}
			}

			return nil
		},
	}
	return cmd
}
