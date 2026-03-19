package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show configuration",
	}
	cmd.AddCommand(newConfigShowCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show resolved configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			ac, err := newAppContext(cmd)
			if err != nil {
				return err
			}
			defer ac.cancel()

			values := ac.cfg.GetResolvedValues()

			if ac.cfg.Format == "json" {
				result := map[string]interface{}{
					"config_files": ac.cfg.LoadedFiles,
					"values":       values,
				}
				ac.out.Result(result)
			} else {
				fmt.Println("Config files:")
				for _, f := range ac.cfg.LoadedFiles {
					fmt.Printf("  %s: %s", f.Path, f.Status)
					if f.Error != "" {
						fmt.Printf(" (%s)", f.Error)
					}
					fmt.Println()
				}

				fmt.Println()
				fmt.Println("Resolved values:")
				for key, val := range values {
					if val.Value == "" {
						fmt.Printf("  %-16s (not set)\n", key+":")
					} else {
						fmt.Printf("  %-16s %s (from: %s)\n", key+":", val.Value, val.Source)
					}
				}
			}

			return nil
		},
	}
	return cmd
}
