package inventory

import (
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/stretchr/testify/assert"
)

func TestComputeLiveness(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	const threshold = 30 * time.Second

	tests := []struct {
		name       string
		priv       *sb.ChassisPrivate
		sbNbCfg    int
		wantAlive  bool
		wantInSync bool
		wantStale  bool
	}{
		{
			name:      "missing chassis private is not alive",
			priv:      nil,
			sbNbCfg:   5,
			wantAlive: false,
		},
		{
			name:       "in sync is alive and not stale",
			priv:       &sb.ChassisPrivate{NbCfg: 5, NbCfgTimestamp: int(now.UnixMilli())},
			sbNbCfg:    5,
			wantAlive:  true,
			wantInSync: true,
		},
		{
			name:       "behind but young is not yet stale",
			priv:       &sb.ChassisPrivate{NbCfg: 4, NbCfgTimestamp: int(now.UnixMilli()) - 1_000},
			sbNbCfg:    5,
			wantAlive:  false,
			wantInSync: false,
			wantStale:  false,
		},
		{
			name:       "behind and old is stale",
			priv:       &sb.ChassisPrivate{NbCfg: 4, NbCfgTimestamp: int(now.UnixMilli()) - 60_000},
			sbNbCfg:    5,
			wantAlive:  false,
			wantInSync: false,
			wantStale:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lv := ComputeLiveness(tc.priv, tc.sbNbCfg, now, threshold)
			assert.Equal(t, tc.sbNbCfg, lv.SBNbCfg)
			assert.Equal(t, tc.wantAlive, lv.Alive)
			assert.Equal(t, tc.wantInSync, lv.InSync)
			assert.Equal(t, tc.wantStale, lv.Stale)
		})
	}
}
