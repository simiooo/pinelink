package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/simiooo/pinelink/cli/internal/client"
	"github.com/simiooo/pinelink/cli/internal/hashfile"
	"github.com/simiooo/pinelink/cli/internal/secure"
)

type uploadResult struct {
	EntryID   string `json:"entry_id"`
	VersionID string `json:"version_id"`
	TaskID    string `json:"task_id,omitempty"`
	Reused    bool   `json:"reused"`
}

func newUploadCommand(opts *options) *cobra.Command {
	var retention string
	var chunkSize, maxChunks int
	command := &cobra.Command{
		Use:  "upload FILE",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := os.Stat(args[0])
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("input is not a regular file")
			}
			plaintext, err := hashfile.Compute(args[0], chunkSize)
			if err != nil {
				return err
			}
			dedupKey := hashfile.DedupKey([]byte(opts.dedupSecret), plaintext.Size, plaintext.ContentRoot)
			identity, err := secure.LoadIdentity(opts.owner, opts.device)
			if err != nil {
				return err
			}
			ciphertextPath, envelopePath, err := secure.SpoolPaths(dedupKey, "library", opts.server, opts.owner, opts.device, opts.scope, opts.encryptionDomain)
			if err != nil {
				return err
			}
			if err := secure.AdoptLegacySpool(dedupKey, ciphertextPath, envelopePath, identity, plaintext.ContentRoot, plaintext.Size); err != nil {
				return err
			}
			envelope, err := os.ReadFile(envelopePath)
			if err != nil || fileMissing(ciphertextPath) {
				_ = os.Remove(ciphertextPath)
				envelope, err = secure.EncryptFile(args[0], ciphertextPath, plaintext.ContentRoot, plaintext.Size, filepath.Base(args[0]), identity)
				if err != nil {
					return err
				}
				if err := os.WriteFile(envelopePath, envelope, 0o600); err != nil {
					return err
				}
			}
			ciphertext, err := hashfile.Compute(ciphertextPath, chunkSize)
			if err != nil {
				return err
			}
			api, err := opts.client()
			if err != nil {
				return err
			}
			prepare, err := api.Prepare(cmd.Context(), client.PrepareRequest{
				ScopeID: opts.scope, EncryptionDomainID: opts.encryptionDomain,
				OwnerID: opts.owner, DeviceID: opts.device, FileName: "encrypted",
				RetentionClass: retention, HashVersion: "sha256-v1", DedupKey: dedupKey,
				ContentRoot: ciphertext.ContentRoot, SizeBytes: ciphertext.Size, ChunkSize: chunkSize,
				EncryptionSuite: secure.Suite, EncryptedManifest: envelope,
			})
			if err != nil {
				return err
			}
			var completed client.CompleteResponse
			switch prepare.Status {
			case "REUSE_AVAILABLE":
				manifest, err := secure.OpenManifest(prepare.Manifest.EncryptedManifest, identity)
				if err != nil {
					return fmt.Errorf("verify reused manifest: %w", err)
				}
				if manifest.PlaintextRoot != plaintext.ContentRoot || manifest.PlaintextSize != plaintext.Size {
					return fmt.Errorf("reused manifest plaintext does not match local file")
				}
				completed, err = api.CommitReuse(cmd.Context(), prepare.ReuseToken)
			case "UPLOAD_REQUIRED", "UPLOAD_IN_PROGRESS":
				if prepare.Manifest.ContentRoot != ciphertext.ContentRoot || prepare.Manifest.SizeBytes != ciphertext.Size ||
					prepare.Manifest.ChunkSize != chunkSize || prepare.Manifest.ChunkCount != len(ciphertext.ChunkHashes) {
					return fmt.Errorf("server upload does not match local spool: status=%s server=(root=%s size=%d chunk=%d count=%d) local=(root=%s size=%d chunk=%d count=%d)",
						prepare.Status, prepare.Manifest.ContentRoot, prepare.Manifest.SizeBytes, prepare.Manifest.ChunkSize, prepare.Manifest.ChunkCount,
						ciphertext.ContentRoot, ciphertext.Size, chunkSize, len(ciphertext.ChunkHashes))
				}
				received := make(map[int]bool, len(prepare.ReceivedChunks))
				for _, index := range prepare.ReceivedChunks {
					received[index] = true
				}
				if err = uploadMissingChunks(cmd, opts, ciphertextPath, prepare, ciphertext, received, maxChunks); err != nil {
					if maxChunks == 0 {
						markTaskFailed(cmd, opts, api, prepare.TaskID, err.Error())
					}
					return err
				}
				completed, err = api.CompleteUpload(cmd.Context(), prepare.UploadID)
			case "ENVELOPE_REQUIRED":
				return fmt.Errorf("an authorized device must create a manifest envelope before reuse")
			default:
				return fmt.Errorf("unknown prepare status %q", prepare.Status)
			}
			if err != nil {
				return err
			}
			_ = os.Remove(ciphertextPath)
			_ = os.Remove(envelopePath)
			result := uploadResult{EntryID: completed.File.ID, VersionID: completed.File.VersionID, TaskID: prepare.TaskID, Reused: completed.Reused}
			if opts.jsonOutput {
				return opts.output(result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "entry_id=%s\nversion_id=%s\ntask_id=%s\nreused=%t\n", result.EntryID, result.VersionID, result.TaskID, result.Reused)
			return nil
		},
	}
	command.Flags().StringVar(&retention, "retention", "TEMP_24H", "TEMP_24H or PERSISTENT")
	command.Flags().IntVar(&chunkSize, "chunk-size", 4<<20, "chunk size in bytes")
	command.Flags().IntVar(&maxChunks, "max-chunks", 0, "stop after this many chunks for a resumable partial upload")
	return command
}

func uploadMissingChunks(cmd *cobra.Command, opts *options, path string, prepare client.PrepareResponse, hashes hashfile.Result, received map[int]bool, maxChunks int) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	api, err := opts.client()
	if err != nil {
		return err
	}
	uploaded := 0
	for index, digest := range hashes.ChunkHashes {
		if received[index] {
			continue
		}
		if maxChunks > 0 && uploaded >= maxChunks {
			return fmt.Errorf("partial upload stopped at %d chunks; upload_id=%s task_id=%s", uploaded, prepare.UploadID, prepare.TaskID)
		}
		if prepare.TaskID != "" {
			task, err := api.GetTask(cmd.Context(), prepare.TaskID, opts.owner, opts.device)
			if err != nil {
				return err
			}
			if task.State != "RUNNING" {
				return fmt.Errorf("upload task is %s; resume it before continuing", task.State)
			}
		}
		offset := int64(index) * int64(prepare.Manifest.ChunkSize)
		length := prepare.Manifest.ChunkSize
		if index == len(hashes.ChunkHashes)-1 {
			length = int(hashes.Size - offset)
		}
		buffer := make([]byte, length)
		if _, err := file.ReadAt(buffer, offset); err != nil {
			return err
		}
		if err := api.UploadChunk(cmd.Context(), prepare.UploadID, index, digest, buffer); err != nil {
			return err
		}
		uploaded++
		if prepare.TaskID != "" {
			if _, err := api.TaskProgress(cmd.Context(), prepare.TaskID, opts.owner, opts.device, offset+int64(length)); err != nil {
				return err
			}
		}
	}
	return nil
}

func fileMissing(path string) bool {
	_, err := os.Stat(path)
	return err != nil
}
