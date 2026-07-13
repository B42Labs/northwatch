package write

import (
	"context"
	"testing"

	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateNATReferences covers the NAT duplicate-detection reference check
// via Preview: a create that duplicates an existing external_ip+type is
// rejected, while a distinct one is accepted.
func TestValidateNATReferences(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupEngineWithClients(t, nbClient, nil)
	ctx := context.Background()

	// Seed a router carrying a dnat_and_snat NAT with external_ip 5.5.5.5.
	testutil.InsertGatewayRouter(t, nbClient, "nat-router", "nat-lrp", nil, []string{"5.5.5.5"})

	t.Run("duplicate external_ip and type is rejected", func(t *testing.T) {
		_, err := engine.Preview(ctx, []WriteOperation{{
			Action: "create", Table: "NAT",
			Fields: jsonFields(t, `{"type":"dnat_and_snat","external_ip":"5.5.5.5","logical_ip":"192.168.0.10"}`),
		}})
		require.Error(t, err)
		assert.True(t, IsInputError(err), "a duplicate NAT is a client error: %v", err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("distinct external_ip is accepted", func(t *testing.T) {
		plan, err := engine.Preview(ctx, []WriteOperation{{
			Action: "create", Table: "NAT",
			Fields: jsonFields(t, `{"type":"dnat_and_snat","external_ip":"6.6.6.6","logical_ip":"192.168.0.11"}`),
		}})
		require.NoError(t, err)
		assert.Equal(t, "pending", plan.Status)
	})
}

// TestValidateACLReferences covers the ACL priority-range check via Preview: an
// out-of-range priority is rejected, an in-range one passes.
func TestValidateACLReferences(t *testing.T) {
	nbClient := testutil.SetupNBTestClient(t)
	engine := setupEngineWithClients(t, nbClient, nil)
	ctx := context.Background()

	t.Run("priority above range is rejected", func(t *testing.T) {
		_, err := engine.Preview(ctx, []WriteOperation{{
			Action: "create", Table: "ACL",
			Fields: jsonFields(t, `{"action":"allow","direction":"from-lport","match":"ip4","priority":40000}`),
		}})
		require.Error(t, err)
		assert.True(t, IsInputError(err), "out-of-range ACL priority is a client error: %v", err)
		assert.Contains(t, err.Error(), "between 0 and 32767")
	})

	t.Run("in-range priority passes", func(t *testing.T) {
		plan, err := engine.Preview(ctx, []WriteOperation{{
			Action: "create", Table: "ACL",
			Fields: jsonFields(t, `{"action":"allow","direction":"from-lport","match":"ip4","priority":100}`),
		}})
		require.NoError(t, err)
		assert.Equal(t, "pending", plan.Status)
	})
}
