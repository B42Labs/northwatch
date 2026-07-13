package main

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/b42labs/northwatch/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogs redirects the default slog logger to a buffer for the test's
// duration so assertions can inspect the posture warnings checkAuthPosture emits.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	prev := slog.Default()
	buf := &bytes.Buffer{}
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func TestCheckAuthPosture_ReadSurfaceWarning(t *testing.T) {
	tokens := map[string]string{"ops": "0123456789abcdef"}

	t.Run("non-loopback bind with tokens warns the read surface is unauthenticated", func(t *testing.T) {
		buf := captureLogs(t)
		cfg := &config.Config{Listen: "0.0.0.0:8080", APITokens: tokens}

		require.NoError(t, checkAuthPosture(cfg))

		out := buf.String()
		assert.Contains(t, out, "read surface")
		assert.Contains(t, out, "write-audit log")
	})

	t.Run("loopback bind with tokens does not warn about the read surface", func(t *testing.T) {
		buf := captureLogs(t)
		cfg := &config.Config{Listen: "127.0.0.1:8080", APITokens: tokens}

		require.NoError(t, checkAuthPosture(cfg))

		assert.NotContains(t, buf.String(), "read surface")
	})
}
