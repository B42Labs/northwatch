// Package snapshot captures a point-in-time copy of the OVN Northbound and
// Southbound databases to a local file and replays it through libovsdb's
// in-memory OVSDB server. This lets Northwatch run entirely offline against a
// captured snapshot — no live OVN connection required.
//
// The capture is OVSDB-native: each row is stored as its generated model and,
// on restore, re-inserted with its original UUID preserved. Because UUIDs are
// kept intact, every cross-table reference (datapaths, port bindings, NAT,
// HA groups, …) stays valid without any remapping.
package snapshot

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/google/uuid"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/database"
	"github.com/ovn-kubernetes/libovsdb/database/inmemory"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
	"github.com/ovn-kubernetes/libovsdb/server"
)

// Version is the on-disk snapshot format version.
const Version = 1

// File is the serialized snapshot: the full contents of both OVN databases.
type File struct {
	Version   int     `json:"version"`
	CreatedAt string  `json:"created_at,omitempty"`
	Source    *Source `json:"source,omitempty"`
	NB        Dump    `json:"nb"`
	SB        Dump    `json:"sb"`
}

// Source records where a snapshot was captured from (informational only).
type Source struct {
	NBAddr string `json:"nb_addr,omitempty"`
	SBAddr string `json:"sb_addr,omitempty"`
}

// Dump holds every row of a single OVSDB database, keyed by table name.
type Dump struct {
	Schema string              `json:"schema"`
	Tables map[string][]Record `json:"tables"`
}

// Record is one row: its original UUID and the JSON-encoded generated model.
type Record struct {
	UUID  string          `json:"uuid"`
	Model json.RawMessage `json:"model"`
}

// RowCount returns the total number of rows across all tables in the dump.
func (d Dump) RowCount() int {
	n := 0
	for _, recs := range d.Tables {
		n += len(recs)
	}
	return n
}

// Capture reads the full contents of both database caches into a File.
// The clients must already be connected and monitoring (MonitorAll).
func Capture(nbClient, sbClient client.Client, nbSchemaName, sbSchemaName string) (*File, error) {
	nbDump, err := captureDump(nbClient, nbSchemaName)
	if err != nil {
		return nil, fmt.Errorf("capturing NB: %w", err)
	}
	sbDump, err := captureDump(sbClient, sbSchemaName)
	if err != nil {
		return nil, fmt.Errorf("capturing SB: %w", err)
	}
	return &File{
		Version:   Version,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		NB:        nbDump,
		SB:        sbDump,
	}, nil
}

// captureDump serializes every cached row of a single database.
func captureDump(c client.Client, schemaName string) (Dump, error) {
	cache := c.Cache()
	if cache == nil {
		return Dump{}, fmt.Errorf("client has no populated cache")
	}
	dump := Dump{Schema: schemaName, Tables: map[string][]Record{}}
	for _, table := range cache.Tables() {
		rc := cache.Table(table)
		if rc == nil {
			continue
		}
		rows := rc.Rows()
		if len(rows) == 0 {
			continue
		}
		uuids := make([]string, 0, len(rows))
		for u := range rows {
			uuids = append(uuids, u)
		}
		sort.Strings(uuids) // deterministic output
		recs := make([]Record, 0, len(rows))
		for _, u := range uuids {
			data, err := json.Marshal(rows[u])
			if err != nil {
				return Dump{}, fmt.Errorf("marshal %s/%s: %w", table, u, err)
			}
			recs = append(recs, Record{UUID: u, Model: data})
		}
		dump.Tables[table] = recs
	}
	return dump, nil
}

// Save writes the snapshot as JSON to w.
func Save(f *File, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(f)
}

// Load reads and validates a snapshot from r.
func Load(r io.Reader) (*File, error) {
	var f File
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return nil, fmt.Errorf("decoding snapshot: %w", err)
	}
	if f.Version != Version {
		return nil, fmt.Errorf("unsupported snapshot version %d (expected %d)", f.Version, Version)
	}
	return &f, nil
}

// Servers holds the in-memory OVSDB servers backing a loaded snapshot along
// with the local unix socket addresses to connect to them.
type Servers struct {
	NBAddr string
	SBAddr string

	dir     string
	servers []*server.OvsdbServer
}

// Close shuts down the in-memory servers and removes their socket directory.
func (s *Servers) Close() {
	for _, srv := range s.servers {
		if srv != nil {
			srv.Close()
		}
	}
	if s.dir != "" {
		_ = os.RemoveAll(s.dir)
	}
}

// Serve loads the snapshot into two in-memory OVSDB servers (one per database)
// listening on unix sockets, and returns their addresses. Connect a normal
// libovsdb client to Servers.NBAddr / Servers.SBAddr to work on the offline copy.
func Serve(f *File, nbClientModel, sbClientModel model.ClientDBModel, nbSchema, sbSchema ovsdb.DatabaseSchema) (*Servers, error) {
	logger := stdr.New(nil)

	dir, err := os.MkdirTemp("", "northwatch-snapshot-*")
	if err != nil {
		return nil, fmt.Errorf("creating socket directory: %w", err)
	}
	s := &Servers{dir: dir}

	nbAddr, nbSrv, err := serveDump(f.NB, nbClientModel, nbSchema, filepath.Join(dir, "nb.sock"), &logger)
	if err != nil {
		s.Close()
		return nil, err
	}
	s.servers = append(s.servers, nbSrv)
	s.NBAddr = nbAddr

	sbAddr, sbSrv, err := serveDump(f.SB, sbClientModel, sbSchema, filepath.Join(dir, "sb.sock"), &logger)
	if err != nil {
		s.Close()
		return nil, err
	}
	s.servers = append(s.servers, sbSrv)
	s.SBAddr = sbAddr

	return s, nil
}

// serveDump builds an in-memory server for one database, loads the dump into it,
// and starts serving on the given unix socket path.
func serveDump(d Dump, clientModel model.ClientDBModel, schema ovsdb.DatabaseSchema, sockPath string, logger *logr.Logger) (string, *server.OvsdbServer, error) {
	if d.Schema != "" && d.Schema != schema.Name {
		return "", nil, fmt.Errorf("snapshot schema %q does not match expected %q", d.Schema, schema.Name)
	}

	dbModel, errs := model.NewDatabaseModel(schema, clientModel)
	if len(errs) > 0 {
		return "", nil, fmt.Errorf("building %s database model: %v", schema.Name, errs)
	}

	db := inmemory.NewDatabase(map[string]model.ClientDBModel{schema.Name: clientModel}, logger)
	// NewOvsdbServer calls CreateDatabase for the provided model, so the
	// database exists before we load rows into it.
	srv, err := server.NewOvsdbServer(db, logger, dbModel)
	if err != nil {
		return "", nil, fmt.Errorf("creating %s server: %w", schema.Name, err)
	}

	if err := loadDump(db, dbModel, schema, d); err != nil {
		srv.Close()
		return "", nil, err
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve("unix", sockPath) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-errCh:
			srv.Close()
			return "", nil, fmt.Errorf("serving %s: %w", schema.Name, err)
		default:
		}
		if srv.Ready() {
			return "unix:" + sockPath, srv, nil
		}
		if time.Now().After(deadline) {
			srv.Close()
			return "", nil, fmt.Errorf("timed out waiting for %s server to become ready", schema.Name)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// loadDump inserts every row of a dump into the in-memory database in a single
// transaction, preserving original UUIDs so cross-table references stay valid.
//
// snapshot.Capture reads tables at slightly different instants, so a capture
// from a churning production database can contain a reference to a row that was
// deleted (or not yet created) between reads. Rather than hard-failing the whole
// load on such a dangling reference, prune each reference that points outside the
// set of UUIDs actually present in the dump.
func loadDump(db database.Database, dbModel model.DatabaseModel, schema ovsdb.DatabaseSchema, d Dump) error {
	dbName := schema.Name

	tables := make([]string, 0, len(d.Tables))
	for t := range d.Tables {
		tables = append(tables, t)
	}
	sort.Strings(tables) // deterministic operation order

	// Collect every UUID present in this dump so dangling references can be pruned.
	present := make(map[string]struct{})
	for _, recs := range d.Tables {
		for _, rec := range recs {
			present[rec.UUID] = struct{}{}
		}
	}

	var ops []ovsdb.Operation
	for _, table := range tables {
		for _, rec := range d.Tables[table] {
			m, err := dbModel.NewModel(table)
			if err != nil {
				return fmt.Errorf("%s: unknown table %q: %w", dbName, table, err)
			}
			if err := json.Unmarshal(rec.Model, m); err != nil {
				return fmt.Errorf("%s/%s %s: unmarshal model: %w", dbName, table, rec.UUID, err)
			}
			info, err := dbModel.NewModelInfo(m)
			if err != nil {
				return fmt.Errorf("%s/%s: model info: %w", dbName, table, err)
			}
			row, err := dbModel.Mapper.NewRow(info)
			if err != nil {
				return fmt.Errorf("%s/%s: build row: %w", dbName, table, err)
			}
			// UUID and version are carried by the operation / managed by the
			// server, not as regular columns.
			delete(row, "_uuid")
			delete(row, "_version")
			row = pruneDanglingRowRefs(schema, table, row, present)
			ops = append(ops, ovsdb.Operation{
				Op:    ovsdb.OperationInsert,
				Table: table,
				UUID:  rec.UUID,
				Row:   row,
			})
		}
	}

	if len(ops) == 0 {
		return nil
	}

	txn := db.NewTransaction(dbName)
	results, update := txn.Transact(ops...)
	for i, r := range results {
		if r != nil && r.Error != "" {
			return fmt.Errorf("%s: loading row %d failed: %s: %s", dbName, i, r.Error, r.Details)
		}
	}
	if err := db.Commit(dbName, uuid.UUID{}, update); err != nil {
		return fmt.Errorf("%s: committing snapshot: %w", dbName, err)
	}
	return nil
}
