package main

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	"github.com/pressly/goose/v3"

	"github.com/simiooo/pinelink/backend/internal/blob"
	"github.com/simiooo/pinelink/backend/internal/config"
	"github.com/simiooo/pinelink/backend/internal/events"
	"github.com/simiooo/pinelink/backend/internal/httpapi"
	"github.com/simiooo/pinelink/backend/internal/lifecycle"
	"github.com/simiooo/pinelink/backend/internal/service"
)

func main() {
	logger := log.With(log.NewStdLogger(os.Stdout), "service", "pinelinkd")
	if err := run(logger); err != nil {
		log.NewHelper(logger).Error(err)
		os.Exit(1)
	}
}

func run(logger log.Logger) error {
	cfg := config.FromEnv()
	startup, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := migrate(cfg.DatabaseURL, cfg.MigrationsDir); err != nil {
		return err
	}
	pool, err := pgxpool.New(startup, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(startup); err != nil {
		return err
	}

	objects, err := blob.NewS3(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	if err != nil {
		return err
	}
	// A healthy filer can precede its S3 listener during cold startup. Stay alive
	// and retry within the startup deadline instead of entering a restart loop.
	for {
		if err := objects.EnsureBucket(startup); err != nil {
			select {
			case <-startup.Done():
				return fmt.Errorf("object storage startup: %w", err)
			case <-time.After(time.Second):
				continue
			}
		}
		break
	}
	connection, err := nats.Connect(cfg.NATSURL,
		nats.Name("pinelinkd"),
		nats.Timeout(3*time.Second),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(30),
		nats.ReconnectWait(time.Second),
	)
	if err != nil {
		return err
	}
	defer connection.Close()

	svc := service.New(pool, objects)
	if cfg.HistorySigningSeed != "" {
		seed, e := base64.StdEncoding.DecodeString(cfg.HistorySigningSeed)
		if e != nil || len(seed) != ed25519.SeedSize {
			return fmt.Errorf("invalid history signing seed")
		}
		if e = svc.SetReplicationKey(ed25519.NewKeyFromSeed(seed)); e != nil {
			return e
		}
	}
	httpServer := httpapi.NewServer(cfg.HTTPAddr, svc, logger, cfg.DeviceAuthRequired, cfg.CORSOrigins)
	outbox := events.NewWorker(pool, connection, logger)
	maintenance := service.NewMaintenance(pool, objects, time.Second, nil)
	relay := service.NewRelayManager(svc, nil)
	history := service.NewHistoryReplicator(svc, time.Second, nil)
	app := kratos.New(
		kratos.Name("pinelinkd"),
		kratos.Server(httpServer,
			lifecycle.NewWorker(outbox.Start),
			lifecycle.NewWorker(maintenance.Start),
			lifecycle.NewWorker(relay.Start),
			lifecycle.NewWorker(history.Start)),
		kratos.StopTimeout(15*time.Second),
		kratos.Signal(os.Interrupt, syscall.SIGTERM),
		kratos.Logger(logger),
	)
	return app.Run()
}

func migrate(databaseURL, directory string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, directory)
}
