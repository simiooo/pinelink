package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/simiooo/pinelink/cli/internal/lan"
	"github.com/simiooo/pinelink/cli/internal/secure"
)

func localIdentity(opts *options) (*secure.Identity, string, error) {
	identity, err := secure.LoadIdentity(opts.owner, opts.device)
	if err != nil {
		return nil, "", err
	}
	base, err := secure.ConfigDirectory()
	if err != nil {
		return nil, "", err
	}
	return identity, filepath.Join(base, "local", identity.LocalID()), nil
}

func newLANCommand(opts *options) *cobra.Command {
	root := &cobra.Command{Use: "lan", Short: "use encrypted local transfers without an internet service"}
	root.AddCommand(newLANServe(opts), newLANPair(opts), newLANSend(opts, false), newLANSend(opts, true), newLANDiscover(opts), newLANLocalControl(opts))
	root.AddCommand(newLANSync(opts))
	root.AddCommand(&cobra.Command{Use: "history", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, err := localIdentity(opts)
		if err != nil {
			return err
		}
		history, err := lan.History(identity, directory)
		if err != nil {
			return err
		}
		return printLocal(opts, history)
	}})
	root.AddCommand(&cobra.Command{Use: "storage", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, err := localIdentity(opts)
		if err != nil {
			return err
		}
		info, err := lan.Storage(identity, directory)
		if err != nil {
			return err
		}
		return printLocal(opts, info)
	}})
	var after int64
	var stream string
	changes := &cobra.Command{Use: "changes", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, err := localIdentity(opts)
		if err != nil {
			return err
		}
		page, err := lan.Changes(identity, directory, after, 200, stream)
		if err != nil {
			return err
		}
		return printLocal(opts, page)
	}}
	changes.Flags().Int64Var(&after, "after", 0, "last committed event sequence")
	changes.Flags().StringVar(&stream, "stream", "", "expected database stream id; detects a profile reset")
	root.AddCommand(changes)
	for _, action := range []string{"accept", "reject", "cancel", "pause", "resume"} {
		action := action
		root.AddCommand(&cobra.Command{Use: action + " TRANSFER_ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			var client *lan.Client
			var err error
			if action == "accept" || action == "reject" {
				client, err = ownLocalClient(opts)
			} else {
				client, err = localTransferClient(opts, args[0])
			}
			if err != nil {
				return err
			}
			defer client.Close()
			current, err := client.Transfer(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			current, err = client.Action(cmd.Context(), args[0], action, current.Version)
			if err != nil {
				return err
			}
			return outputLocal(opts, current)
		}})
	}
	return root
}

func newLANServe(opts *options) *cobra.Command {
	var listen, address, output, name, ready string
	var autoPair, autoAccept, noDiscovery, noSync bool
	command := &cobra.Command{Use: "serve", Short: "receive files on this device's local network", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, err := localIdentity(opts)
		if err != nil {
			return err
		}
		listener, err := net.Listen("tcp", listen)
		if err != nil {
			return err
		}
		defer listener.Close()
		if address == "" {
			address, err = lan.DefaultAddress(listener)
			if err != nil {
				return err
			}
		}
		server, err := lan.NewServer(identity, lan.Config{Directory: directory, ReceiveDirectory: output, Name: name, Address: address, AutoPair: autoPair, AutoAccept: autoAccept, Sync: !noSync})
		if err != nil {
			return err
		}
		defer server.Close()
		if err = server.Prepare(listener); err != nil {
			return err
		}
		invitation, err := server.Invite()
		if err != nil {
			return err
		}
		code, err := invitation.Code()
		if err != nil {
			return err
		}
		receiver := struct {
			Peer   lan.Peer `json:"peer"`
			Code   string   `json:"connection_code"`
			Output string   `json:"receive_directory"`
		}{server.Peer(), code, output}
		body, err := json.Marshal(receiver)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(directory, 0o700); err != nil {
			return err
		}
		// Prepare publishes the public endpoint atomically. The invitation is
		// written only to the explicit private ready file / terminal output.
		if ready != "" {
			if err = os.WriteFile(ready, body, 0o600); err != nil {
				return err
			}
		}
		if opts.jsonOutput {
			if err = opts.output(receiver); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "device=%s\nlocal_endpoint=%s\nconnection_code=%s\n", server.Peer().ID, address, code)
		}
		if !noDiscovery {
			stop, discoveryErr := lan.Advertise(server.Peer())
			if discoveryErr == nil {
				defer stop()
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), "Local discovery is unavailable; use the connection code.")
			}
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return server.Serve(ctx, listener)
	}}
	command.Flags().StringVar(&listen, "listen", ":19091", "local TCP listen address")
	command.Flags().StringVar(&address, "advertise", "", "reachable https://IP:port for the connection code")
	command.Flags().StringVar(&output, "receive-dir", ".", "directory explicitly selected for received files")
	command.Flags().StringVar(&name, "name", "", "device name shown during pairing")
	// Cobra binds options after construction; resolve an empty default at RunE.
	command.Flags().StringVar(&ready, "ready-file", "", "write private local connection information to this file")
	command.Flags().BoolVar(&autoPair, "auto-pair", false, "explicitly approve holders of this receiver's one-time invitation")
	command.Flags().BoolVar(&autoAccept, "auto-accept", false, "explicitly accept file offers from paired devices")
	command.Flags().BoolVar(&noDiscovery, "no-discovery", false, "use manual connection codes without multicast")
	command.Flags().BoolVar(&noSync, "no-sync", false, "disable this process's automatic history synchronization")
	original := command.RunE
	command.RunE = func(cmd *cobra.Command, args []string) error {
		if name == "" {
			name = opts.device
		}
		return original(cmd, args)
	}
	return command
}

func newLANPair(opts *options) *cobra.Command {
	var name, address string
	command := &cobra.Command{Use: "pair CONNECTION_CODE", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		invitation, err := lan.ParseInvitation(args[0])
		if err != nil {
			return err
		}
		identity, directory, err := localIdentity(opts)
		if err != nil {
			return err
		}
		peer := invitation.Peer
		if address != "" {
			peer.Address = address
		}
		client, err := lan.NewClient(identity, peer, directory)
		if err != nil {
			return err
		}
		defer client.Close()
		if name == "" {
			name = opts.device
		}
		pair, err := client.Pair(cmd.Context(), invitation, name)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "pair_request=%s\ncompare_code=%s\n", pair.ID, pair.Code)
		expectedCode := pair.Code
		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
		defer cancel()
		for pair.State == "WAITING" {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(150 * time.Millisecond):
			}
			pair, err = client.PairStatus(ctx, pair.ID)
			if err != nil {
				return err
			}
			if pair.Code != expectedCode {
				return fmt.Errorf("local pairing comparison changed")
			}
		}
		if pair.State != "APPROVED" {
			return fmt.Errorf("local pairing is %s", pair.State)
		}
		if err = client.Remember(); err != nil {
			return err
		}
		if opts.jsonOutput {
			return opts.output(client.Peer())
		}
		fmt.Fprintf(cmd.OutOrStdout(), "peer_id=%s\nname=%s\n", peer.ID, peer.Name)
		return nil
	}}
	command.Flags().StringVar(&name, "name", "", "this device's name")
	command.Flags().StringVar(&address, "address", "", "use another reachable IP endpoint with the same pinned device key")
	return command
}

func newLANSend(opts *options, resume bool) *cobra.Command {
	var peerID, address string
	var offerOnly bool
	use := "send FILE"
	if resume {
		use = "resume-send TRANSFER_ID"
	}
	command := &cobra.Command{Use: use, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		identity, directory, err := localIdentity(opts)
		if err != nil {
			return err
		}
		var peer lan.Peer
		if resume {
			peer, err = lan.SavedPeer(directory, args[0])
		} else {
			peer, err = lan.ReadContact(directory, peerID)
		}
		if err != nil {
			return err
		}
		if address != "" {
			peer.Address = address
		}
		client, err := lan.NewClient(identity, peer, directory)
		if err != nil {
			return err
		}
		defer client.Close()
		id := args[0]
		if !resume {
			prepared, err := client.PrepareFile(cmd.Context(), args[0])
			if prepared.ID != "" && !offerOnly {
				if autoErr := client.AutomaticSend(prepared.ID, true); autoErr != nil {
					return autoErr
				}
			}
			if prepared.ID != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "transfer_id=%s\n", prepared.ID)
			}
			if err != nil {
				return err
			}
			id = prepared.ID
			if offerOnly {
				return outputLocal(opts, prepared)
			}
		}
		if resume {
			if err = client.AutomaticSend(id, true); err != nil {
				return err
			}
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		result, err := client.Send(ctx, id, nil)
		if err != nil {
			return fmt.Errorf("transfer_id=%s: %w", id, err)
		}
		return outputLocal(opts, result)
	}}
	if !resume {
		command.Flags().StringVar(&peerID, "peer", "", "verified peer id from lan pair")
		_ = command.MarkFlagRequired("peer")
		command.Flags().BoolVar(&offerOnly, "offer-only", false, "prepare a file offer; transfer later with resume-send")
	}
	command.Flags().StringVar(&address, "address", "", "use another reachable IP endpoint with the same pinned device key")
	return command
}

func localTransferClient(opts *options, id string) (*lan.Client, error) {
	identity, directory, err := localIdentity(opts)
	if err != nil {
		return nil, err
	}
	peer, err := lan.SavedPeer(directory, id)
	if err == nil {
		return lan.NewClient(identity, peer, directory)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return ownLocalClient(opts)
}

func newLANDiscover(opts *options) *cobra.Command {
	var timeout time.Duration
	command := &cobra.Command{Use: "discover", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		peers, err := lan.Discover(ctx)
		if err != nil {
			return err
		}
		if opts.jsonOutput {
			return opts.output(peers)
		}
		for _, p := range peers {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %v\n", p.ID, p.Name, p.Addresses)
		}
		return nil
	}}
	command.Flags().DurationVar(&timeout, "timeout", 3*time.Second, "local multicast browse time")
	return command
}

func ownLocalClient(opts *options) (*lan.Client, error) {
	identity, directory, err := localIdentity(opts)
	if err != nil {
		return nil, err
	}
	peer, err := lan.LocalPeer(identity, directory)
	if err != nil {
		return nil, err
	}
	return lan.NewClient(identity, peer, directory)
}

func newLANLocalControl(opts *options) *cobra.Command {
	root := &cobra.Command{Use: "local", Short: "approve pairing and inspect this receiver"}
	root.AddCommand(&cobra.Command{Use: "requests", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := ownLocalClient(opts)
		if err != nil {
			return err
		}
		defer c.Close()
		p, err := c.PairRequests(cmd.Context())
		if err != nil {
			return err
		}
		return printLocal(opts, p)
	}})
	for _, action := range []string{"approve", "reject"} {
		action := action
		root.AddCommand(&cobra.Command{Use: action + " PAIR_REQUEST_ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			c, err := ownLocalClient(opts)
			if err != nil {
				return err
			}
			defer c.Close()
			p, err := c.PairAction(cmd.Context(), args[0], action)
			if err != nil {
				return err
			}
			return printLocal(opts, p)
		}})
	}
	root.AddCommand(&cobra.Command{Use: "transfers", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := ownLocalClient(opts)
		if err != nil {
			return err
		}
		defer c.Close()
		t, err := c.Transfers(cmd.Context())
		if err != nil {
			return err
		}
		return printLocal(opts, t)
	}})
	root.AddCommand(&cobra.Command{Use: "invite", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := ownLocalClient(opts)
		if err != nil {
			return err
		}
		defer c.Close()
		i, err := c.Invite(cmd.Context())
		if err != nil {
			return err
		}
		code, err := i.Code()
		if err != nil {
			return err
		}
		return printLocal(opts, map[string]string{"connection_code": code})
	}})
	root.AddCommand(&cobra.Command{Use: "forget PEER_ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := ownLocalClient(opts)
		if err != nil {
			return err
		}
		defer c.Close()
		return c.Forget(cmd.Context(), args[0])
	}})
	return root
}
func printLocal(opts *options, value any) error {
	if opts.jsonOutput {
		return opts.output(value)
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		fmt.Println(string(body))
	}
	return err
}
func outputLocal(opts *options, t lan.Transfer) error {
	if opts.jsonOutput {
		return opts.output(t)
	}
	fmt.Printf("transfer_id=%s\nstate=%s\nacknowledged_bytes=%d\nroute=%s\n", t.ID, t.State, t.Acknowledged, t.Route)
	return nil
}
