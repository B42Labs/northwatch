package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultMonitorBatchDelay is the default delay between staged per-table monitor
// requests for live OVN connections (server mode and snapshot capture). Staging
// spreads the initial-dump load so a large deployment isn't serialized into one
// giant monitor reply at startup. Offline snapshot replay overrides this to 0,
// since it connects to local in-memory servers with nothing to protect.
const DefaultMonitorBatchDelay = 200 * time.Millisecond

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
	OpenStackUsername    string `json:"os_username,omitempty"`
	OpenStackPassword    string `json:"os_password,omitempty"`
	OpenStackProjectName string `json:"os_project_name,omitempty"`
	OpenStackDomainName  string `json:"os_domain_name,omitempty"`
	OpenStackRegionName  string `json:"os_region_name,omitempty"`
	OpenStackCACert      string `json:"os_cacert,omitempty"`
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

	// Offline snapshot mode: path to a snapshot file to serve instead of
	// connecting to a live OVN deployment. When set, the NB/SB addresses are
	// synthesized from local in-memory servers backing the snapshot.
	SnapshotFile string // --snapshot / NORTHWATCH_SNAPSHOT

	// Initial monitor tuning. These bound the load Northwatch puts on the OVN
	// databases while building the initial cache (live startup and snapshot
	// capture). MonitorBatchDelay > 0 spreads the initial dump over staged
	// per-table monitor requests; MonitorSkipTables excludes the largest tables
	// entirely. Both default to off (single MonitorAll over every table).
	MonitorBatchDelay time.Duration // --monitor-batch-delay / NORTHWATCH_MONITOR_BATCH_DELAY
	MonitorSkipTables []string      // --monitor-skip-tables / NORTHWATCH_MONITOR_SKIP_TABLES

	// OpenStack enrichment (all optional)
	OpenStackAuthURL     string
	OpenStackUsername    string
	OpenStackPassword    string
	OpenStackProjectName string
	OpenStackDomainName  string
	OpenStackRegionName  string
	OpenStackCACert      string // PEM CA bundle for verifying the OpenStack API (clouds.yaml `cacert`)
	EnrichmentCacheTTL   time.Duration

	// Kubernetes enrichment (all optional)
	KubeEnrichment bool
	Kubeconfig     string
	KubeContext    string

	// Chassis inventory staleness: how long an out-of-sync chassis may lag the
	// current nb_cfg generation before the aggregated chassis inventory flags it
	// as stale (lagging/stuck). It does not affect alive/down — a chassis is
	// alive whenever it is present and in-sync — so a steady-state cluster with a
	// frozen nb_cfg_timestamp stays healthy regardless of this value.
	ChassisStaleThreshold time.Duration // --chassis-stale-threshold / NORTHWATCH_CHASSIS_STALE_THRESHOLD

	// Per-chassis Open_vSwitch (OVS) visibility. Opt-in and disabled by default:
	// the integration is enabled only when OVSMgmtAddrs is non-empty, populated
	// from the JSON mapping file given by --ovs-mgmt-addr-file. The map keys are
	// chassis system-ids (matching SB Chassis.name / OVS external_ids:system-id)
	// and the values are OVSDB management addresses (e.g. "tcp:10.0.0.1:6640" or
	// "ssl:10.0.0.1:6640"). The TLS material is required only for ssl: endpoints.
	OVSMgmtAddrs map[string]string // --ovs-mgmt-addr-file / NORTHWATCH_OVS_MGMT_ADDR_FILE
	OVSTLSCert   string            // --ovs-tls-cert / NORTHWATCH_OVS_TLS_CERT
	OVSTLSKey    string            // --ovs-tls-key / NORTHWATCH_OVS_TLS_KEY
	OVSTLSCA     string            // --ovs-tls-ca / NORTHWATCH_OVS_TLS_CA

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

	// Logging. LogLevel is one of debug|info|warn|error; LogFormat is text|json.
	LogLevel  string // --log-level / NORTHWATCH_LOG_LEVEL
	LogFormat string // --log-format / NORTHWATCH_LOG_FORMAT
}

func Parse(args []string) (*Config, error) {
	fs := flag.NewFlagSet("northwatch", flag.ContinueOnError)

	cfg := &Config{}
	// Malformed environment defaults are collected here and reported together
	// after flag parsing, mirroring the strict duration paths below, so a typo
	// like NORTHWATCH_WRITE_ENABLED=True fails loudly instead of silently
	// meaning false.
	var envErrs []error
	fs.StringVar(&cfg.Listen, "listen", envOrDefault("NORTHWATCH_LISTEN", ":8080"), "HTTP listen address")
	fs.StringVar(&cfg.OVNNBAddr, "ovn-nb-addr", os.Getenv("NORTHWATCH_OVN_NB_ADDR"), "OVN Northbound DB address, comma-separated for failover (e.g. tcp:10.0.0.1:6641,tcp:10.0.0.2:6641)")
	fs.StringVar(&cfg.OVNSBAddr, "ovn-sb-addr", os.Getenv("NORTHWATCH_OVN_SB_ADDR"), "OVN Southbound DB address, comma-separated for failover (e.g. tcp:10.0.0.1:6642,tcp:10.0.0.2:6642)")

	// Multi-cluster config file
	fs.StringVar(&cfg.ConfigFile, "config-file", os.Getenv("NORTHWATCH_CONFIG_FILE"), "Path to JSON config file for multi-cluster")

	// Offline snapshot mode
	fs.StringVar(&cfg.SnapshotFile, "snapshot", os.Getenv("NORTHWATCH_SNAPSHOT"), "Path to a snapshot file to serve offline (no live OVN connection; takes precedence over --ovn-*-addr and --config-file)")

	// Initial monitor tuning (limits load on large OVN databases at startup)
	var monitorBatchDelayStr string
	fs.StringVar(&monitorBatchDelayStr, "monitor-batch-delay", envOrDefault("NORTHWATCH_MONITOR_BATCH_DELAY", DefaultMonitorBatchDelay.String()), "Delay between staged per-table monitor requests on connect (e.g. 100ms, 1s); 0 loads all tables in a single request (offline --snapshot replay always uses 0)")
	var monitorSkipTablesStr string
	fs.StringVar(&monitorSkipTablesStr, "monitor-skip-tables", os.Getenv("NORTHWATCH_MONITOR_SKIP_TABLES"), "Comma-separated OVN table names to never monitor (e.g. Logical_Flow,MAC_Binding,FDB); reduces initial load on huge deployments")

	// OpenStack enrichment flags
	fs.StringVar(&cfg.OpenStackAuthURL, "os-auth-url", os.Getenv("OS_AUTH_URL"), "OpenStack Keystone auth URL")
	fs.StringVar(&cfg.OpenStackUsername, "os-username", os.Getenv("OS_USERNAME"), "OpenStack username")
	fs.StringVar(&cfg.OpenStackPassword, "os-password", os.Getenv("OS_PASSWORD"), "OpenStack password")
	fs.StringVar(&cfg.OpenStackProjectName, "os-project-name", os.Getenv("OS_PROJECT_NAME"), "OpenStack project name")
	fs.StringVar(&cfg.OpenStackDomainName, "os-domain-name", os.Getenv("OS_USER_DOMAIN_NAME"), "OpenStack user domain name")
	fs.StringVar(&cfg.OpenStackRegionName, "os-region-name", os.Getenv("OS_REGION_NAME"), "OpenStack region name")
	fs.StringVar(&cfg.OpenStackCACert, "os-cacert", os.Getenv("OS_CACERT"), "Path to a PEM CA bundle for verifying the OpenStack API (maps to clouds.yaml `cacert`)")

	// Kubernetes enrichment flags
	fs.BoolVar(&cfg.KubeEnrichment, "kube-enrichment", envOrDefaultBool("NORTHWATCH_KUBE_ENRICHMENT", false, &envErrs), "Enable Kubernetes enrichment")
	fs.StringVar(&cfg.Kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "Path to kubeconfig file")
	fs.StringVar(&cfg.KubeContext, "kube-context", os.Getenv("NORTHWATCH_KUBE_CONTEXT"), "Kubeconfig context to use")

	var cacheTTLStr string
	fs.StringVar(&cacheTTLStr, "enrichment-cache-ttl", envOrDefault("NORTHWATCH_ENRICHMENT_CACHE_TTL", "5m"), "Enrichment cache TTL (e.g. 5m, 1h)")

	var chassisStaleStr string
	fs.StringVar(&chassisStaleStr, "chassis-stale-threshold", envOrDefault("NORTHWATCH_CHASSIS_STALE_THRESHOLD", "60s"), "How long an out-of-sync chassis may lag the current nb_cfg generation before it is flagged stale in the chassis inventory and treated as not alive for gateway HA-member election (Go duration)")

	// Per-chassis OVS visibility flags (opt-in, disabled unless the mapping file
	// is set). TLS material is required only for ssl: management addresses.
	var ovsMgmtAddrFile string
	fs.StringVar(&ovsMgmtAddrFile, "ovs-mgmt-addr-file", os.Getenv("NORTHWATCH_OVS_MGMT_ADDR_FILE"), "Path to a JSON file mapping chassis system-id to OVSDB management address ({\"<system-id>\": \"ssl:<ip>:6640\"}); enables opt-in per-chassis Open_vSwitch visibility")
	fs.StringVar(&cfg.OVSTLSCert, "ovs-tls-cert", os.Getenv("NORTHWATCH_OVS_TLS_CERT"), "Path to the client certificate (PEM) for ssl: OVS management connections")
	fs.StringVar(&cfg.OVSTLSKey, "ovs-tls-key", os.Getenv("NORTHWATCH_OVS_TLS_KEY"), "Path to the client private key (PEM) for ssl: OVS management connections")
	fs.StringVar(&cfg.OVSTLSCA, "ovs-tls-ca", os.Getenv("NORTHWATCH_OVS_TLS_CA"), "Path to the CA bundle (PEM) verifying ssl: OVS management servers")

	// Alerting flags
	fs.StringVar(&cfg.AlertWebhookURLs, "alert-webhook-urls", os.Getenv("NORTHWATCH_ALERT_WEBHOOK_URLS"), "Comma-separated webhook URLs for alert notifications")

	// WebSocket flags
	fs.StringVar(&cfg.WSAllowedOrigins, "ws-allowed-origins", os.Getenv("NORTHWATCH_WS_ALLOWED_ORIGINS"), "Comma-separated allowed Origin host patterns for WebSocket connections (empty = disable origin check)")

	// Logging flags
	fs.StringVar(&cfg.LogLevel, "log-level", envOrDefault("NORTHWATCH_LOG_LEVEL", "info"), "Log level: debug, info, warn or error")
	fs.StringVar(&cfg.LogFormat, "log-format", envOrDefault("NORTHWATCH_LOG_FORMAT", "text"), "Log output format: text or json")

	// Write operation flags
	fs.BoolVar(&cfg.WriteEnabled, "write-enabled", envOrDefaultBool("NORTHWATCH_WRITE_ENABLED", false, &envErrs), "Enable write operations to OVN NB")
	var writePlanTTLStr string
	fs.StringVar(&writePlanTTLStr, "write-plan-ttl", envOrDefault("NORTHWATCH_WRITE_PLAN_TTL", "10m"), "TTL for write operation plans (e.g. 10m, 1h)")
	fs.IntVar(&cfg.WriteRateLimit, "write-rate-limit", envOrDefaultInt("NORTHWATCH_WRITE_RATE_LIMIT", 30, &envErrs), "Maximum write operations per minute")

	// History flags
	fs.StringVar(&cfg.HistoryDBPath, "history-db-path", envOrDefault("NORTHWATCH_HISTORY_DB_PATH", "northwatch-history.db"), "Path to SQLite history database")
	var snapshotIntervalStr string
	fs.StringVar(&snapshotIntervalStr, "snapshot-interval", envOrDefault("NORTHWATCH_SNAPSHOT_INTERVAL", "5m"), "Automatic snapshot interval (e.g. 5m, 1h)")
	var eventRetentionStr string
	fs.StringVar(&eventRetentionStr, "event-retention", envOrDefault("NORTHWATCH_EVENT_RETENTION", "24h"), "Event log retention duration (e.g. 24h, 7d)")
	fs.Int64Var(&cfg.EventMaxCount, "event-max-count", envOrDefaultInt64("NORTHWATCH_EVENT_MAX_COUNT", 0, &envErrs), "Maximum number of events to retain (0 = unlimited)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	// Fail on any malformed environment default (collected above) before doing
	// any further work, so a bad NORTHWATCH_* value never silently defaults.
	if err := errors.Join(envErrs...); err != nil {
		return nil, err
	}

	if err := validateLogLevel(cfg.LogLevel); err != nil {
		return nil, err
	}
	if err := validateLogFormat(cfg.LogFormat); err != nil {
		return nil, err
	}

	// Offline snapshot mode takes precedence: the cluster list is synthesized
	// in main from the in-memory servers backing the snapshot, so no live
	// addresses or config file are required here.
	if cfg.SnapshotFile != "" {
		// nothing to validate; clusters are filled in once the snapshot is served
	} else if cfg.ConfigFile != "" {
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
				OpenStackUsername:    cfg.OpenStackUsername,
				OpenStackPassword:    cfg.OpenStackPassword,
				OpenStackProjectName: cfg.OpenStackProjectName,
				OpenStackDomainName:  cfg.OpenStackDomainName,
				OpenStackRegionName:  cfg.OpenStackRegionName,
				OpenStackCACert:      cfg.OpenStackCACert,
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

	cst, err := time.ParseDuration(chassisStaleStr)
	if err != nil {
		return nil, fmt.Errorf("invalid chassis-stale-threshold: %w", err)
	}
	if cst <= 0 {
		// A zero (or negative) threshold would flag every out-of-sync chassis as
		// stale the instant it falls behind, leaving no grace for normal config
		// propagation. Reject it rather than ship that noise.
		return nil, fmt.Errorf("chassis-stale-threshold must be positive")
	}
	cfg.ChassisStaleThreshold = cst

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

	mbd, err := time.ParseDuration(monitorBatchDelayStr)
	if err != nil {
		return nil, fmt.Errorf("invalid monitor-batch-delay: %w", err)
	}
	if mbd < 0 {
		return nil, fmt.Errorf("monitor-batch-delay must not be negative")
	}
	cfg.MonitorBatchDelay = mbd
	cfg.MonitorSkipTables = SplitCSV(monitorSkipTablesStr)

	// OVS visibility is opt-in: only load the system-id → mgmt-addr mapping when a
	// file is given, leaving OVSMgmtAddrs nil (disabled) otherwise.
	if ovsMgmtAddrFile != "" {
		addrs, err := loadOVSMgmtAddrs(ovsMgmtAddrFile)
		if err != nil {
			return nil, fmt.Errorf("loading OVS mgmt-addr file: %w", err)
		}
		if err := requireOVSTLSForSSL(addrs, cfg.OVSTLSCert, cfg.OVSTLSKey, cfg.OVSTLSCA); err != nil {
			return nil, err
		}
		cfg.OVSMgmtAddrs = addrs
	}

	return cfg, nil
}

// loadOVSMgmtAddrs reads a JSON object mapping chassis system-id to OVSDB
// management address ({"<system-id>": "ssl:<ip>:6640"}). It rejects an empty
// map and any empty key or value so a malformed mapping fails loudly at startup
// rather than silently enabling zero chassis.
func loadOVSMgmtAddrs(path string) (map[string]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied mapping-file path (flag/env), not attacker input
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var addrs map[string]string
	if err := json.Unmarshal(data, &addrs); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("%s must map at least one system-id to an address", path)
	}
	for id, addr := range addrs {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("%s: empty system-id key", path)
		}
		if strings.TrimSpace(addr) == "" {
			return nil, fmt.Errorf("%s: empty address for system-id %q", path, id)
		}
	}
	return addrs, nil
}

// requireOVSTLSForSSL rejects any ssl: management address when no OVS TLS
// cert/key/CA is configured. Without client TLS material such a connection can
// never complete a handshake; left unchecked, every ssl: member would retry
// forever behind a generic "unreachable" log line with no hint at the real
// cause. A partial cert/key/CA set is caught later by BuildTLSConfig, so this
// only guards the all-empty case (which mirrors BuildTLSConfig returning nil).
func requireOVSTLSForSSL(addrs map[string]string, certFile, keyFile, caFile string) error {
	if certFile != "" || keyFile != "" || caFile != "" {
		return nil
	}
	for id, addr := range addrs {
		for _, ep := range strings.Split(addr, ",") {
			if strings.HasPrefix(strings.TrimSpace(ep), "ssl:") {
				return fmt.Errorf("chassis %q uses ssl: address %q but no OVS TLS material configured (--ovs-tls-cert/--ovs-tls-key/--ovs-tls-ca)", id, addr)
			}
		}
	}
	return nil
}

// SplitCSV parses a comma-separated list into a slice, trimming whitespace and
// dropping empty entries. Returns nil for an empty input.
func SplitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
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

// envOrDefaultBool parses a boolean environment default strictly. It accepts the
// strconv.ParseBool forms (1/t/T/TRUE/true/True/0/f/F/FALSE/false/False) plus
// yes/no for backwards compatibility. A non-empty but unparseable value appends a
// descriptive error to errs (so it fails Parse) instead of silently defaulting.
func envOrDefaultBool(key string, defaultVal bool, errs *[]error) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes":
		return true
	case "no":
		return false
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		*errs = append(*errs, fmt.Errorf("invalid %s %q: want a boolean (true/false/1/0/yes/no)", key, v))
		return defaultVal
	}
	return b
}

// envOrDefaultInt parses an integer environment default strictly, appending an
// error to errs on a non-empty unparseable value rather than silently defaulting.
func envOrDefaultInt(key string, defaultVal int, errs *[]error) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	result, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		*errs = append(*errs, fmt.Errorf("invalid %s %q: want an integer", key, v))
		return defaultVal
	}
	return result
}

// envOrDefaultInt64 parses a 64-bit integer environment default strictly,
// appending an error to errs on a non-empty unparseable value.
func envOrDefaultInt64(key string, defaultVal int64, errs *[]error) int64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	result, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("invalid %s %q: want an integer", key, v))
		return defaultVal
	}
	return result
}

// validateLogLevel rejects any --log-level / NORTHWATCH_LOG_LEVEL outside the
// supported set.
func validateLogLevel(level string) error {
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("invalid log-level %q: want debug, info, warn or error", level)
	}
}

// validateLogFormat rejects any --log-format / NORTHWATCH_LOG_FORMAT outside the
// supported set.
func validateLogFormat(format string) error {
	switch format {
	case "text", "json":
		return nil
	default:
		return fmt.Errorf("invalid log-format %q: want text or json", format)
	}
}
