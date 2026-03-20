package config

import (
	"flag"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Listen    string
	OVNNBAddr string
	OVNSBAddr string

	// OpenStack enrichment (all optional)
	OpenStackAuthURL     string
	OpenStackUsername     string
	OpenStackPassword    string
	OpenStackProjectName string
	OpenStackDomainName  string
	OpenStackRegionName  string
	EnrichmentCacheTTL   time.Duration

	// Alerting
	AlertWebhookURLs string // comma-separated webhook URLs

	// History & snapshots
	HistoryDBPath    string
	SnapshotInterval time.Duration
	EventRetention   time.Duration
	EventMaxCount    int64 // max number of events to retain (0 = unlimited)
}

func Parse(args []string) (*Config, error) {
	fs := flag.NewFlagSet("northwatch", flag.ContinueOnError)

	cfg := &Config{}
	fs.StringVar(&cfg.Listen, "listen", envOrDefault("NORTHWATCH_LISTEN", ":8080"), "HTTP listen address")
	fs.StringVar(&cfg.OVNNBAddr, "ovn-nb-addr", os.Getenv("NORTHWATCH_OVN_NB_ADDR"), "OVN Northbound DB address, comma-separated for failover (e.g. tcp:10.0.0.1:6641,tcp:10.0.0.2:6641)")
	fs.StringVar(&cfg.OVNSBAddr, "ovn-sb-addr", os.Getenv("NORTHWATCH_OVN_SB_ADDR"), "OVN Southbound DB address, comma-separated for failover (e.g. tcp:10.0.0.1:6642,tcp:10.0.0.2:6642)")

	// OpenStack enrichment flags
	fs.StringVar(&cfg.OpenStackAuthURL, "os-auth-url", os.Getenv("OS_AUTH_URL"), "OpenStack Keystone auth URL")
	fs.StringVar(&cfg.OpenStackUsername, "os-username", os.Getenv("OS_USERNAME"), "OpenStack username")
	fs.StringVar(&cfg.OpenStackPassword, "os-password", os.Getenv("OS_PASSWORD"), "OpenStack password")
	fs.StringVar(&cfg.OpenStackProjectName, "os-project-name", os.Getenv("OS_PROJECT_NAME"), "OpenStack project name")
	fs.StringVar(&cfg.OpenStackDomainName, "os-domain-name", os.Getenv("OS_USER_DOMAIN_NAME"), "OpenStack user domain name")
	fs.StringVar(&cfg.OpenStackRegionName, "os-region-name", os.Getenv("OS_REGION_NAME"), "OpenStack region name")

	var cacheTTLStr string
	fs.StringVar(&cacheTTLStr, "enrichment-cache-ttl", envOrDefault("NORTHWATCH_ENRICHMENT_CACHE_TTL", "5m"), "Enrichment cache TTL (e.g. 5m, 1h)")

	// Alerting flags
	fs.StringVar(&cfg.AlertWebhookURLs, "alert-webhook-urls", os.Getenv("NORTHWATCH_ALERT_WEBHOOK_URLS"), "Comma-separated webhook URLs for alert notifications")

	// History flags
	fs.StringVar(&cfg.HistoryDBPath, "history-db-path", envOrDefault("NORTHWATCH_HISTORY_DB_PATH", "northwatch-history.db"), "Path to SQLite history database")
	var snapshotIntervalStr string
	fs.StringVar(&snapshotIntervalStr, "snapshot-interval", envOrDefault("NORTHWATCH_SNAPSHOT_INTERVAL", "5m"), "Automatic snapshot interval (e.g. 5m, 1h)")
	var eventRetentionStr string
	fs.StringVar(&eventRetentionStr, "event-retention", envOrDefault("NORTHWATCH_EVENT_RETENTION", "24h"), "Event log retention duration (e.g. 24h, 7d)")
	fs.Int64Var(&cfg.EventMaxCount, "event-max-count", envOrDefaultInt64("NORTHWATCH_EVENT_MAX_COUNT", 0), "Maximum number of events to retain (0 = unlimited)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if cfg.OVNNBAddr == "" {
		return nil, fmt.Errorf("--ovn-nb-addr is required (or set NORTHWATCH_OVN_NB_ADDR)")
	}
	if cfg.OVNSBAddr == "" {
		return nil, fmt.Errorf("--ovn-sb-addr is required (or set NORTHWATCH_OVN_SB_ADDR)")
	}

	ttl, err := time.ParseDuration(cacheTTLStr)
	if err != nil {
		return nil, fmt.Errorf("invalid enrichment-cache-ttl: %w", err)
	}
	cfg.EnrichmentCacheTTL = ttl

	si, err := time.ParseDuration(snapshotIntervalStr)
	if err != nil {
		return nil, fmt.Errorf("invalid snapshot-interval: %w", err)
	}
	cfg.SnapshotInterval = si

	er, err := time.ParseDuration(eventRetentionStr)
	if err != nil {
		return nil, fmt.Errorf("invalid event-retention: %w", err)
	}
	cfg.EventRetention = er

	return cfg, nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envOrDefaultInt64(key string, defaultVal int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	var result int64
	if _, err := fmt.Sscanf(v, "%d", &result); err != nil {
		return defaultVal
	}
	return result
}
