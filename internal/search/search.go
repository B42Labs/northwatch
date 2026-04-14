package search

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strings"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// QueryType identifies the inferred kind of a search query string.
type QueryType string

// Recognized query types returned by ClassifyQuery.
const (
	QueryIPv4     QueryType = "ipv4"
	QueryIPv6     QueryType = "ipv6"
	QueryMAC      QueryType = "mac"
	QueryUUID     QueryType = "uuid"
	QueryFreeText QueryType = "text"
)

// Result is the per-table search result returned to API clients.
type Result struct {
	Database string           `json:"database"`
	Table    string           `json:"table"`
	Matches  []map[string]any `json:"matches"`
}

// TableDef registers a single OVSDB table for searching.
type TableDef struct {
	Name      string
	ListFunc  func(ctx context.Context) (any, error)
	ModelType reflect.Type
}

// Engine performs cross-database searches over a fixed set of tables.
type Engine struct {
	nbTables []TableDef
	sbTables []TableDef
}

var uuidRegexp = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var macRegexp = regexp.MustCompile(`^([0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}$`)

// ClassifyQuery infers the kind of a query string (IP, MAC, UUID, text).
func ClassifyQuery(q string) QueryType {
	q = strings.TrimSpace(q)
	if uuidRegexp.MatchString(q) {
		return QueryUUID
	}
	if macRegexp.MatchString(q) {
		return QueryMAC
	}
	if ip := net.ParseIP(q); ip != nil {
		if ip.To4() != nil {
			return QueryIPv4
		}
		return QueryIPv6
	}
	// Check CIDR notation
	if ip, _, err := net.ParseCIDR(q); err == nil {
		if ip.To4() != nil {
			return QueryIPv4
		}
		return QueryIPv6
	}
	return QueryFreeText
}

// RegisterTable creates a TableDef for use with the search engine.
func RegisterTable[T any](name string, c client.Client) TableDef {
	var zero T
	return TableDef{
		Name:      name,
		ModelType: reflect.TypeOf(zero),
		ListFunc: func(ctx context.Context) (any, error) {
			var results []T
			if err := c.List(ctx, &results); err != nil {
				return nil, err
			}
			return results, nil
		},
	}
}

// NewEngine creates a search Engine over the provided NB and SB tables.
func NewEngine(nbTables, sbTables []TableDef) *Engine {
	return &Engine{nbTables: nbTables, sbTables: sbTables}
}

// Search executes a query against all registered tables and returns the
// per-table matches. The query is classified internally and matched as a
// case-insensitive substring against every string-like field.
func (e *Engine) Search(ctx context.Context, query string) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}

	queryLower := strings.ToLower(query)
	var results []Result

	for _, td := range e.nbTables {
		matches, err := searchTable(ctx, td, queryLower)
		if err != nil {
			return nil, fmt.Errorf("searching NB %s: %w", td.Name, err)
		}
		if len(matches) > 0 {
			results = append(results, Result{
				Database: "nb",
				Table:    td.Name,
				Matches:  matches,
			})
		}
	}

	for _, td := range e.sbTables {
		matches, err := searchTable(ctx, td, queryLower)
		if err != nil {
			return nil, fmt.Errorf("searching SB %s: %w", td.Name, err)
		}
		if len(matches) > 0 {
			results = append(results, Result{
				Database: "sb",
				Table:    td.Name,
				Matches:  matches,
			})
		}
	}

	return results, nil
}

func searchTable(ctx context.Context, td TableDef, queryLower string) ([]map[string]any, error) {
	data, err := td.ListFunc(ctx)
	if err != nil {
		return nil, err
	}

	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	var matches []map[string]any
	for i := 0; i < v.Len(); i++ {
		row := v.Index(i)
		if matchesQuery(row, queryLower) {
			matches = append(matches, api.ModelToMap(row.Interface()))
		}
	}
	return matches, nil
}

func matchesQuery(v reflect.Value, queryLower string) bool {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := v.Field(i)
		if matchFieldValue(field, queryLower) {
			return true
		}
	}
	return false
}

func matchFieldValue(v reflect.Value, queryLower string) bool {
	switch v.Kind() {
	case reflect.String:
		return strings.Contains(strings.ToLower(v.String()), queryLower)
	case reflect.Ptr:
		if !v.IsNil() {
			return matchFieldValue(v.Elem(), queryLower)
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if matchFieldValue(v.Index(i), queryLower) {
				return true
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if matchFieldValue(key, queryLower) {
				return true
			}
			if matchFieldValue(v.MapIndex(key), queryLower) {
				return true
			}
		}
	}
	return false
}

