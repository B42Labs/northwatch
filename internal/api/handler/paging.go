package handler

import (
	"cmp"
	"net/http"
	"slices"
	"strconv"

	"github.com/b42labs/northwatch/internal/api"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
)

// maxListPageSize is the hard cap on rows in one list response, and the default
// when no limit is given. Without it, GET /api/v1/sb/logical-flows serializes
// the entire cache — potentially millions of rows — into a single response,
// which any unauthenticated client can request repeatedly.
const maxListPageSize = 5000

// writePagedModels writes a window of results as a JSON array, honouring the
// limit and offset query parameters and capping the page at maxListPageSize.
//
// The body stays a bare array (rather than gaining a pagination envelope) so
// existing clients keep working; the window is described in headers instead:
// X-Total-Count carries the unpaginated total, and X-Truncated is set when the
// response does not contain every remaining row.
//
// The emitted page is ordered by UUID: the cache iterates a map, so without an
// order an offset would step through an arbitrary permutation and could show a
// row twice while never showing another.
func writePagedModels[T any](w http.ResponseWriter, r *http.Request, results []T) {
	limit, ok := pageParam(w, r, "limit", maxListPageSize, maxListPageSize)
	if !ok {
		return
	}
	offset, ok := pageParam(w, r, "offset", 0, 0)
	if !ok {
		return
	}

	total := len(results)

	// Sort by UUID without copying rows. getUUID takes any, so passing a row by
	// value boxes a heap copy of the whole struct on every comparison; on the
	// largest tables that is millions of allocations to return one page. Read
	// each UUID once through a pointer, sort an index, then build only the page.
	type keyed struct {
		uuid string
		i    int
	}
	keys := make([]keyed, total)
	for j := range results {
		keys[j] = keyed{getUUID(&results[j]), j}
	}
	slices.SortFunc(keys, func(a, b keyed) int { return cmp.Compare(a.uuid, b.uuid) })

	start := min(offset, total)
	end := min(start+limit, total)
	page := make([]T, 0, end-start)
	for _, k := range keys[start:end] {
		page = append(page, results[k.i])
	}

	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	if end < total {
		w.Header().Set("X-Truncated", "true")
	}
	api.WriteJSON(w, http.StatusOK, ovndb.ModelsToMaps(page))
}

// pageParam parses a non-negative integer query parameter. An absent parameter
// yields def; a value above max (when max > 0) is clamped rather than rejected,
// so a client asking for everything gets the capped page instead of an error.
// It reports whether parsing succeeded; on failure it has written a 400.
func pageParam(w http.ResponseWriter, r *http.Request, name string, def, max int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, true
	}

	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		api.WriteError(w, http.StatusBadRequest, "invalid "+name+" parameter: want a non-negative integer")
		return 0, false
	}
	if max > 0 && v > max {
		return max, true
	}
	return v, true
}
