package cli

import (
	"fmt"
	"sort"

	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
	"github.com/spf13/cobra"
)

func newVersionListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List application versions",
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

			versions, err := ac.client.GetVersions(ac.ctx, appSecret)
			if err != nil {
				ac.out.Error(err, "")
				return exitError(exitCodeFromError(err))
			}

			includeDrafts, _ := cmd.Flags().GetBool("include-drafts")
			if !includeDrafts {
				var filtered []interface{}
				for i := range versions {
					if versions[i].Published {
						filtered = append(filtered, versions[i])
					}
				}
				// Re-type for output
				var published []struct {
					ID    int
					Label string
				}
				for _, v := range versions {
					if v.Published {
						published = append(published, struct {
							ID    int
							Label string
						}{v.ID, v.Label})
					}
				}
			}

			// Sort
			sortOrder, _ := cmd.Flags().GetString("sort")
			sort.Slice(versions, func(i, j int) bool {
				if sortOrder == "asc" {
					return versions[i].ID < versions[j].ID
				}
				return versions[i].ID > versions[j].ID
			})

			// Filter drafts
			if !includeDrafts {
				var filtered = versions[:0]
				for _, v := range versions {
					if v.Published || !v.Draft {
						filtered = append(filtered, v)
					}
				}
				versions = filtered
			}

			// Limit
			limit, _ := cmd.Flags().GetInt("limit")
			if limit > 0 && len(versions) > limit {
				versions = versions[:limit]
			}

			if ac.cfg.Format == "json" {
				ac.out.Result(versions)
			} else {
				if len(versions) == 0 {
					ac.out.Info("No versions found.")
					return nil
				}
				fmt.Printf("%-8s %-20s %-12s\n", "ID", "LABEL", "STATUS")
				fmt.Printf("%-8s %-20s %-12s\n", "--", "-----", "------")
				for _, v := range versions {
					status := "published"
					if v.Draft {
						status = "draft"
					} else if v.PendingPublish {
						status = "publishing"
					}
					fmt.Printf("%-8d %-20s %-12s\n", v.ID, v.Label, status)
				}
			}

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Application secret (env: PATCHKIT_APP)")
	cmd.Flags().IntP("limit", "n", 0, "Max versions to show (0 = all)")
	cmd.Flags().String("sort", "desc", "Sort order: asc, desc")
	cmd.Flags().Bool("include-drafts", false, "Include draft versions")
	return cmd
}
