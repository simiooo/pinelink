package cmd

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/simiooo/pinelink/cli/internal/client"
	"github.com/simiooo/pinelink/cli/internal/secure"
)

type options struct {
	server           string
	scope            string
	encryptionDomain string
	owner            string
	device           string
	dedupSecret      string
	jsonOutput       bool
}

var BuildVersion = "dev"

func Execute() error {
	hostname, _ := os.Hostname()
	opts := &options{}
	root := &cobra.Command{
		Use:           "pinelink",
		Short:         "Pinelink file transfer CLI",
		Version:       BuildVersion,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&opts.server, "server", env("PINELINK_SERVER", "http://127.0.0.1:8080"), "backend HTTP URL")
	root.PersistentFlags().StringVar(&opts.scope, "scope", env("PINELINK_SCOPE", "dev-scope"), "dedup scope")
	root.PersistentFlags().StringVar(&opts.encryptionDomain, "encryption-domain", env("PINELINK_ENCRYPTION_DOMAIN", "dev-encryption-domain"), "encryption domain")
	root.PersistentFlags().StringVar(&opts.owner, "owner", env("PINELINK_OWNER", "dev-owner"), "owner identity")
	root.PersistentFlags().StringVar(&opts.device, "device", env("PINELINK_DEVICE", hostname), "device identity")
	root.PersistentFlags().StringVar(&opts.dedupSecret, "dedup-secret", env("PINELINK_DEDUP_SECRET", "pinelink-dev-dedup-secret"), "domain dedup secret")
	root.PersistentFlags().BoolVar(&opts.jsonOutput, "json", false, "print JSON")
	root.AddCommand(
		newUploadCommand(opts), newDownloadCommand(opts), newSendCommand(opts),
		newTransferActionCommand(opts, "accept"), newTransferActionCommand(opts, "reject"),
		newTransferActionCommand(opts, "cancel"), newNotificationsCommand(opts),
		newTransfersCommand(opts), newTasksCommand(opts), newDeviceCommand(opts),
		newBroadcastCommand(opts), newFileActionCommand(opts, "promote"),
		newLANCommand(opts), newLocalWebCommand(),
		newFileActionCommand(opts, "delete-file"), newQuotaCommand(opts), newP2PCommand(opts),
	)
	return root.Execute()
}

func (o *options) client() (*client.Client, error) {
	identity, err := secure.LoadIdentity(o.owner, o.device)
	if err != nil {
		return nil, err
	}
	return client.New(o.server, identity, o.owner, o.device)
}

func (o *options) output(value any) error {
	if o.jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	return nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
