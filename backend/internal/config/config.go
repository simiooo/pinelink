package config

import (
	"github.com/simiooo/pinelink/backend/internal/accounts"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Accounts           accounts.Config
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
	RelayTransport     string
	S3Region           string
	NATSCAFile         string
	HTTPTLSCertFile    string
	HTTPTLSKeyFile     string
}

func FromEnv() Config {
	useSSL, _ := strconv.ParseBool(getenv("PINELINK_S3_USE_SSL", "false"))
	deviceAuth, _ := strconv.ParseBool(getenv("PINELINK_DEVICE_AUTH_REQUIRED", "false"))
	origins := strings.Split(getenv("PINELINK_CORS_ORIGINS", ""), ",")
	for index := range origins {
		origins[index] = strings.TrimSpace(origins[index])
	}
	return Config{
		Accounts:           accountConfig(),
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
		RelayTransport:     getenv("PINELINK_RELAY_TRANSPORT", "webrtc"),
		S3Region:           getenv("PINELINK_S3_REGION", ""),
		NATSCAFile:         getenv("PINELINK_NATS_CA_FILE", ""),
		HTTPTLSCertFile:    getenv("PINELINK_HTTP_TLS_CERT_FILE", ""),
		HTTPTLSKeyFile:     getenv("PINELINK_HTTP_TLS_KEY_FILE", ""),
	}
}

func accountConfig() accounts.Config {
	required, _ := strconv.ParseBool(getenv("PINELINK_ACCOUNTS_REQUIRED", "false"))
	breach, _ := strconv.ParseBool(getenv("PINELINK_PASSWORD_BREACH_CHECK", "false"))
	quota, _ := strconv.ParseInt(getenv("PINELINK_ACCOUNT_QUOTA_BYTES", "1073741824"), 10, 64)
	var proxies []netip.Prefix
	for _, v := range strings.Split(getenv("PINELINK_AUTH_TRUSTED_PROXIES", ""), ",") {
		if p, err := netip.ParsePrefix(strings.TrimSpace(v)); err == nil {
			proxies = append(proxies, p)
		}
	}
	return accounts.Config{TrustedProxies: proxies, Required: required, Origin: getenv("PINELINK_ACCOUNT_ORIGIN", ""), EncryptionKey: getenv("PINELINK_ACCOUNT_ENCRYPTION_KEY", ""), GoogleClientID: getenv("PINELINK_GOOGLE_CLIENT_ID", ""), GoogleClientSecret: getenv("PINELINK_GOOGLE_CLIENT_SECRET", ""), GoogleRedirectURI: getenv("PINELINK_GOOGLE_REDIRECT_URI", ""), ResendKey: getenv("PINELINK_RESEND_API_KEY", ""), MailFrom: getenv("PINELINK_MAIL_FROM", ""), ReplyTo: getenv("PINELINK_MAIL_REPLY_TO", ""), QuotaBytes: quota, BreachCheck: breach}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
