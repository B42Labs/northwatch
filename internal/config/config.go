package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	Listen    string
	OVNNBAddr string
	OVNSBAddr string
}

func Parse(args []string) (*Config, error) {
	fs := flag.NewFlagSet("northwatch", flag.ContinueOnError)

	cfg := &Config{}
	fs.StringVar(&cfg.Listen, "listen", envOrDefault("NORTHWATCH_LISTEN", ":8080"), "HTTP listen address")
	fs.StringVar(&cfg.OVNNBAddr, "ovn-nb-addr", os.Getenv("NORTHWATCH_OVN_NB_ADDR"), "OVN Northbound DB address (e.g. tcp:127.0.0.1:6641)")
	fs.StringVar(&cfg.OVNSBAddr, "ovn-sb-addr", os.Getenv("NORTHWATCH_OVN_SB_ADDR"), "OVN Southbound DB address (e.g. tcp:127.0.0.1:6642)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if cfg.OVNNBAddr == "" {
		return nil, fmt.Errorf("--ovn-nb-addr is required (or set NORTHWATCH_OVN_NB_ADDR)")
	}
	if cfg.OVNSBAddr == "" {
		return nil, fmt.Errorf("--ovn-sb-addr is required (or set NORTHWATCH_OVN_SB_ADDR)")
	}

	return cfg, nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
