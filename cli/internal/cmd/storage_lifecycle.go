package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

func newDeviceCommand(opts *options) *cobra.Command {
	root := &cobra.Command{Use: "device", Short: "manage recipient devices"}
	for _, action := range []string{"register", "revoke"} {
		action := action
		root.AddCommand(&cobra.Command{
			Use:  action,
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				api, err := opts.client()
				if err != nil {
					return err
				}
				identity, err := secure.LoadIdentity(opts.owner, opts.device)
				if err != nil {
					return err
				}
				device, err := api.DeviceAction(cmd.Context(), action, opts.owner, opts.device, identity.PublicKey(), identity.SigningPublicKey())
				if err != nil {
					return err
				}
				if opts.jsonOutput {
					return opts.output(device)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", device.DeviceID, device.Status)
				return nil
			},
		})
	}
	root.AddCommand(&cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			devices, err := api.ListDevices(cmd.Context(), opts.owner)
			if err != nil {
				return err
			}
			if opts.jsonOutput {
				return opts.output(devices)
			}
			for _, device := range devices.Devices {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", device.DeviceID, device.Status)
			}
			return nil
		},
	})
	return root
}

func newBroadcastCommand(opts *options) *cobra.Command {
	var recipientOwner string
	command := &cobra.Command{
		Use:  "broadcast ENTRY_ID",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			devices, err := api.ListDevices(cmd.Context(), recipientOwner)
			if err != nil {
				return err
			}
			for _, device := range devices.Devices {
				if device.Status == "ACTIVE" {
					if err := ensureEnvelope(cmd, opts, args[0], recipientOwner, device.DeviceID); err != nil {
						return err
					}
				}
			}
			transfers, err := api.Broadcast(cmd.Context(), args[0], opts.owner, opts.device, recipientOwner)
			if err != nil {
				return err
			}
			if opts.jsonOutput {
				return opts.output(transfers)
			}
			for _, transfer := range transfers.Transfers {
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", transfer.ID, transfer.RecipientDeviceID, transfer.State)
			}
			return nil
		},
	}
	command.Flags().StringVar(&recipientOwner, "to-owner", "", "recipient owner whose devices are notified")
	_ = command.MarkFlagRequired("to-owner")
	return command
}

func newFileActionCommand(opts *options, action string) *cobra.Command {
	var transferID string
	command := &cobra.Command{
		Use:  action + " ENTRY_ID",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			file, err := api.GetFile(cmd.Context(), args[0], opts.owner, opts.device, transferID)
			if err != nil {
				return err
			}
			apiAction := action
			if apiAction == "delete-file" {
				apiAction = "delete"
			}
			file, err = api.FileAction(cmd.Context(), args[0], apiAction, opts.owner, opts.device, file.Version, transferID)
			if err != nil {
				return err
			}
			if opts.jsonOutput {
				return opts.output(file)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s v%d\n", file.ID, file.RetentionClass, file.State, file.Version)
			return nil
		},
	}
	if action == "promote" {
		command.Flags().StringVar(&transferID, "transfer", "", "accepted or restorable transfer grant")
	}
	return command
}

func newQuotaCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:  "quota",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := opts.client()
			if err != nil {
				return err
			}
			quota, err := api.GetQuota(cmd.Context(), opts.scope)
			if err != nil {
				return err
			}
			if opts.jsonOutput {
				return opts.output(quota)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "used=%d\nquota=%d\nversion=%d\n", quota.UsedBytes, quota.QuotaBytes, quota.Version)
			return nil
		},
	}
}
