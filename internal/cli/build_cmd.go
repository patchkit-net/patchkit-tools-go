package cli

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/patchkit-net/patchkit-tools-go/internal/content"
	"github.com/patchkit-net/patchkit-tools-go/internal/diff"
	"github.com/patchkit-net/patchkit-tools-go/internal/native"
	"github.com/patchkit-net/patchkit-tools-go/internal/pack1"
)

func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Create local build artifacts (diff, content, signatures)",
	}
	cmd.AddCommand(
		newBuildDiffCmd(),
		newBuildContentCmd(),
		newBuildSignaturesCmd(),
	)
	return cmd
}

func newBuildDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Create diff file (local operation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ac, err := newAppContext(cmd)
			if err != nil {
				return err
			}
			defer ac.cancel()

			filesDir, _ := cmd.Flags().GetString("files")
			sigFile, _ := cmd.Flags().GetString("signatures")
			output, _ := cmd.Flags().GetString("output")
			summaryPath, _ := cmd.Flags().GetString("summary")
			packaging, _ := cmd.Flags().GetString("packaging")
			algorithm, _ := cmd.Flags().GetString("delta-algorithm")
			encKey, _ := cmd.Flags().GetString("encryption-key")
			hashesFile, _ := cmd.Flags().GetString("previous-hashes")
			threads, _ := cmd.Flags().GetInt("threads")

			if filesDir == "" || sigFile == "" || output == "" {
				return fmt.Errorf("--files, --signatures, and --output are required")
			}

			if packaging == "pack1" && encKey == "" {
				return fmt.Errorf("--encryption-key is required for pack1 packaging")
			}

			// Extract signatures ZIP to temp dir
			tmpDir, err := os.MkdirTemp("", "pkt-diff-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmpDir)

			sigsDir := filepath.Join(tmpDir, "signatures")
			if err := extractZip(sigFile, sigsDir); err != nil {
				return fmt.Errorf("extracting signatures: %w", err)
			}

			deltaDir := filepath.Join(tmpDir, "deltas")

			// Load previous hashes if provided
			var prevHashes map[string]string
			if hashesFile != "" {
				data, err := os.ReadFile(hashesFile)
				if err != nil {
					return fmt.Errorf("reading hashes file: %w", err)
				}
				if err := json.Unmarshal(data, &prevHashes); err != nil {
					return fmt.Errorf("parsing hashes file: %w", err)
				}
			}

			// Run diff pipeline
			var deltaAlgorithm native.DeltaAlgorithm
			switch algorithm {
			case "turbopatch":
				deltaAlgorithm = native.AlgorithmTurbopatch
			default:
				deltaAlgorithm = native.AlgorithmLibrsync
			}

			ac.out.Info("Computing diff...")
			result, err := diff.Run(ac.ctx, &diff.Config{
				ContentDir:     filesDir,
				SignaturesDir:  sigsDir,
				OutputDir:      deltaDir,
				TempDir:        tmpDir,
				Algorithm:      deltaAlgorithm,
				Workers:        threads,
				PreviousHashes: prevHashes,
				ProgressFn: func(current, total int, fileName string) {
					ac.out.UpdateProgress(int64(current))
				},
			})
			if err != nil {
				return fmt.Errorf("diff pipeline: %w", err)
			}

			// Package results
			switch packaging {
			case "pack1":
				packer, err := pack1.NewPacker(encKey)
				if err != nil {
					return err
				}
				metaPath := output + ".meta"
				_, err = packer.PackFiles(result.DeltaFiles, output, metaPath)
				if err != nil {
					return fmt.Errorf("pack1 packaging: %w", err)
				}
				ac.out.Info(fmt.Sprintf("Pack1 archive: %s", output))
				ac.out.Info(fmt.Sprintf("Pack1 metadata: %s", metaPath))
			default:
				p := content.NewPackager()
				if err := p.PackFiles(result.DeltaFiles, output); err != nil {
					return fmt.Errorf("zip packaging: %w", err)
				}
			}

			// Write summary
			if summaryPath != "" {
				summaryJSON, err := result.Summary.JSONIndent()
				if err != nil {
					return err
				}
				if err := os.WriteFile(summaryPath, summaryJSON, 0644); err != nil {
					return err
				}
				ac.out.Info(fmt.Sprintf("Summary: %s", summaryPath))
			}

			ac.out.Info(fmt.Sprintf("Diff complete: %d added, %d modified, %d removed, %d unchanged",
				len(result.Summary.AddedFiles),
				len(result.Summary.ModifiedFiles),
				len(result.Summary.RemovedFiles),
				len(result.Summary.UnchangedFiles),
			))

			ac.out.Result(map[string]interface{}{
				"output":  output,
				"summary": result.Summary,
			})

			return nil
		},
	}
	cmd.Flags().StringP("signatures", "s", "", "Previous version signatures ZIP")
	cmd.Flags().StringP("files", "f", "", "New version files directory")
	cmd.Flags().StringP("output", "o", "", "Output diff file path")
	cmd.Flags().String("summary", "", "Output diff summary JSON path")
	cmd.Flags().String("packaging", "zip", "Packaging format: zip, pack1")
	cmd.Flags().String("delta-algorithm", "librsync", "Delta algorithm: librsync, turbopatch")
	cmd.Flags().String("encryption-key", "", "Pack1 encryption key (required for pack1)")
	cmd.Flags().String("previous-hashes", "", "JSON file with previous file hashes")
	cmd.Flags().IntP("threads", "t", 4, "Parallel diff threads")
	return cmd
}

func newBuildContentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "content",
		Short: "Pack directory into content archive (local operation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ac, err := newAppContext(cmd)
			if err != nil {
				return err
			}
			defer ac.cancel()

			filesDir, _ := cmd.Flags().GetString("files")
			output, _ := cmd.Flags().GetString("output")

			if filesDir == "" || output == "" {
				return fmt.Errorf("--files and --output are required")
			}

			ac.out.Info(fmt.Sprintf("Packaging %s...", filesDir))
			p := content.NewPackager()
			if err := p.PackDir(filesDir, output); err != nil {
				return fmt.Errorf("packaging content: %w", err)
			}

			info, _ := os.Stat(output)
			ac.out.Info(fmt.Sprintf("Content archive: %s (%d bytes)", output, info.Size()))

			ac.out.Result(map[string]interface{}{
				"output": output,
				"size":   info.Size(),
			})

			return nil
		},
	}
	cmd.Flags().StringP("files", "f", "", "Source directory")
	cmd.Flags().StringP("output", "o", "", "Output archive path")
	return cmd
}

func newBuildSignaturesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "signatures",
		Short: "Download version signatures",
		RunE: func(cmd *cobra.Command, args []string) error {
			ac, err := newAppContext(cmd)
			if err != nil {
				return err
			}
			defer ac.cancel()

			appSecret := ac.cfg.App
			if appSecret == "" {
				appSecret, _ = cmd.Flags().GetString("app")
			}
			if appSecret == "" {
				return fmt.Errorf("--app or PATCHKIT_APP is required")
			}

			versionID, _ := cmd.Flags().GetInt("version")
			output, _ := cmd.Flags().GetString("output")

			if versionID == 0 || output == "" {
				return fmt.Errorf("--version and --output are required")
			}

			ac.out.Info(fmt.Sprintf("Downloading signatures for version %d...", versionID))

			outFile, err := os.Create(output)
			if err != nil {
				return err
			}
			defer outFile.Close()

			err = ac.client.DownloadSignatures(ac.ctx, appSecret, versionID, outFile, func(bytesRead, totalBytes int64) {
				ac.out.UpdateProgress(bytesRead)
			})
			if err != nil {
				return fmt.Errorf("downloading signatures: %w", err)
			}

			outInfo, _ := os.Stat(output)
			ac.out.Info(fmt.Sprintf("Signatures saved: %s (%d bytes)", output, outInfo.Size()))

			ac.out.Result(map[string]interface{}{
				"output": output,
				"size":   outInfo.Size(),
			})

			return nil
		},
	}
	cmd.Flags().StringP("app", "a", "", "Application secret (env: PATCHKIT_APP)")
	cmd.Flags().IntP("version", "v", 0, "Version ID")
	cmd.Flags().StringP("output", "o", "", "Output file path")
	return cmd
}

// extractZip extracts a ZIP file to a destination directory.
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

