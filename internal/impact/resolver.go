package impact

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// Sentinel errors for distinguishing client-side (400) from not-found (404) failures.
var (
	ErrUnsupportedDB = errors.New("impact analysis is only supported for NB entities")
	ErrUnknownTable  = errors.New("unknown table")
)

const (
	defaultMaxDepth    = 5
	defaultMaxEntities = 1000
)

// Resolver computes impact trees for OVN entities by walking their reference graph.
type Resolver struct {
	nbClient    client.Client
	sbClient    client.Client // may be nil
	maxDepth    int
	maxEntities int
}

// NewResolver creates a new impact Resolver.
// sbClient is optional; if nil, SB correlations are skipped.
func NewResolver(nbClient, sbClient client.Client) *Resolver {
	return &Resolver{
		nbClient:    nbClient,
		sbClient:    sbClient,
		maxDepth:    defaultMaxDepth,
		maxEntities: defaultMaxEntities,
	}
}

// SetLimits overrides the default traversal limits.
// maxDepth controls recursion depth (default 5); maxEntities caps total visited nodes (default 1000).
func (r *Resolver) SetLimits(maxDepth, maxEntities int) {
	if maxDepth > 0 {
		r.maxDepth = maxDepth
	}
	if maxEntities > 0 {
		r.maxEntities = maxEntities
	}
}

// Resolve computes the full impact tree for the entity identified by db/table/uuid.
func (r *Resolver) Resolve(ctx context.Context, db, table, uuid string) (*ImpactResult, error) {
	if db != "nb" {
		return nil, ErrUnsupportedDB
	}

	if _, ok := NBModelTypes[table]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTable, table)
	}

	fields, err := r.readEntity(ctx, db, table, uuid)
	if err != nil {
		return nil, fmt.Errorf("entity not found: %w", err)
	}

	root := ImpactNode{
		Database: db,
		Table:    table,
		UUID:     uuid,
		Name:     extractName(fields),
		RefType:  RefRoot,
	}

	visited := map[string]bool{uuid: true}
	truncated := false

	r.resolveForward(ctx, &root, table, fields, 0, visited, &truncated)
	r.resolveReverse(ctx, &root, table, uuid, visited, &truncated)
	r.resolveSBCorrelation(ctx, &root, table, uuid, visited, &truncated)

	summary := computeSummary(&root, truncated)

	return &ImpactResult{Root: root, Summary: summary}, nil
}

// resolveForward walks forward (strong/weak) references from the entity.
func (r *Resolver) resolveForward(ctx context.Context, node *ImpactNode, table string, fields map[string]any, depth int, visited map[string]bool, truncated *bool) {
	if depth >= r.maxDepth {
		*truncated = true
		return
	}

	edges, ok := NBRefs[table]
	if !ok {
		return
	}

	for _, edge := range edges {
		uuids := extractUUIDs(fields, edge.Column)
		for _, childUUID := range uuids {
			if visited[childUUID] || len(visited) >= r.maxEntities {
				if len(visited) >= r.maxEntities {
					*truncated = true
				}
				continue
			}
			visited[childUUID] = true

			childFields, err := r.readEntity(ctx, "nb", edge.TargetTable, childUUID)
			if err != nil {
				continue // entity may have been deleted between reads
			}

			child := ImpactNode{
				Database: "nb",
				Table:    edge.TargetTable,
				UUID:     childUUID,
				Name:     extractName(childFields),
				RefType:  edge.Type,
				Column:   edge.Column,
			}

			r.resolveForward(ctx, &child, edge.TargetTable, childFields, depth+1, visited, truncated)
			node.Children = append(node.Children, child)
		}
	}
}

// resolveReverse finds entities that reference the target and would lose their reference.
func (r *Resolver) resolveReverse(ctx context.Context, node *ImpactNode, table, uuid string, visited map[string]bool, truncated *bool) {
	reverseEdges, ok := ReverseRefs[table]
	if !ok {
		return
	}

	for _, edge := range reverseEdges {
		matches, err := r.listEntitiesReferencing(ctx, "nb", edge.SourceTable, edge.Column, uuid)
		if err != nil {
			continue
		}
		for _, m := range matches {
			mUUID, _ := m["_uuid"].(string)
			if mUUID == "" || visited[mUUID] {
				continue
			}
			if len(visited) >= r.maxEntities {
				*truncated = true
				return
			}
			visited[mUUID] = true

			child := ImpactNode{
				Database: "nb",
				Table:    edge.SourceTable,
				UUID:     mUUID,
				Name:     extractName(m),
				RefType:  RefReverse,
				Column:   edge.Column,
			}
			// Don't recurse into reverse refs to avoid cycles and unbounded growth.
			node.Children = append(node.Children, child)
		}
	}
}

// resolveSBCorrelation finds SB Datapath_Binding and Port_Binding entities that
// realize this NB entity.
func (r *Resolver) resolveSBCorrelation(ctx context.Context, node *ImpactNode, table, uuid string, visited map[string]bool, truncated *bool) {
	if r.sbClient == nil {
		return
	}
	corr, ok := SBCorrelations[table]
	if !ok {
		return
	}

	// Find Datapath_Binding(s) whose external_ids[corr.DatapathKey] == uuid
	dpKey := corr.DatapathKey
	var datapaths []sb.DatapathBinding
	if err := r.sbClient.WhereCache(func(dp *sb.DatapathBinding) bool {
		return dp.ExternalIDs[dpKey] == uuid
	}).List(ctx, &datapaths); err != nil {
		return
	}

	for _, dp := range datapaths {
		if len(visited) >= r.maxEntities {
			*truncated = true
			return
		}
		visited[dp.UUID] = true

		dpNode := ImpactNode{
			Database: "sb",
			Table:    "Datapath_Binding",
			UUID:     dp.UUID,
			RefType:  RefCorrelation,
		}

		// Find Port_Bindings on this datapath
		dpUUID := dp.UUID
		var portBindings []sb.PortBinding
		if err := r.sbClient.WhereCache(func(pb *sb.PortBinding) bool {
			return pb.Datapath == dpUUID
		}).List(ctx, &portBindings); err == nil {
			for _, pb := range portBindings {
				if len(visited) >= r.maxEntities {
					*truncated = true
					break
				}
				visited[pb.UUID] = true

				pbNode := ImpactNode{
					Database: "sb",
					Table:    "Port_Binding",
					UUID:     pb.UUID,
					Name:     pb.LogicalPort,
					RefType:  RefCorrelation,
				}
				dpNode.Children = append(dpNode.Children, pbNode)
			}
		}

		node.Children = append(node.Children, dpNode)
	}
}

// readEntity fetches a single entity by UUID and returns it as a map.
func (r *Resolver) readEntity(ctx context.Context, db, table, uuid string) (map[string]any, error) {
	modelType, c, err := r.modelAndClient(db, table)
	if err != nil {
		return nil, err
	}

	modelPtr := reflect.New(modelType)
	model := modelPtr.Elem()

	// Set the UUID field.
	for i := 0; i < modelType.NumField(); i++ {
		tag := modelType.Field(i).Tag.Get("ovsdb")
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		if tag == "_uuid" {
			model.Field(i).SetString(uuid)
			break
		}
	}

	if err := c.Get(ctx, modelPtr.Interface()); err != nil {
		return nil, fmt.Errorf("row not found: %w", err)
	}

	return api.ModelToMap(modelPtr.Interface()), nil
}

// listEntitiesReferencing lists all entities of sourceTable where column contains targetUUID.
func (r *Resolver) listEntitiesReferencing(ctx context.Context, db, sourceTable, column, targetUUID string) ([]map[string]any, error) {
	modelType, c, err := r.modelAndClient(db, sourceTable)
	if err != nil {
		return nil, err
	}

	// Create *[]T and call List.
	sliceType := reflect.SliceOf(modelType)
	slicePtr := reflect.New(sliceType)
	if err := c.List(ctx, slicePtr.Interface()); err != nil {
		return nil, err
	}

	slice := slicePtr.Elem()
	var results []map[string]any
	for i := 0; i < slice.Len(); i++ {
		m := api.ModelToMap(slice.Index(i).Interface())
		if containsUUID(m, column, targetUUID) {
			results = append(results, m)
		}
	}
	return results, nil
}

// modelAndClient returns the reflect.Type and client for a given db/table pair.
func (r *Resolver) modelAndClient(db, table string) (reflect.Type, client.Client, error) {
	if db == "nb" {
		t, ok := NBModelTypes[table]
		if !ok {
			return nil, nil, fmt.Errorf("unknown NB table %q", table)
		}
		return t, r.nbClient, nil
	}
	if db == "sb" {
		t, ok := SBModelTypes[table]
		if !ok {
			return nil, nil, fmt.Errorf("unknown SB table %q", table)
		}
		if r.sbClient == nil {
			return nil, nil, fmt.Errorf("SB client not available")
		}
		return t, r.sbClient, nil
	}
	return nil, nil, fmt.Errorf("unknown database %q", db)
}

// extractUUIDs extracts UUID strings from a column value, handling []string, *string, and string.
func extractUUIDs(fields map[string]any, column string) []string {
	val, ok := fields[column]
	if !ok || val == nil {
		return nil
	}

	switch v := val.(type) {
	case []string:
		return v
	case *string:
		if v != nil && *v != "" {
			return []string{*v}
		}
	case string:
		if v != "" {
			return []string{v}
		}
	}
	return nil
}

// containsUUID checks whether fields[column] contains the given UUID.
func containsUUID(fields map[string]any, column, uuid string) bool {
	val, ok := fields[column]
	if !ok || val == nil {
		return false
	}

	switch v := val.(type) {
	case []string:
		for _, s := range v {
			if s == uuid {
				return true
			}
		}
	case *string:
		return v != nil && *v == uuid
	case string:
		return v == uuid
	}
	return false
}

// extractName tries to get a human-readable name from an entity's fields.
func extractName(fields map[string]any) string {
	if name, ok := fields["name"]; ok {
		switch v := name.(type) {
		case string:
			return v
		case *string:
			if v != nil {
				return *v
			}
		}
	}
	return ""
}

// computeSummary walks the tree and produces aggregate counts.
func computeSummary(root *ImpactNode, truncated bool) ImpactSummary {
	s := ImpactSummary{
		ByTable:   make(map[string]int),
		ByRefType: make(map[string]int),
		Truncated: truncated,
	}
	walkSummary(root, 0, &s)
	return s
}

func walkSummary(node *ImpactNode, depth int, s *ImpactSummary) {
	if depth > s.MaxDepth {
		s.MaxDepth = depth
	}
	// Don't count the root in totals.
	if node.RefType != RefRoot {
		s.TotalAffected++
		s.ByTable[node.Table]++
		s.ByRefType[string(node.RefType)]++
	}
	for i := range node.Children {
		walkSummary(&node.Children[i], depth+1, s)
	}
}
