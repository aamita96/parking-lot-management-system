// Package config reads service settings from the environment (NFR-9: no secrets in the repo).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env         string // local | staging | production
	HTTPAddr    string
	DatabaseURL string
	CORSOrigins []string
	PublicURL   string // frontend base URL, used for the registration QR code and email links

	JWTSecret        []byte
	CodeSecret       []byte // HMAC key for email-code hashes
	AdminAccessTTL   time.Duration
	AdminRefreshTTL  time.Duration
	OwnerTokenTTL    time.Duration
	CookieSecure     bool
	BootstrapAdmin   string
	BootstrapPass    string
	SimulatorEnabled bool
	SeedDemo         bool

	KafkaBrokers           []string
	KafkaTopic             string
	KafkaDLQTopic          string
	KafkaPartitions        int32
	KafkaReplicationFactor int16
	KafkaGroup             string
	// Login for managed Kafka (e.g. Aiven, Confluent). Leave empty for the local Docker Kafka.
	KafkaSASLMechanism string // PLAIN | SCRAM-SHA-256 | SCRAM-SHA-512
	KafkaUsername      string
	KafkaPassword      string
	KafkaTLS           bool
	KafkaCACert        string // provider CA certificate: PEM text, or a file path (Aiven needs it)
	KafkaClientCert    string // client certificate login (Aiven default): PEM text or file path
	KafkaClientKey     string // private key for KafkaClientCert: PEM text or file path

	EmailEnabled   bool
	DevExposeCodes bool // only honoured when EmailEnabled=false
	ResendAPIKey   string
	EmailFrom      string
	EmailReplyTo   string

	NotifierHTTPAddr string
	// RunNotifier also runs the email sender inside the API process, so a small deployment
	// (e.g. Render's free plan) needs one service instead of two.
	RunNotifier bool
}

func Load() (Config, error) {
	c := Config{
		Env:         get("APP_ENV", "local"),
		HTTPAddr:    httpAddr(),
		DatabaseURL: get("DATABASE_URL", "postgres://plms:plms@localhost:5432/plms?sslmode=disable"),
		CORSOrigins: list("CORS_ORIGINS", "http://localhost:5173,http://localhost:3000"),
		PublicURL:   strings.TrimRight(get("PUBLIC_URL", "http://localhost:5173"), "/"),

		JWTSecret:        []byte(get("JWT_SECRET", "")),
		CodeSecret:       []byte(get("CODE_HASH_SECRET", "")),
		AdminAccessTTL:   dur("ADMIN_ACCESS_TTL", 15*time.Minute),
		AdminRefreshTTL:  dur("ADMIN_REFRESH_TTL", 7*24*time.Hour),
		OwnerTokenTTL:    dur("OWNER_TOKEN_TTL", 30*24*time.Hour),
		CookieSecure:     boolean("COOKIE_SECURE", false),
		BootstrapAdmin:   get("ADMIN_BOOTSTRAP_EMAIL", ""),
		BootstrapPass:    get("ADMIN_BOOTSTRAP_PASSWORD", ""),
		SimulatorEnabled: boolean("SIMULATOR_ENABLED", false),
		SeedDemo:         boolean("SEED_DEMO", false),

		KafkaBrokers:           list("KAFKA_BROKERS", "localhost:9094"),
		KafkaTopic:             get("KAFKA_TOPIC", "parking.ticket-events.v1"),
		KafkaDLQTopic:          get("KAFKA_DLQ_TOPIC", "parking.ticket-events.v1.dlq"),
		KafkaPartitions:        int32(integer("KAFKA_PARTITIONS", 6)),
		KafkaReplicationFactor: int16(integer("KAFKA_REPLICATION_FACTOR", 1)),
		KafkaGroup:             get("KAFKA_GROUP", "plms-notifications"),
		KafkaSASLMechanism:     strings.ToUpper(get("KAFKA_SASL_MECHANISM", "")),
		KafkaUsername:          get("KAFKA_USERNAME", ""),
		KafkaPassword:          get("KAFKA_PASSWORD", ""),
		KafkaTLS:               boolean("KAFKA_TLS", false),
		KafkaCACert:            get("KAFKA_CA_CERT", ""),
		KafkaClientCert:        get("KAFKA_CLIENT_CERT", ""),
		KafkaClientKey:         get("KAFKA_CLIENT_KEY", ""),

		EmailEnabled:   boolean("EMAIL_ENABLED", false),
		DevExposeCodes: boolean("DEV_EXPOSE_CODES", false),
		ResendAPIKey:   get("RESEND_API_KEY", ""),
		EmailFrom:      get("EMAIL_FROM", "PLMS <onboarding@resend.dev>"),
		EmailReplyTo:   get("EMAIL_REPLY_TO", ""),

		NotifierHTTPAddr: get("NOTIFIER_HTTP_ADDR", ":8081"),
		RunNotifier:      boolean("RUN_NOTIFIER", false),
	}
	if len(c.JWTSecret) < 32 {
		return c, fmt.Errorf("JWT_SECRET must be set to at least 32 characters")
	}
	if len(c.CodeSecret) < 16 {
		return c, fmt.Errorf("CODE_HASH_SECRET must be set to at least 16 characters")
	}
	if c.EmailEnabled && c.ResendAPIKey == "" {
		return c, fmt.Errorf("RESEND_API_KEY is required when EMAIL_ENABLED=true")
	}
	if c.Env == "production" && c.SimulatorEnabled {
		return c, fmt.Errorf("SIMULATOR_ENABLED must be false in production")
	}
	return c, nil
}

// httpAddr uses HTTP_ADDR if set, otherwise the PORT that hosts like Render provide, otherwise :8080.
func httpAddr() string {
	if addr := get("HTTP_ADDR", ""); addr != "" {
		return addr
	}
	if port := get("PORT", ""); port != "" {
		return ":" + port
	}
	return ":8080"
}

// ExposeDevCodes reports whether verification codes may be returned in API responses
// (local demos without a Resend key only).
func (c Config) ExposeDevCodes() bool {
	return !c.EmailEnabled && c.DevExposeCodes && c.Env != "production"
}

func get(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func list(k, def string) []string {
	var out []string
	for _, p := range strings.Split(get(k, def), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func boolean(k string, def bool) bool {
	v, err := strconv.ParseBool(get(k, strconv.FormatBool(def)))
	if err != nil {
		return def
	}
	return v
}

func integer(k string, def int) int {
	v, err := strconv.Atoi(get(k, strconv.Itoa(def)))
	if err != nil {
		return def
	}
	return v
}

func dur(k string, def time.Duration) time.Duration {
	v, err := time.ParseDuration(get(k, def.String()))
	if err != nil {
		return def
	}
	return v
}
