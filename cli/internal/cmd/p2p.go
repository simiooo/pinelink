package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/simiooo/pinelink/cli/internal/client"
	"github.com/simiooo/pinelink/cli/internal/hashfile"
	"github.com/simiooo/pinelink/cli/internal/p2p"
	"github.com/simiooo/pinelink/cli/internal/secure"
)

func newP2PCommand(opts *options) *cobra.Command {
	root := &cobra.Command{Use: "p2p", Short: "transfer encrypted files over WebRTC"}
	root.AddCommand(newP2PSendCommand(opts), newP2PReceiveCommand(opts), newP2PProgressCommand(opts))
	return root
}

func newP2PProgressCommand(opts *options) *cobra.Command {
	return &cobra.Command{Use: "progress TRANSFER_ID", Short: "show delivery, cloud-copy and route observations", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		api, err := opts.client()
		if err != nil {
			return err
		}
		snapshot, err := api.GetP2PProgress(cmd.Context(), args[0], opts.owner, opts.device)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(snapshot)
	}}
}

func newP2PSendCommand(opts *options) *cobra.Command {
	var recipientOwner, recipientDevice, path string
	var timeout time.Duration
	command := &cobra.Command{
		Use:  "send FILE",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			plaintext, err := hashfile.Compute(args[0], 64<<10)
			if err != nil {
				return err
			}
			dedupKey := hashfile.DedupKey([]byte(opts.dedupSecret), plaintext.Size, plaintext.ContentRoot)
			identity, err := secure.LoadIdentity(opts.owner, opts.device)
			if err != nil {
				return err
			}
			ciphertextPath, envelopePath, err := secure.SpoolPaths(dedupKey, "p2p", opts.server, opts.owner, opts.device, opts.scope, opts.encryptionDomain)
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
			ciphertext, err := hashfile.Compute(ciphertextPath, 64<<10)
			if err != nil {
				return err
			}
			ownManifest, err := secure.OpenManifest(envelope, identity)
			if err != nil {
				return err
			}
			publicKey, err := recipientPublicKey(cmd, opts, recipientOwner, recipientDevice)
			if err != nil {
				return err
			}
			recipientEnvelope, err := secure.SealManifest(ownManifest, []string{publicKey})
			if err != nil {
				return err
			}
			api, err := opts.client()
			if err != nil {
				return err
			}
			transfer, err := api.CreateP2P(cmd.Context(), client.P2PCreateRequest{
				SenderOwnerID: opts.owner, SenderDeviceID: opts.device,
				RecipientOwnerID: recipientOwner, RecipientDeviceID: recipientDevice,
				FileName: filepath.Base(args[0]), SizeBytes: ciphertext.Size, ContentRoot: ciphertext.ContentRoot,
				EncryptionSuite: secure.Suite, EncryptedManifest: recipientEnvelope, SenderEncryptedManifest: envelope,
			})
			if err != nil {
				return err
			}
			if err := p2p.RunSender(cmd.Context(), api, transfer, ciphertextPath, path, timeout); err != nil {
				return err
			}
			_ = os.Remove(ciphertextPath)
			_ = os.Remove(envelopePath)
			snapshot, err := api.GetP2PProgress(cmd.Context(), transfer.ID, opts.owner, opts.device)
			if err != nil {
				return err
			}
			transfer, err = api.GetTransfer(cmd.Context(), transfer.ID, opts.owner, opts.device)
			if err != nil {
				return err
			}
			if opts.jsonOutput {
				return opts.output(struct {
					client.Transfer
					Stored    bool `json:"stored"`
					Delivered bool `json:"delivered"`
				}{transfer, snapshot.Completion.Stored, snapshot.Completion.Delivered})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "transfer_id=%s\nstate=%s\nstored=%t\ndelivered=%t\n", transfer.ID, transfer.State, snapshot.Completion.Stored, snapshot.Completion.Delivered)
			return nil
		},
	}
	command.Flags().StringVar(&recipientOwner, "to-owner", "", "recipient owner")
	command.Flags().StringVar(&recipientDevice, "to-device", "", "recipient device")
	command.Flags().StringVar(&path, "path", "auto", "direct, relay or auto")
	command.Flags().DurationVar(&timeout, "direct-timeout", 3*time.Second, "direct connection attempt timeout")
	_ = command.MarkFlagRequired("to-owner")
	_ = command.MarkFlagRequired("to-device")
	return command
}

func newP2PReceiveCommand(opts *options) *cobra.Command {
	var path string
	var timeout time.Duration
	command := &cobra.Command{
		Use:  "receive TRANSFER_ID OUTPUT",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			transfer, err := api.GetTransfer(cmd.Context(), args[0], opts.owner, opts.device)
			if err != nil {
				return err
			}
			if transfer.State != "ACCEPTED" {
				return fmt.Errorf("transfer must be accepted before P2P receive")
			}
			partPath := args[1] + ".p2p"
			check := func() error {
				task, err := api.GetTask(cmd.Context(), transfer.ID, opts.owner, opts.device)
				if err != nil {
					return err
				}
				if task.State != "RUNNING" {
					return fmt.Errorf("P2P task is %s", task.State)
				}
				current, err := api.GetTransfer(cmd.Context(), transfer.ID, opts.owner, opts.device)
				if err != nil {
					return err
				}
				if current.State != "ACCEPTED" {
					return fmt.Errorf("P2P transfer is %s", current.State)
				}
				return nil
			}
			var completed client.Transfer
			finalize := func() error {
				if err := p2p.VerifyPart(partPath, transfer); err != nil {
					return err
				}
				identity, err := secure.LoadIdentity(opts.owner, opts.device)
				if err != nil {
					return err
				}
				manifest, err := secure.OpenManifest(transfer.EncryptedManifest, identity)
				if err != nil {
					return err
				}
				plaintextPath := args[1] + ".plain"
				_ = os.Remove(plaintextPath)
				if err := secure.DecryptManifestFile(partPath, plaintextPath, manifest); err != nil {
					return err
				}
				plaintext, err := hashfile.Compute(plaintextPath, 4<<20)
				if err != nil {
					return err
				}
				if plaintext.ContentRoot != manifest.PlaintextRoot || plaintext.Size != manifest.PlaintextSize {
					return fmt.Errorf("P2P plaintext checksum mismatch")
				}
				if err := os.Rename(plaintextPath, args[1]); err != nil {
					return err
				}
				_ = os.Remove(partPath)
				for attempt := 0; attempt < 2; attempt++ {
					current, err := api.GetTransfer(cmd.Context(), transfer.ID, opts.owner, opts.device)
					if err != nil {
						return err
					}
					completed, err = api.CompleteP2P(cmd.Context(), transfer.ID, opts.owner, opts.device, current.Version)
					if err == nil {
						break
					}
					if attempt == 1 {
						return err
					}
				}
				return nil
			}
			if err := p2p.RunReceiver(cmd.Context(), api, transfer, partPath, path, timeout, check, finalize); err != nil {
				return err
			}
			if opts.jsonOutput {
				return opts.output(map[string]any{"transfer_id": completed.ID, "state": completed.State, "output": args[1]})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "transfer_id=%s\nstate=%s\noutput=%s\n", completed.ID, completed.State, args[1])
			return nil
		},
	}
	command.Flags().StringVar(&path, "path", "auto", "direct, relay or auto")
	command.Flags().DurationVar(&timeout, "direct-timeout", 3*time.Second, "direct connection attempt timeout")
	return command
}

func recipientPublicKey(cmd *cobra.Command, opts *options, ownerID, deviceID string) (string, error) {
	api, err := opts.client()
	if err != nil {
		return "", err
	}
	return api.RecipientPublicKey(cmd.Context(), ownerID, deviceID)
}
