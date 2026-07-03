package inventory

import (
	"context"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// ChassisByUUID resolves a single SB Chassis by its UUID via a client point-get.
// It returns (chassis, true) on success or (nil, false) when the UUID is empty
// or cannot be resolved. It centralizes the UUID->Chassis lookup shared by the
// OVS correlator and the port diagnostics so they cannot diverge.
func ChassisByUUID(ctx context.Context, c client.Client, uuid string) (*sb.Chassis, bool) {
	if uuid == "" {
		return nil, false
	}
	ch := &sb.Chassis{UUID: uuid}
	if err := c.Get(ctx, ch); err != nil {
		return nil, false
	}
	return ch, true
}
