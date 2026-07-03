// Package severity defines the shared healthy/warning/error status vocabulary
// used by the gateway analyzer, router diagnostics and debug port diagnostics,
// and surfaced verbatim to the frontend severity rendering (lib/status.ts).
//
// The constants are deliberately untyped so they assign to both plain string
// fields and typed aliases such as debug.DiagnosticSeverity without conversion.
package severity

const (
	// Healthy indicates no problem was found.
	Healthy = "healthy"
	// Warning indicates a degraded but non-fatal condition.
	Warning = "warning"
	// Error indicates a fault that needs attention.
	Error = "error"
)
