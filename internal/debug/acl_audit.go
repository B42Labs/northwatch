package debug

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// ACLAuditSeverity represents the severity of an ACL audit finding.
type ACLAuditSeverity string

const (
	AuditSeverityInfo    ACLAuditSeverity = "info"
	AuditSeverityWarning ACLAuditSeverity = "warning"
	AuditSeverityError   ACLAuditSeverity = "error"
)

// ACLAuditFinding represents a single finding from the ACL audit.
type ACLAuditFinding struct {
	Type            string           `json:"type"` // "shadow", "conflict", "redundant"
	Severity        ACLAuditSeverity `json:"severity"`
	Message         string           `json:"message"`
	ACLUUID         string           `json:"acl_uuid"`
	ACLPriority     int              `json:"acl_priority"`
	ACLMatch        string           `json:"acl_match"`
	ACLAction       string           `json:"acl_action"`
	ACLDirection    string           `json:"acl_direction"`
	RelatedUUID     string           `json:"related_uuid,omitempty"`
	RelatedPriority int              `json:"related_priority,omitempty"`
	RelatedMatch    string           `json:"related_match,omitempty"`
	RelatedAction   string           `json:"related_action,omitempty"`
	Context         string           `json:"context,omitempty"`
}

// ACLAuditResult is the aggregate result of an ACL audit.
type ACLAuditResult struct {
	Total    int               `json:"total_acls"`
	Findings []ACLAuditFinding `json:"findings"`
	Summary  ACLAuditSummary   `json:"summary"`
}

// ACLAuditSummary counts findings by type.
type ACLAuditSummary struct {
	Shadows   int `json:"shadows"`
	Conflicts int `json:"conflicts"`
	Redundant int `json:"redundant"`
}

// ACLAuditor analyzes ACL rules for shadowing and conflicts.
type ACLAuditor struct {
	NB client.Client
}

// Audit runs the full ACL audit across all ACLs.
func (a *ACLAuditor) Audit(ctx context.Context) (*ACLAuditResult, error) {
	var acls []nb.ACL
	if err := a.NB.List(ctx, &acls); err != nil {
		return nil, fmt.Errorf("listing ACLs: %w", err)
	}

	var portGroups []nb.PortGroup
	if err := a.NB.List(ctx, &portGroups); err != nil {
		return nil, fmt.Errorf("listing port groups: %w", err)
	}

	aclContext := buildACLContextMap(portGroups)

	result := &ACLAuditResult{
		Total:    len(acls),
		Findings: []ACLAuditFinding{},
	}

	type groupKey struct {
		Direction string
		Tier      int
	}
	groups := make(map[groupKey][]nb.ACL)
	for _, acl := range acls {
		key := groupKey{Direction: acl.Direction, Tier: acl.Tier}
		groups[key] = append(groups[key], acl)
	}

	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			return group[i].Priority > group[j].Priority
		})

		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				findings := compareACLs(group[i], group[j], aclContext)
				result.Findings = append(result.Findings, findings...)
			}
		}
	}

	for _, f := range result.Findings {
		switch f.Type {
		case "shadow":
			result.Summary.Shadows++
		case "conflict":
			result.Summary.Conflicts++
		case "redundant":
			result.Summary.Redundant++
		}
	}

	return result, nil
}

func compareACLs(higher, lower nb.ACL, aclContext map[string]string) []ACLAuditFinding {
	var findings []ACLAuditFinding

	if higher.Direction != lower.Direction {
		return nil
	}

	relation := matchRelation(higher.Match, lower.Match)

	switch {
	case relation == matchEqual:
		if higher.Action == lower.Action {
			findings = append(findings, ACLAuditFinding{
				Type:            "redundant",
				Severity:        AuditSeverityWarning,
				Message:         fmt.Sprintf("ACL at priority %d has identical match and action as ACL at priority %d", lower.Priority, higher.Priority),
				ACLUUID:         lower.UUID,
				ACLPriority:     lower.Priority,
				ACLMatch:        lower.Match,
				ACLAction:       lower.Action,
				ACLDirection:    lower.Direction,
				RelatedUUID:     higher.UUID,
				RelatedPriority: higher.Priority,
				RelatedMatch:    higher.Match,
				RelatedAction:   higher.Action,
				Context:         aclContext[lower.UUID],
			})
		} else {
			findings = append(findings, ACLAuditFinding{
				Type:            "shadow",
				Severity:        AuditSeverityError,
				Message:         fmt.Sprintf("ACL at priority %d is completely shadowed by ACL at priority %d (same match, different action)", lower.Priority, higher.Priority),
				ACLUUID:         lower.UUID,
				ACLPriority:     lower.Priority,
				ACLMatch:        lower.Match,
				ACLAction:       lower.Action,
				ACLDirection:    lower.Direction,
				RelatedUUID:     higher.UUID,
				RelatedPriority: higher.Priority,
				RelatedMatch:    higher.Match,
				RelatedAction:   higher.Action,
				Context:         aclContext[lower.UUID],
			})
		}

	case relation == matchSuperset && higher.Priority > lower.Priority:
		if higher.Action != lower.Action {
			findings = append(findings, ACLAuditFinding{
				Type:            "shadow",
				Severity:        AuditSeverityError,
				Message:         fmt.Sprintf("ACL at priority %d is shadowed by broader ACL at priority %d", lower.Priority, higher.Priority),
				ACLUUID:         lower.UUID,
				ACLPriority:     lower.Priority,
				ACLMatch:        lower.Match,
				ACLAction:       lower.Action,
				ACLDirection:    lower.Direction,
				RelatedUUID:     higher.UUID,
				RelatedPriority: higher.Priority,
				RelatedMatch:    higher.Match,
				RelatedAction:   higher.Action,
				Context:         aclContext[lower.UUID],
			})
		}

	case relation == matchConflict:
		findings = append(findings, ACLAuditFinding{
			Type:            "conflict",
			Severity:        AuditSeverityWarning,
			Message:         fmt.Sprintf("ACLs at priorities %d and %d have overlapping but contradictory matches", higher.Priority, lower.Priority),
			ACLUUID:         lower.UUID,
			ACLPriority:     lower.Priority,
			ACLMatch:        lower.Match,
			ACLAction:       lower.Action,
			ACLDirection:    lower.Direction,
			RelatedUUID:     higher.UUID,
			RelatedPriority: higher.Priority,
			RelatedMatch:    higher.Match,
			RelatedAction:   higher.Action,
			Context:         aclContext[lower.UUID],
		})
	}

	return findings
}

type matchRelationType int

const (
	matchDisjoint matchRelationType = iota
	matchEqual
	matchSuperset
	matchSubset
	matchConflict
)

func matchRelation(a, b string) matchRelationType {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)

	if a == b {
		return matchEqual
	}

	if a == "1" {
		return matchSuperset
	}
	if b == "1" {
		return matchSubset
	}

	conjA := parseConjuncts(a)
	conjB := parseConjuncts(b)

	if isSubsetOfConjuncts(conjA, conjB) {
		return matchSuperset
	}
	if isSubsetOfConjuncts(conjB, conjA) {
		return matchSubset
	}

	commonCount := countCommonConjuncts(conjA, conjB)
	if commonCount > 0 && commonCount < len(conjA) && commonCount < len(conjB) {
		return matchConflict
	}

	return matchDisjoint
}

func parseConjuncts(match string) []string {
	parts := strings.Split(match, "&&")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	sort.Strings(result)
	return result
}

func isSubsetOfConjuncts(a, b []string) bool {
	if len(a) == 0 {
		return false
	}
	bSet := make(map[string]bool, len(b))
	for _, conj := range b {
		bSet[conj] = true
	}
	for _, conj := range a {
		if !bSet[conj] {
			return false
		}
	}
	return true
}

func countCommonConjuncts(a, b []string) int {
	bSet := make(map[string]bool, len(b))
	for _, conj := range b {
		bSet[conj] = true
	}
	count := 0
	for _, conj := range a {
		if bSet[conj] {
			count++
		}
	}
	return count
}

func buildACLContextMap(portGroups []nb.PortGroup) map[string]string {
	result := make(map[string]string)
	for _, pg := range portGroups {
		for _, aclUUID := range pg.ACLs {
			result[aclUUID] = pg.Name
		}
	}
	return result
}
