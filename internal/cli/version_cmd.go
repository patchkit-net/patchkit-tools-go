package cli

import (
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Manage application versions",
	}
	cmd.AddCommand(
		newVersionPushCmd(),
		newVersionCreateCmd(),
		newVersionUpdateCmd(),
		newVersionPublishCmd(),
		newVersionListCmd(),
		newVersionUploadCmd(),
		newVersionImportCmd(),
		newVersionStatusCmd(),
	)
	return cmd
}
