package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSBTestClient(t *testing.T) client.Client {
	t.Helper()
	return testutil.SetupSBTestClient(t)
}

func insertChassis(t *testing.T, c client.Client, name, hostname, ip string) string {
	t.Helper()

	namedEncapUUID := "encap_" + name
	encap := &sb.Encap{
		UUID:        namedEncapUUID,
		Type:        "geneve",
		IP:          ip,
		ChassisName: name,
	}
	encapOps, err := c.Create(encap)
	require.NoError(t, err)

	chassis := &sb.Chassis{
		Name:     name,
		Hostname: hostname,
		Encaps:   []string{namedEncapUUID},
	}
	chassisOps, err := c.Create(chassis)
	require.NoError(t, err)

	ops := append(encapOps, chassisOps...)
	reply, err := c.Transact(context.Background(), ops...)
	require.NoError(t, err)
	_, err = ovsdb.CheckOperationResults(reply, ops)
	require.NoError(t, err)

	uuid := reply[1].UUID.GoUUID

	require.Eventually(t, func() bool {
		ch := &sb.Chassis{UUID: uuid}
		return c.Get(context.Background(), ch) == nil
	}, 2*time.Second, 10*time.Millisecond)

	return uuid
}

// TestSB runs all SB handler tests under a single shared test server
// to avoid file descriptor exhaustion from multiple servers.
func TestSB(t *testing.T) {
	c := setupSBTestClient(t)

	t.Run("ListChassis", func(t *testing.T) {
		insertChassis(t, c, "chassis-1", "host-1", "192.168.1.1")
		insertChassis(t, c, "chassis-2", "host-2", "192.168.1.2")

		mux := http.NewServeMux()
		RegisterSB(mux, c)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/chassis", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Len(t, body, 2)
	})

	t.Run("GetChassis", func(t *testing.T) {
		// chassis-1 was inserted by previous subtest
		var chassisList []sb.Chassis
		err := c.List(context.Background(), &chassisList)
		require.NoError(t, err)
		require.NotEmpty(t, chassisList)

		uuid := chassisList[0].UUID

		mux := http.NewServeMux()
		RegisterSB(mux, c)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/v1/sb/chassis/%s", uuid), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.NotEmpty(t, body["name"])
	})

	t.Run("LogicalFlows_Empty", func(t *testing.T) {
		mux := http.NewServeMux()
		RegisterSB(mux, c)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/logical-flows", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Empty(t, body)
	})

	t.Run("LogicalFlows_InvalidTableID", func(t *testing.T) {
		mux := http.NewServeMux()
		RegisterSB(mux, c)

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/logical-flows?table_id=abc", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestHandleLogicalFlows_Filters drives the filtered branch of
// handleLogicalFlows against a small seeded flow set on the shared SB server.
func TestHandleLogicalFlows_Filters(t *testing.T) {
	sbc := testutil.SetupSBTestClient(t)

	dp := insertDatapath(t, sbc, map[string]string{"logical-switch": "sw-flt"})
	insertLogicalFlow(t, sbc, dp, "ingress", 0, 100, `inport == "a"`, "next;", nil)
	insertLogicalFlow(t, sbc, dp, "ingress", 3, 50, `ip4.dst == 10.0.0.1`, "next;", nil)
	insertLogicalFlow(t, sbc, dp, "egress", 0, 10, "1", "output;", nil)

	mux := http.NewServeMux()
	RegisterSB(mux, sbc)

	get := func(t *testing.T, url string) []map[string]any {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var body []map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body
	}

	t.Run("by pipeline and datapath", func(t *testing.T) {
		body := get(t, "/api/v1/sb/logical-flows?datapath="+dp+"&pipeline=ingress")
		assert.Len(t, body, 2)
	})

	t.Run("by table_id", func(t *testing.T) {
		body := get(t, "/api/v1/sb/logical-flows?datapath="+dp+"&table_id=3")
		assert.Len(t, body, 1)
	})

	t.Run("by match substring", func(t *testing.T) {
		body := get(t, "/api/v1/sb/logical-flows?match=ip4.dst")
		assert.Len(t, body, 1)
	})
}

// TestLogicalFlows_Pagination covers both branches of the logical-flows handler.
// This is the endpoint the cap exists for: unfiltered, it used to serialize the
// entire Logical_Flow cache — potentially millions of rows — into one response.
func TestLogicalFlows_Pagination(t *testing.T) {
	sbc := setupSBTestClient(t)
	dpUUID := insertDatapath(t, sbc, map[string]string{"logical-switch": "sw-page"})
	for i := range 8 {
		insertLogicalFlow(t, sbc, dpUUID, "ingress", i, 100, fmt.Sprintf("ip4.dst == 10.0.0.%d", i), "next;", nil)
	}

	mux := http.NewServeMux()
	RegisterSB(mux, sbc)

	list := func(t *testing.T, query string) (*httptest.ResponseRecorder, []map[string]any) {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
			"/api/v1/sb/logical-flows"+query, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var got []map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		return rec, got
	}

	t.Run("unfiltered branch is paged", func(t *testing.T) {
		rec, got := list(t, "?limit=3")
		assert.Len(t, got, 3)
		assert.Equal(t, "8", rec.Header().Get("X-Total-Count"))
		assert.Equal(t, "true", rec.Header().Get("X-Truncated"))
	})

	t.Run("filtered branch is paged", func(t *testing.T) {
		rec, got := list(t, "?pipeline=ingress&limit=3")
		assert.Len(t, got, 3)
		assert.Equal(t, "8", rec.Header().Get("X-Total-Count"))
		assert.Equal(t, "true", rec.Header().Get("X-Truncated"))
	})

	t.Run("filtered branch rejects a bad limit", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
			"/api/v1/sb/logical-flows?pipeline=ingress&limit=-5", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
