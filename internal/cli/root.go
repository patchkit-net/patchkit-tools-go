package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/patchkit-net/patchkit-tools-go/internal/api"
	"github.com/patchkit-net/patchkit-tools-go/internal/config"
	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/patchkit-net/patchkit-tools-go/internal/output"
	"github.com/spf13/cobra"
)

var (
	// Set via ldflags at build time.
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// appContext holds shared state for all commands.
type appContext struct {
	cfg    *config.Config
	client *api.Client
	out    output.Outputter
	ctx    context.Context
	cancel context.CancelFunc
}

func newAppContext(cmd *cobra.Command) (*appContext, error) {
	config.BindFlags(cmd)

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Apply flag-only settings
	cfg.Format, _ = cmd.Root().PersistentFlags().GetString("format")
	cfg.Quiet, _ = cmd.Root().PersistentFlags().GetBool("quiet")
	cfg.Verbose, _ = cmd.Root().PersistentFlags().GetBool("verbose")
	cfg.DryRun, _ = cmd.Root().PersistentFlags().GetBool("dry-run")
	cfg.Interactive, _ = cmd.Root().PersistentFlags().GetBool("interactive")
	cfg.ProgressEvents, _ = cmd.Root().PersistentFlags().GetBool("progress-events")

	debugFlag, _ := cmd.Root().PersistentFlags().GetBool("debug")
	if debugFlag {
		cfg.Debug = true
	}

	mode := output.ModeText
	if cfg.Format == "json" {
		mode = output.ModeJSON
	}
	out := output.New(mode, cfg.Quiet, cfg.ProgressEvents)

	// Check gitignore warning
	if warning := config.CheckGitignoreWarning(); warning != "" {
		out.Warn(warning)
	}

	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		out.Warn("Interrupted.")
		cancel()
	}()

	client := api.NewClient(cfg.APIURL, cfg.APIKey)
	client.Debug = cfg.Debug
	if cfg.Debug {
		client.DebugLog = func(msg string) {
			fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", msg)
		}
	}

	return &appContext{
		cfg:    cfg,
		client: client,
		out:    out,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// NewRootCommand creates the root cobra command.
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "pkt",
		Short: "PatchKit Tools - Version management CLI",
		Long:  "PatchKit Tools - Build, upload, and publish game versions.",
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, Date),
	}

	// Global flags
	pf := rootCmd.PersistentFlags()
	pf.StringP("api-key", "k", "", "API key (env: PATCHKIT_API_KEY)")
	pf.String("api-url", "", "API URL (env: PATCHKIT_API_URL)")
	pf.StringP("format", "F", "text", "Output format: text, json")
	pf.BoolP("quiet", "q", false, "Suppress informational output")
	pf.Bool("verbose", false, "Show additional context")
	pf.Bool("debug", false, "Enable debug output (env: PATCHKIT_DEBUG)")
	pf.Bool("dry-run", false, "Show what would happen without making changes")
	pf.BoolP("interactive", "i", false, "Guided interactive mode")
	pf.Bool("progress-events", false, "Emit NDJSON progress events to stderr")

	// Register subcommands
	rootCmd.AddCommand(
		newVersionCmd(),
		newBuildCmd(),
		newChannelCmd(),
		newAppCmd(),
		newConfigCmd(),
	)

	return rootCmd
}

// Execute runs the root command.
func Execute() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		code := exitcode.GeneralError
		if e, ok := err.(*exitErr); ok {
			code = e.code
		} else if apiErr, ok := err.(*api.APIError); ok {
			code = apiErr.ExitCode()
		}
		os.Exit(code)
	}
}

// requireApp validates and returns the app secret from flags/config.
func requireApp(cmd *cobra.Command, cfg *config.Config) (string, error) {
	app, _ := cmd.Flags().GetString("app")
	if app == "" {
		app = cfg.App
	}
	if app == "" {
		return "", fmt.Errorf("application secret is required. Set PATCHKIT_APP or use --app")
	}
	return app, nil
}

// requireVersion returns the version ID from flags.
func requireVersion(cmd *cobra.Command) (int, error) {
	v, _ := cmd.Flags().GetInt("version")
	if v == 0 {
		return 0, fmt.Errorf("version ID is required. Use --version")
	}
	return v, nil
}
