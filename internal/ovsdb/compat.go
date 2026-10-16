package ovsdb

import (
	"fmt"
	"strings"
)

// MinOVNVersion and MinOVSVersion are the oldest OVN and Open vSwitch releases
// Northwatch supports. The generated nb, sb and vs models come from these
// releases' schemas (OVN_VERSION / OVS_VERSION in the Makefile). libovsdb
// rejects a server whose schema lacks a modeled table or column but accepts
// extra ones, so the models work against these releases and against newer
// ones, whose schemas only add to them.
const (
	MinOVNVersion = "24.03"
	MinOVSVersion = "3.3"
)

// compatibilityDocsURL documents the supported version range.
const compatibilityDocsURL = "https://b42labs.github.io/northwatch/reference/compatibility"

// withSchemaHint annotates a libovsdb schema-validation failure, which only
// lists the tables and columns the server lacks, with the release range
// Northwatch supports. product is "OVN" or "Open vSwitch". Any other error is
// returned unchanged.
func withSchemaHint(err error, product, minVersion string) error {
	// libovsdb reports the mismatch as a plain fmt.Errorf ("database %s
	// validation error (%d): ..."), so the message is the only thing to match.
	if err == nil || !strings.Contains(err.Error(), "validation error") {
		return err
	}
	return fmt.Errorf("incompatible %s schema (Northwatch supports %s %s and newer, see %s): %w",
		product, product, minVersion, compatibilityDocsURL, err)
}
