package ovnsim

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChooseActionIsDeterministic(t *testing.T) {
	seq := func() []string {
		rng := newRand(42)
		out := make([]string, 20)
		for i := range out {
			out[i] = chooseAction(rng, 6, 6, false)
		}
		return out
	}
	assert.Equal(t, seq(), seq(), "same seed must yield the same action sequence")
}

func TestChooseActionHomeostasis(t *testing.T) {
	// Below target: switch creation should dominate switch deletion.
	creates, deletes := 0, 0
	rng := newRand(7)
	for i := 0; i < 2000; i++ {
		switch chooseAction(rng, 1, 10, false) {
		case "switch.create":
			creates++
		case "switch.delete":
			deletes++
		}
	}
	assert.Greater(t, creates, deletes*3, "below target, creates should dominate")

	// Above target: deletion should dominate creation.
	creates, deletes = 0, 0
	for i := 0; i < 2000; i++ {
		switch chooseAction(rng, 20, 10, false) {
		case "switch.create":
			creates++
		case "switch.delete":
			deletes++
		}
	}
	assert.Greater(t, deletes, creates*3, "above target, deletes should dominate")
}

func TestChooseActionBinderGate(t *testing.T) {
	rng := newRand(1)
	sawBind := false
	for i := 0; i < 500; i++ {
		if a := chooseAction(rng, 6, 6, false); a == "bind" || a == "migrate" || a == "unbind" {
			sawBind = true
		}
	}
	assert.False(t, sawBind, "binder actions must not appear without a binder")

	sawBind = false
	sawUnbind := false
	for i := 0; i < 500; i++ {
		switch chooseAction(rng, 6, 6, true) {
		case "bind", "migrate":
			sawBind = true
		case "unbind":
			sawUnbind = true
		}
	}
	assert.True(t, sawBind, "binder actions should appear with a binder")
	assert.False(t, sawUnbind, "unbind is not a random action (would flood unbound-VIF alerts)")
}

// TestSimulatorStepsMutate runs many steps against a seeded database and checks
// the simulator keeps producing valid mutations without error and stays roughly
// bounded around the target.
func TestSimulatorStepsMutate(t *testing.T) {
	c := setupNB(t)
	_, err := Seed(context.Background(), c, Options{Switches: 5, Routers: 2, PortsPerSwitch: 3})
	require.NoError(t, err)
	eventuallyCount[nb.LogicalSwitch](t, c, 5)

	sim := NewSimulator(c, SimConfig{
		Options: Options{Switches: 5, Routers: 2, PortsPerSwitch: 3},
		Target:  5,
	})

	for i := 0; i < 60; i++ {
		desc, err := sim.Step(context.Background())
		require.NoErrorf(t, err, "step %d", i)
		require.NotEmpty(t, desc)
	}

	// Homeostasis keeps the switch count in a sane band around the target.
	var sws []nb.LogicalSwitch
	require.NoError(t, c.List(context.Background(), &sws))
	owned := 0
	for _, s := range sws {
		if owned2(s.ExternalIDs) {
			owned++
		}
	}
	assert.GreaterOrEqual(t, owned, 1)
	assert.LessOrEqual(t, owned, 15)
}

func owned2(ids map[string]string) bool { return ids[SimTag] != "" }

func TestSimulatorCreateDeleteRoundTrip(t *testing.T) {
	c := setupNB(t)
	sim := NewSimulator(c, SimConfig{Options: Options{Switches: 1, Routers: 1, PortsPerSwitch: 1}, Target: 3})

	desc, err := sim.createSwitch(context.Background())
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(desc, "create switch nw-ls-"))
	eventuallyCount[nb.LogicalSwitch](t, c, 1)

	desc, err = sim.createRouter(context.Background())
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(desc, "create router nw-lr-"))
	eventuallyCount[nb.LogicalRouter](t, c, 1)
}

func TestFreeIndex(t *testing.T) {
	assert.Equal(t, 1, freeIndex(map[int]bool{}))
	assert.Equal(t, 2, freeIndex(map[int]bool{1: true}))
	assert.Equal(t, 3, freeIndex(map[int]bool{1: true, 2: true, 4: true}))
}

func TestIndexFromName(t *testing.T) {
	idx, ok := indexFromName("nw-ls-007", "nw-ls-")
	assert.True(t, ok)
	assert.Equal(t, 7, idx)

	idx, ok = indexFromName("nw-ls-007-vif-001", "nw-ls-")
	assert.True(t, ok)
	assert.Equal(t, 7, idx)

	_, ok = indexFromName("other-1", "nw-ls-")
	assert.False(t, ok)
}

func TestRunRespectsContextCancel(t *testing.T) {
	c := setupNB(t)
	sim := NewSimulator(c, SimConfig{Options: Options{Switches: 2, Routers: 1, PortsPerSwitch: 1}, Target: 2})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := sim.Run(ctx, 5*time.Millisecond)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
