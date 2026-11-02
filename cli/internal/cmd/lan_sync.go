package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/simiooo/pinelink/cli/internal/lan"
)

func newLANSync(opts *options) *cobra.Command {
	root := &cobra.Command{Use: "sync", Short: "replicate encrypted history and recover after disconnection"}
	var server, pin, owner, device string
	var configureOnly bool
	bind := &cobra.Command{Use: "bind", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, e := localIdentity(opts)
		if e != nil {
			return e
		}
		if owner == "" {
			owner = opts.owner
		}
		if device == "" {
			device = opts.device
		}
		if e = lan.ConfigureCloudSync(identity, directory, server, pin, owner, device); e != nil {
			return e
		}
		if !configureOnly {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			e = lan.SyncOnce(ctx, identity, directory)
			cancel()
			if e != nil {
				cmd.PrintErrln("History sync configured; the worker will retry. Inspect lan sync status.")
			}
		}
		status, e := lan.SyncStatus(identity, directory)
		if e != nil {
			return e
		}
		return printLocal(opts, status)
	}}
	bind.Flags().StringVar(&server, "server", "", "history server URL")
	bind.Flags().StringVar(&pin, "server-key", "", "pinned Ed25519 history server public key")
	bind.Flags().StringVar(&owner, "cloud-owner", "", "cloud identity without changing this local key profile")
	bind.Flags().StringVar(&device, "cloud-device", "", "cloud device identifier")
	bind.Flags().BoolVar(&configureOnly, "configure-only", false, "persist configuration without a network request")
	_ = bind.MarkFlagRequired("server")
	_ = bind.MarkFlagRequired("server-key")
	root.AddCommand(bind)
	root.AddCommand(&cobra.Command{Use: "once", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, e := localIdentity(opts)
		if e != nil {
			return e
		}
		return lan.SyncOnce(cmd.Context(), identity, directory)
	}})
	root.AddCommand(&cobra.Command{Use: "run", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, e := localIdentity(opts)
		if e != nil {
			return e
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return lan.RunSync(ctx, identity, directory)
	}})
	root.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, e := localIdentity(opts)
		if e != nil {
			return e
		}
		status, e := lan.SyncStatus(identity, directory)
		if e != nil {
			return e
		}
		return printLocal(opts, status)
	}})
	root.AddCommand(&cobra.Command{Use: "history", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, e := localIdentity(opts)
		if e != nil {
			return e
		}
		history, e := lan.ReplicaHistory(identity, directory)
		if e != nil {
			return e
		}
		return printLocal(opts, history)
	}})
	root.AddCommand(&cobra.Command{Use: "disable", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, e := localIdentity(opts)
		if e != nil {
			return e
		}
		return lan.DisableCloudSync(identity, directory)
	}})
	return root
}
