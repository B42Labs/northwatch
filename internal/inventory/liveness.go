package inventory

import (
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// ComputeLiveness derives in-sync, alive and stale from the nb_cfg generation,
// comparing a chassis's Chassis_Private.nb_cfg against the SB_Global.nb_cfg the
// central components have written. This is the single source of truth for
// chassis liveness, shared by the inventory builder and the gateway analyzer so
// their logic cannot diverge.
//
// A nil priv (no Chassis_Private row for the chassis) yields a zero-value
// Liveness (not in-sync, not alive), except SBNbCfg which is always populated
// for context.
func ComputeLiveness(priv *sb.ChassisPrivate, sbNbCfg int, now time.Time, staleThreshold time.Duration) Liveness {
	l := Liveness{SBNbCfg: sbNbCfg}
	if priv == nil {
		return l
	}
	l.NbCfg = priv.NbCfg
	l.NbCfgTimestamp = int64(priv.NbCfgTimestamp)
	l.InSync = priv.NbCfg == sbNbCfg
	// A present-and-in-sync chassis is alive. We do NOT key alive off the age of
	// nb_cfg_timestamp: that timestamp only advances when nb_cfg itself changes
	// (see ovn-sb(5), Chassis_Private:nb_cfg_timestamp), so on a steady-state
	// cluster with no config churn it freezes and an age check would mark every
	// healthy chassis down once the staleness window elapses.
	l.Alive = l.InSync
	if priv.NbCfgTimestamp > 0 {
		// AgeMs is informational only. nb_cfg_timestamp is written by the
		// chassis's own ovn-controller clock, foreign relative to now; a future
		// timestamp (age < 0) means clock skew and is clamped to 0 rather than
		// leaking a negative age into the response.
		if age := now.UnixMilli() - int64(priv.NbCfgTimestamp); age > 0 {
			l.AgeMs = age
		}
		// Stale is the age-based signal, scoped to an out-of-sync chassis: it has
		// received a newer nb_cfg generation but has not acknowledged it within
		// staleThreshold, i.e. it is lagging/stuck rather than merely mid-update.
		l.Stale = !l.InSync && l.AgeMs > staleThreshold.Milliseconds()
	}
	return l
}
