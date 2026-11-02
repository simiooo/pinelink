package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	MigrationsDir      string
	NATSURL            string
	S3Endpoint         string
	S3AccessKey        string
	S3SecretKey        string
	S3Bucket           string
	S3UseSSL           bool
	DeviceAuthRequired bool
	CORSOrigins        []string
	HistorySigningSeed string
}

func FromEnv() Config {
	useSSL, _ := strconv.ParseBool(getenv("PINELINK_S3_USE_SSL", "false"))
	deviceAuth, _ := strconv.ParseBool(getenv("PINELINK_DEVICE_AUTH_REQUIRED", "false"))
	origins := strings.Split(getenv("PINELINK_CORS_ORIGINS", ""), ",")
	for index := range origins {
		origins[index] = strings.TrimSpace(origins[index])
	}
	return Config{
		HTTPAddr:           getenv("PINELINK_HTTP_ADDR", ":8080"),
		DatabaseURL:        getenv("PINELINK_DATABASE_URL", "postgres://pinelink:pinelink_dev@127.0.0.1:55432/pinelink?sslmode=disable"),
		MigrationsDir:      getenv("PINELINK_MIGRATIONS_DIR", "../infra/migrations"),
		NATSURL:            getenv("PINELINK_NATS_URL", "nats://127.0.0.1:54222"),
		S3Endpoint:         getenv("PINELINK_S3_ENDPOINT", "127.0.0.1:59000"),
		S3AccessKey:        getenv("PINELINK_S3_ACCESS_KEY", "pinelink"),
		S3SecretKey:        getenv("PINELINK_S3_SECRET_KEY", "pinelink_dev_secret"),
		S3Bucket:           getenv("PINELINK_S3_BUCKET", "pinelink-dev"),
		S3UseSSL:           useSSL,
		DeviceAuthRequired: deviceAuth,
		CORSOrigins:        origins,
		HistorySigningSeed: getenv("PINELINK_HISTORY_SIGNING_SEED", ""),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
