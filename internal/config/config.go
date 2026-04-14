package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

// ClusterConfig defines the connection and enrichment settings for a single OVN cluster.
type ClusterConfig struct {
	Name       string            `json:"name"`
	Label      string            `json:"label"`
	OVNNBAddr  string            `json:"ovn_nb_addr"`
	OVNSBAddr  string            `json:"ovn_sb_addr"`
	Enrichment *EnrichmentConfig `json:"enrichment,omitempty"`
}

// EnrichmentConfig defines optional enrichment provider settings for a cluster.
type EnrichmentConfig struct {
	Type                 string `json:"type"` // "openstack" or "kubernetes"
	OpenStackAuthURL     string `json:"os_auth_url,omitempty"`
	OpenStackUsername     string `json:"os_username,omitempty"`
	OpenStackPassword    string `json:"os_password,omitempty"`
	OpenStackProjectName string `json:"os_project_name,omitempty"`
	OpenStackDomainName  string `json:"os_domain_name,omitempty"`
	OpenStackRegionName  string `json:"os_region_name,omitempty"`
	Kubeconfig           string `json:"kubeconfig,omitempty"`
	KubeContext          string `json:"kube_context,omitempty"`
}

type Config struct {
	Listen    string
	OVNNBAddr string
	OVNSBAddr string

	// Multi-cluster config file
	ConfigFile string          // --config-file / NORTHWATCH_CONFIG_FILE
	Clusters   []ClusterConfig // populated from file or from flat flags

	// OpenStack enrichment (all optional)
	OpenStackAuthURL     string
	OpenStackUsername     string
	OpenStackPassword    string
	OpenStackProjectName string
	OpenStackDomainName  string
	OpenStackRegionName  string
	EnrichmentCacheTTL   time.Duration

	// Kubernetes enrichment (all optional)
	KubeEnrichment bool
	Kubeconfig     string
	KubeContext    string

	// Alerting
	AlertWebhookURLs string // comma-separated webhook URLs

	// WebSocket origin allowlist (comma-separated host patterns).
	// Empty disables origin checking, suitable for single-tenant deployments
	// behind an operator-controlled reverse proxy.
	WSAllowedOrigins string

	// Write operations
	WriteEnabled   bool
	WritePlanTTL   time.Duration
	WriteRateLimit int

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

	// Multi-cluster config file
	fs.StringVar(&cfg.ConfigFile, "config-file", os.Getenv("NORTHWATCH_CONFIG_FILE"), "Path to JSON config file for multi-cluster")

	// OpenStack enrichment flags
	fs.StringVar(&cfg.OpenStackAuthURL, "os-auth-url", os.Getenv("OS_AUTH_URL"), "OpenStack Keystone auth URL")
	fs.StringVar(&cfg.OpenStackUsername, "os-username", os.Getenv("OS_USERNAME"), "OpenStack username")
	fs.StringVar(&cfg.OpenStackPassword, "os-password", os.Getenv("OS_PASSWORD"), "OpenStack password")
	fs.StringVar(&cfg.OpenStackProjectName, "os-project-name", os.Getenv("OS_PROJECT_NAME"), "OpenStack project name")
	fs.StringVar(&cfg.OpenStackDomainName, "os-domain-name", os.Getenv("OS_USER_DOMAIN_NAME"), "OpenStack user domain name")
	fs.StringVar(&cfg.OpenStackRegionName, "os-region-name", os.Getenv("OS_REGION_NAME"), "OpenStack region name")

	// Kubernetes enrichment flags
	fs.BoolVar(&cfg.KubeEnrichment, "kube-enrichment", envOrDefaultBool("NORTHWATCH_KUBE_ENRICHMENT", false), "Enable Kubernetes enrichment")
	fs.StringVar(&cfg.Kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "Path to kubeconfig file")
	fs.StringVar(&cfg.KubeContext, "kube-context", os.Getenv("NORTHWATCH_KUBE_CONTEXT"), "Kubeconfig context to use")

	var cacheTTLStr string
	fs.StringVar(&cacheTTLStr, "enrichment-cache-ttl", envOrDefault("NORTHWATCH_ENRICHMENT_CACHE_TTL", "5m"), "Enrichment cache TTL (e.g. 5m, 1h)")

	// Alerting flags
	fs.StringVar(&cfg.AlertWebhookURLs, "alert-webhook-urls", os.Getenv("NORTHWATCH_ALERT_WEBHOOK_URLS"), "Comma-separated webhook URLs for alert notifications")

	// WebSocket flags
	fs.StringVar(&cfg.WSAllowedOrigins, "ws-allowed-origins", os.Getenv("NORTHWATCH_WS_ALLOWED_ORIGINS"), "Comma-separated allowed Origin host patterns for WebSocket connections (empty = disable origin check)")

	// Write operation flags
	fs.BoolVar(&cfg.WriteEnabled, "write-enabled", envOrDefaultBool("NORTHWATCH_WRITE_ENABLED", false), "Enable write operations to OVN NB")
	var writePlanTTLStr string
	fs.StringVar(&writePlanTTLStr, "write-plan-ttl", envOrDefault("NORTHWATCH_WRITE_PLAN_TTL", "10m"), "TTL for write operation plans (e.g. 10m, 1h)")
	fs.IntVar(&cfg.WriteRateLimit, "write-rate-limit", envOrDefaultInt("NORTHWATCH_WRITE_RATE_LIMIT", 30), "Maximum write operations per minute")

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

	// Load cluster config from file if specified
	if cfg.ConfigFile != "" {
		if err := loadConfigFile(cfg); err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	} else {
		// Require NB/SB addresses when not using a config file
		if cfg.OVNNBAddr == "" {
			return nil, fmt.Errorf("--ovn-nb-addr is required (or set NORTHWATCH_OVN_NB_ADDR)")
		}
		if cfg.OVNSBAddr == "" {
			return nil, fmt.Errorf("--ovn-sb-addr is required (or set NORTHWATCH_OVN_SB_ADDR)")
		}

		// Synthesize a single-element Clusters slice from flat flags
		cc := ClusterConfig{
			Name:      "default",
			Label:     "Default",
			OVNNBAddr: cfg.OVNNBAddr,
			OVNSBAddr: cfg.OVNSBAddr,
		}
		if cfg.OpenStackAuthURL != "" {
			cc.Enrichment = &EnrichmentConfig{
				Type:                 "openstack",
				OpenStackAuthURL:     cfg.OpenStackAuthURL,
				OpenStackUsername:     cfg.OpenStackUsername,
				OpenStackPassword:    cfg.OpenStackPassword,
				OpenStackProjectName: cfg.OpenStackProjectName,
				OpenStackDomainName:  cfg.OpenStackDomainName,
				OpenStackRegionName:  cfg.OpenStackRegionName,
			}
		} else if cfg.KubeEnrichment {
			cc.Enrichment = &EnrichmentConfig{
				Type:        "kubernetes",
				Kubeconfig:  cfg.Kubeconfig,
				KubeContext: cfg.KubeContext,
			}
		}
		cfg.Clusters = []ClusterConfig{cc}
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

	wpt, err := time.ParseDuration(writePlanTTLStr)
	if err != nil {
		return nil, fmt.Errorf("invalid write-plan-ttl: %w", err)
	}
	cfg.WritePlanTTL = wpt

	return cfg, nil
}

// configFile is the JSON structure for the multi-cluster config file.
type configFile struct {
	Clusters []ClusterConfig `json:"clusters"`
}

// loadConfigFile reads and parses a JSON config file for multi-cluster setup.
func loadConfigFile(cfg *Config) error {
	data, err := os.ReadFile(cfg.ConfigFile)
	if err != nil {
		return fmt.Errorf("reading %s: %w", cfg.ConfigFile, err)
	}

	var cf configFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return fmt.Errorf("parsing %s: %w", cfg.ConfigFile, err)
	}

	if len(cf.Clusters) == 0 {
		return fmt.Errorf("config file must define at least one cluster")
	}

	for i, c := range cf.Clusters {
		if c.Name == "" {
			return fmt.Errorf("cluster[%d]: name is required", i)
		}
		if c.OVNNBAddr == "" {
			return fmt.Errorf("cluster[%d] %q: ovn_nb_addr is required", i, c.Name)
		}
		if c.OVNSBAddr == "" {
			return fmt.Errorf("cluster[%d] %q: ovn_sb_addr is required", i, c.Name)
		}
		if c.Label == "" {
			cf.Clusters[i].Label = c.Name
		}
	}

	cfg.Clusters = cf.Clusters
	return nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envOrDefaultBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	return v == "true" || v == "1" || v == "yes"
}

func envOrDefaultInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	var result int
	if _, err := fmt.Sscanf(v, "%d", &result); err != nil {
		return defaultVal
	}
	return result
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
