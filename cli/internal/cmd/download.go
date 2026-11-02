package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/simiooo/pinelink/cli/internal/client"
	"github.com/simiooo/pinelink/cli/internal/hashfile"
	"github.com/simiooo/pinelink/cli/internal/secure"
)

type downloadResult struct {
	EntryID       string `json:"entry_id"`
	PullID        string `json:"pull_id"`
	TaskID        string `json:"task_id"`
	Bytes         int64  `json:"bytes"`
	FirstDelivery bool   `json:"first_delivery"`
	Output        string `json:"output"`
}

func newDownloadCommand(opts *options) *cobra.Command {
	var resume bool
	var transferID, pullID string
	var rateLimit int64
	command := &cobra.Command{
		Use:  "download ENTRY_ID OUTPUT",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			var pull client.Pull
			if pullID != "" {
				pull, err = api.GetPull(cmd.Context(), pullID, opts.owner, opts.device)
				resume = true
			} else {
				pull, err = api.CreatePull(cmd.Context(), args[0], opts.owner, opts.device, transferID)
			}
			if err != nil {
				return err
			}
			if pull.EntryID != args[0] {
				return fmt.Errorf("pull does not belong to entry %s", args[0])
			}
			if err := downloadToFile(cmd, opts, api, pull, args[1], resume, rateLimit); err != nil {
				markTaskFailed(cmd, opts, api, pull.TaskID, err.Error())
				return err
			}
			completed, err := api.CompletePull(cmd.Context(), pull.ID, opts.device, pull.ContentRoot)
			if err != nil {
				return err
			}
			result := downloadResult{EntryID: pull.EntryID, PullID: pull.ID, TaskID: pull.TaskID, Bytes: pull.SizeBytes, FirstDelivery: completed.FirstDelivery, Output: args[1]}
			if opts.jsonOutput {
				return opts.output(result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "entry_id=%s\npull_id=%s\ntask_id=%s\nbytes=%d\nfirst_delivery=%t\noutput=%s\n", result.EntryID, result.PullID, result.TaskID, result.Bytes, result.FirstDelivery, result.Output)
			return nil
		},
	}
	command.Flags().BoolVar(&resume, "resume", false, "resume OUTPUT.part")
	command.Flags().StringVar(&transferID, "transfer", "", "accepted transfer request ID")
	command.Flags().StringVar(&pullID, "pull-id", "", "resume an existing pull and task")
	command.Flags().Int64Var(&rateLimit, "rate-limit", 0, "maximum download bytes per second, 0 for unlimited")
	return command
}

func downloadToFile(cmd *cobra.Command, opts *options, api *client.Client, pull client.Pull, output string, resume bool, rateLimit int64) error {
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("output already exists: %s", output)
	} else if !os.IsNotExist(err) {
		return err
	}
	part := output + ".part"
	var offset int64
	if resume {
		info, err := os.Stat(part)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			offset = info.Size()
		}
	}
	if offset > pull.SizeBytes {
		return fmt.Errorf("partial file is larger than remote content")
	}
	if err := checkDownloadTask(cmd, opts, api, pull); err != nil {
		return err
	}
	flags := os.O_CREATE | os.O_WRONLY
	if resume {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(part, flags, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	digest := sha256.New()
	if offset > 0 {
		existing, err := os.Open(part)
		if err != nil {
			return err
		}
		_, err = io.Copy(digest, existing)
		_ = existing.Close()
		if err != nil {
			return err
		}
	}
	written := offset
	for written < pull.SizeBytes {
		if err := checkDownloadTask(cmd, opts, api, pull); err != nil {
			return err
		}
		end := min(written+256<<10-1, pull.SizeBytes-1)
		response, err := api.ContentRange(cmd.Context(), pull.ID, written, end)
		if err != nil {
			if stateErr := checkDownloadTask(cmd, opts, api, pull); stateErr != nil {
				return stateErr
			}
			return err
		}
		expected := end - written + 1
		data, readErr := io.ReadAll(io.LimitReader(response.Body, expected+1))
		closeErr := response.Body.Close()
		// Revocation can truncate an already-open HTTP response. Prefer the
		// authoritative pause/cancel reason over a transport-level EOF.
		if readErr != nil || closeErr != nil || int64(len(data)) != expected {
			if stateErr := checkDownloadTask(cmd, opts, api, pull); stateErr != nil {
				return stateErr
			}
		}
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if int64(len(data)) != expected {
			return fmt.Errorf("range returned %d bytes, expected %d", len(data), expected)
		}
		if _, err := io.MultiWriter(file, digest).Write(data); err != nil {
			return err
		}
		written += expected
		if _, err := api.TaskProgress(cmd.Context(), pull.TaskID, opts.owner, opts.device, written); err != nil {
			return err
		}
		if rateLimit > 0 {
			delay := time.Duration(expected * int64(time.Second) / rateLimit)
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-time.After(delay):
			}
		}
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != pull.SizeBytes {
		return fmt.Errorf("downloaded %d bytes, expected %d", info.Size(), pull.SizeBytes)
	}
	if hex.EncodeToString(digest.Sum(nil)) != pull.ContentRoot {
		return fmt.Errorf("content checksum mismatch for pull %s: got %s expected %s", pull.ID, hex.EncodeToString(digest.Sum(nil)), pull.ContentRoot)
	}
	if err := file.Close(); err != nil {
		return err
	}
	return finalizeDownload(cmd, opts, api, pull, part, output)
}

func finalizeDownload(cmd *cobra.Command, opts *options, api *client.Client, pull client.Pull, part, output string) error {
	envelope, err := api.GetManifest(cmd.Context(), pull.EntryID, opts.owner, opts.device, pull.TransferID)
	if err != nil {
		return err
	}
	identity, err := secure.LoadIdentity(opts.owner, opts.device)
	if err != nil {
		return err
	}
	manifest, err := secure.OpenManifest(envelope.EncryptedManifest, identity)
	if err != nil {
		return err
	}
	plaintextPath := output + ".plain"
	_ = os.Remove(plaintextPath)
	if err := secure.DecryptManifestFile(part, plaintextPath, manifest); err != nil {
		_ = os.Remove(plaintextPath)
		return err
	}
	plaintext, err := hashfile.Compute(plaintextPath, 4<<20)
	if err != nil {
		return err
	}
	if plaintext.ContentRoot != manifest.PlaintextRoot || plaintext.Size != manifest.PlaintextSize {
		_ = os.Remove(plaintextPath)
		return fmt.Errorf("plaintext checksum mismatch")
	}
	if err := os.Rename(plaintextPath, output); err != nil {
		return err
	}
	return os.Remove(part)
}

func checkDownloadTask(cmd *cobra.Command, opts *options, api *client.Client, pull client.Pull) error {
	if pull.TaskID != "" {
		task, err := api.GetTask(cmd.Context(), pull.TaskID, opts.owner, opts.device)
		if err != nil {
			return err
		}
		if task.State != "RUNNING" {
			return fmt.Errorf("download task is %s; resume it before continuing", task.State)
		}
	}
	if pull.TransferID != "" {
		transfer, err := api.GetTransfer(cmd.Context(), pull.TransferID, opts.owner, opts.device)
		if err != nil {
			return err
		}
		// COMPLETED only permits a restore when the server's explicit live grant
		// authorizes this pull; state alone does not grant content access.
		if transfer.State != "ACCEPTED" && transfer.State != "COMPLETED" {
			return fmt.Errorf("transfer is %s; download cannot continue", transfer.State)
		}
	}
	return nil
}
