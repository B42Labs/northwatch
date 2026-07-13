package search

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strings"

	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
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
	Name     string
	ListFunc func(ctx context.Context) (any, error)
}

// DatabaseTables groups the searchable tables of a single database under its
// API-facing database name (e.g. "nb", "sb", "ovs"). The name is emitted
// verbatim in each Result so the engine is not hardwired to any fixed set of
// databases.
type DatabaseTables struct {
	Name   string
	Tables []TableDef
}

// Engine performs cross-database searches over a fixed set of databases, each
// with its own set of tables.
type Engine struct {
	dbs []DatabaseTables
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
	return TableDef{
		Name: name,
		ListFunc: func(ctx context.Context) (any, error) {
			var results []T
			if err := c.List(ctx, &results); err != nil {
				return nil, err
			}
			return results, nil
		},
	}
}

// NewEngine creates a search Engine over the provided databases. Results are
// emitted in the order the databases are passed.
func NewEngine(dbs []DatabaseTables) *Engine {
	return &Engine{dbs: dbs}
}

// Match caps bounding a single search. A one-character query substring-matches
// nearly every row of every registered table, so an uncapped search materializes
// the whole of both databases into one response — the cheapest way for an
// unauthenticated client to exhaust memory.
const (
	maxMatchesPerTable = 100
	maxTotalMatches    = 500
)

// Search executes a query against all registered tables and returns the
// per-table matches. The query is classified internally and matched as a
// case-insensitive substring against every string-like field.
//
// Matches are capped per table and in total; the second return value reports
// whether the caps dropped anything, so a caller can tell a complete result set
// from a partial one and tell the user to narrow the query.
func (e *Engine) Search(ctx context.Context, query string) ([]Result, bool, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, false, fmt.Errorf("empty query")
	}

	queryLower := strings.ToLower(query)
	var results []Result
	total := 0
	truncated := false

	for _, db := range e.dbs {
		for _, td := range db.Tables {
			if total >= maxTotalMatches {
				return results, true, nil
			}

			remaining := min(maxMatchesPerTable, maxTotalMatches-total)
			matches, more, err := searchTable(ctx, td, queryLower, remaining)
			if err != nil {
				return nil, false, fmt.Errorf("searching %s %s: %w", db.Name, td.Name, err)
			}
			truncated = truncated || more

			if len(matches) > 0 {
				total += len(matches)
				results = append(results, Result{
					Database: db.Name,
					Table:    td.Name,
					Matches:  matches,
				})
			}
		}
	}

	return results, truncated, nil
}

// searchTable collects at most limit matches from one table. It reports whether
// it stopped early, i.e. whether the table holds further matches beyond limit.
func searchTable(ctx context.Context, td TableDef, queryLower string, limit int) ([]map[string]any, bool, error) {
	data, err := td.ListFunc(ctx)
	if err != nil {
		return nil, false, err
	}

	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	var matches []map[string]any
	for i := 0; i < v.Len(); i++ {
		row := v.Index(i)
		if !matchesQuery(row, queryLower) {
			continue
		}
		if len(matches) >= limit {
			return matches, true, nil
		}
		matches = append(matches, ovndb.ModelToMap(row.Interface()))
	}
	return matches, false, nil
}

func matchesQuery(v reflect.Value, queryLower string) bool {
	if v.Kind() == reflect.Pointer {
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
	case reflect.Pointer:
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
