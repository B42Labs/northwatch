package debug

import (
	"testing"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/stretchr/testify/assert"
)

func TestMatchRelation(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		expected matchRelationType
	}{
		{"equal", `inport == "port1" && ip4.dst == 10.0.0.1`, `inport == "port1" && ip4.dst == 10.0.0.1`, matchEqual},
		{"superset - match-all", "1", `inport == "port1"`, matchSuperset},
		{"subset - match-all", `inport == "port1"`, "1", matchSubset},
		{"superset - fewer conjuncts", `inport == "port1"`, `inport == "port1" && ip4.dst == 10.0.0.1`, matchSuperset},
		{"subset - more conjuncts", `inport == "port1" && ip4.dst == 10.0.0.1`, `inport == "port1"`, matchSubset},
		{"disjoint", `inport == "port1"`, `inport == "port2"`, matchDisjoint},
		{"conflict", `inport == "port1" && ip4.dst == 10.0.0.1`, `inport == "port1" && ip4.dst == 10.0.0.2`, matchConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, matchRelation(tt.a, tt.b))
		})
	}
}

func TestParseConjuncts(t *testing.T) {
	result := parseConjuncts(`inport == "port1" && ip4.dst == 10.0.0.1 && tcp.dst == 80`)
	assert.Len(t, result, 3)
}

func TestCompareACLs_Shadow(t *testing.T) {
	higher := nb.ACL{UUID: "h", Priority: 1000, Direction: "from-lport", Match: `inport == "port1"`, Action: "drop"}
	lower := nb.ACL{UUID: "l", Priority: 900, Direction: "from-lport", Match: `inport == "port1" && ip4.dst == 10.0.0.1`, Action: "allow"}
	findings := compareACLs(higher, lower, map[string]string{})
	assert.Len(t, findings, 1)
	assert.Equal(t, "shadow", findings[0].Type)
}

func TestCompareACLs_Redundant(t *testing.T) {
	higher := nb.ACL{UUID: "h", Priority: 1000, Direction: "from-lport", Match: `inport == "port1"`, Action: "drop"}
	lower := nb.ACL{UUID: "l", Priority: 900, Direction: "from-lport", Match: `inport == "port1"`, Action: "drop"}
	findings := compareACLs(higher, lower, map[string]string{})
	assert.Len(t, findings, 1)
	assert.Equal(t, "redundant", findings[0].Type)
}

func TestCompareACLs_Conflict(t *testing.T) {
	higher := nb.ACL{UUID: "h", Priority: 1000, Direction: "from-lport", Match: `inport == "port1" && ip4.dst == 10.0.0.1`, Action: "drop"}
	lower := nb.ACL{UUID: "l", Priority: 900, Direction: "from-lport", Match: `inport == "port1" && ip4.dst == 10.0.0.2`, Action: "allow"}
	findings := compareACLs(higher, lower, map[string]string{})
	assert.Len(t, findings, 1)
	assert.Equal(t, "conflict", findings[0].Type)
}
