package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/simiooo/pinelink/cli/internal/client"
	"github.com/simiooo/pinelink/cli/internal/secure"
)

func newSendCommand(opts *options) *cobra.Command {
	var recipientOwner, recipientDevice, mode string
	command := &cobra.Command{
		Use:  "send ENTRY_ID",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			if err := ensureEnvelope(cmd, opts, args[0], recipientOwner, recipientDevice); err != nil {
				return err
			}
			transfer, err := api.CreateTransfer(cmd.Context(), client.TransferCreateRequest{
				EntryID: args[0], DeliveryMode: mode, SenderOwnerID: opts.owner, SenderDeviceID: opts.device,
				RecipientOwnerID: recipientOwner, RecipientDeviceID: recipientDevice,
			})
			if err != nil {
				return err
			}
			return outputTransfer(cmd, opts, transfer)
		},
	}
	command.Flags().StringVar(&recipientOwner, "to-owner", "", "recipient owner")
	command.Flags().StringVar(&recipientDevice, "to-device", "", "recipient device")
	command.Flags().StringVar(&mode, "mode", "ASYNC", "ONLINE, ASYNC or BROADCAST")
	_ = command.MarkFlagRequired("to-owner")
	_ = command.MarkFlagRequired("to-device")
	return command
}

func newTransferActionCommand(opts *options, action string) *cobra.Command {
	return &cobra.Command{
		Use:  action + " TRANSFER_ID",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			transfer, err := api.GetTransfer(cmd.Context(), args[0], opts.owner, opts.device)
			if err != nil {
				return err
			}
			transfer, err = api.TransferAction(cmd.Context(), args[0], action, opts.owner, opts.device, transfer.Version, "")
			if err != nil {
				return err
			}
			return outputTransfer(cmd, opts, transfer)
		},
	}
}

func newNotificationsCommand(opts *options) *cobra.Command {
	var state string
	var watch, once bool
	command := &cobra.Command{
		Use:  "notifications",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			if !watch {
				list, err := api.ListTransfers(cmd.Context(), "/v1/notifications", opts.owner, opts.device, "recipient", state, 100)
				if err != nil {
					return err
				}
				if opts.jsonOutput {
					return opts.output(list)
				}
				for _, transfer := range list.Transfers {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", transfer.ID, transfer.State, transfer.DeliveryMode)
				}
				return nil
			}
			// Reconcile the authoritative pending queue on startup and every poll. An event
			// cursor seeded after a request arrived must not hide that request forever.
			seen := map[string]bool{}
			for {
				list, err := api.ListTransfers(cmd.Context(), "/v1/notifications", opts.owner, opts.device, "recipient", state, 100)
				if err != nil {
					return err
				}
				nextSeen := map[string]bool{}
				for _, transfer := range list.Transfers {
					nextSeen[transfer.ID] = true
					if seen[transfer.ID] {
						continue
					}
					if transfer.RecipientOwnerID != opts.owner || transfer.RecipientDeviceID != opts.device {
						continue
					}
					if err := outputTransfer(cmd, opts, transfer); err != nil {
						return err
					}
					if once {
						return nil
					}
				}
				seen = nextSeen
				select {
				case <-cmd.Context().Done():
					return cmd.Context().Err()
				case <-time.After(500 * time.Millisecond):
				}
			}
		},
	}
	command.Flags().StringVar(&state, "state", "PENDING", "notification state")
	command.Flags().BoolVar(&watch, "watch", false, "watch for new transfer requests")
	command.Flags().BoolVar(&once, "once", false, "exit after the first matching notification")
	return command
}

func newTransfersCommand(opts *options) *cobra.Command {
	var role, state string
	var limit int
	command := &cobra.Command{
		Use:  "transfers",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			list, err := api.ListTransfers(cmd.Context(), "/v1/transfers", opts.owner, opts.device, role, state, limit)
			if err != nil {
				return err
			}
			if opts.jsonOutput {
				return opts.output(list)
			}
			for _, transfer := range list.Transfers {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s v%d\n", transfer.ID, transfer.DeliveryMode, transfer.State, transfer.Version)
			}
			return nil
		},
	}
	command.Flags().StringVar(&role, "role", "", "sender or recipient")
	command.Flags().StringVar(&state, "state", "", "filter state")
	command.Flags().IntVar(&limit, "limit", 50, "maximum entries")
	return command
}

func ensureEnvelope(cmd *cobra.Command, opts *options, entryID, targetOwnerID, targetDeviceID string) error {
	api, err := opts.client()
	if err != nil {
		return err
	}
	devices, err := api.ListDevices(cmd.Context(), targetOwnerID)
	if err != nil {
		return err
	}
	var publicKey string
	for _, device := range devices.Devices {
		if device.DeviceID == targetDeviceID && device.Status == "ACTIVE" && device.PublicKey != nil {
			publicKey = *device.PublicKey
		}
	}
	if publicKey == "" {
		return fmt.Errorf("recipient device %s/%s is not registered with an encryption key", targetOwnerID, targetDeviceID)
	}
	identity, err := secure.LoadIdentity(opts.owner, opts.device)
	if err != nil {
		return err
	}
	own, err := api.GetManifest(cmd.Context(), entryID, opts.owner, opts.device, "")
	if err != nil {
		return err
	}
	manifest, err := secure.OpenManifest(own.EncryptedManifest, identity)
	if err != nil {
		return err
	}
	envelope, err := secure.SealManifest(manifest, []string{publicKey})
	if err != nil {
		return err
	}
	return api.PutManifestEnvelope(cmd.Context(), entryID, opts.owner, opts.device, targetOwnerID, targetDeviceID, envelope)
}

func outputTransfer(cmd *cobra.Command, opts *options, transfer client.Transfer) error {
	if opts.jsonOutput {
		return opts.output(transfer)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "transfer_id=%s\nstate=%s\nversion=%d\n", transfer.ID, transfer.State, transfer.Version)
	return nil
}
