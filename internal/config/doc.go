// Package config parses Northwatch's runtime configuration from command-line
// flags and environment variables (and, for multi-cluster deployments, a JSON
// config file). There is no YAML configuration.
//
// Precedence is flag > environment variable > built-in default: every flag has a
// matching NORTHWATCH_* (or well-known OS_*/KUBECONFIG) environment variable used
// as its default, which an explicit flag overrides. Parsing is strict — a
// malformed boolean, integer, duration, log level or log format fails Parse
// loudly rather than silently falling back to a default, so a configuration typo
// cannot quietly change behavior.
package config
