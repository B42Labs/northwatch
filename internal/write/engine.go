package write

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/b42labs/northwatch/internal/history"
	"github.com/b42labs/northwatch/internal/impact"
	ovndb "github.com/b42labs/northwatch/internal/ovsdb"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
)

// Engine orchestrates safe writes to the OVN Northbound database.
type Engine struct {
	nbClient    client.Client
	sbClient    client.Client // optional, used for SB-aware active chassis detection
	registry    *Registry
	collector   *history.Collector
	auditStore  *AuditStore
	plans       *PlanCache
	planTTL     time.Duration
	secret      []byte
	mu          sync.Mutex
	rateLimiter *rateLimiter
	resolver    *impact.Resolver // optional, enables impact analysis on delete operations
	// nbReady reports whether the NB cache is currently authoritative (the
	// client is connected and its monitors are not suspended). Cache-based
	// reference existence checks are only trustworthy when it returns true; when
	// it returns false — a reconnect in progress, or a snapshot session that has
	// purged the live cache — those checks are skipped and OVSDB's server-side
	// referential integrity is relied on instead. A nil predicate (the default,
	// used in tests) treats the cache as always authoritative.
	nbReady func() bool
}

// NewEngine creates a new write Engine with the given rate limit (operations per minute, 0 = unlimited).
// After construction, callers should invoke Start to begin background plan cache cleanup
// and call the returned stop function during shutdown.
// sbClient is optional (may be nil) and enables SB-aware active chassis detection for failover/evacuate.
// Returns an error only if crypto/rand fails to seed the apply-token secret.
func NewEngine(nbClient, sbClient client.Client, registry *Registry, collector *history.Collector, auditStore *AuditStore, planTTL time.Duration, rateLimit int) (*Engine, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("seeding write engine secret: %w", err)
	}
	e := &Engine{
		nbClient:   nbClient,
		sbClient:   sbClient,
		registry:   registry,
		collector:  collector,
		auditStore: auditStore,
		plans:      NewPlanCache(planTTL),
		planTTL:    planTTL,
		secret:     secret,
	}
	if rateLimit > 0 {
		e.rateLimiter = newRateLimiter(rateLimit)
	}
	return e, nil
}

// Start launches the background plan cache cleanup goroutine.
// The returned function stops the goroutine; call it during shutdown.
func (e *Engine) Start(ctx context.Context) func() {
	cleanupCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.plans.StartCleanup(cleanupCtx, e.planTTL)
	}()
	return func() {
		cancel()
		<-done
	}
}

// SetResolver sets the impact resolver for computing dependency analysis on delete operations.
func (e *Engine) SetResolver(r *impact.Resolver) {
	e.resolver = r
}

// SetReadyFunc sets the predicate that reports whether the NB cache is currently
// authoritative. Call it during setup (before serving) with the live cluster's
// readiness check so cache-based reference validation is skipped while the cache
// is non-authoritative (reconnecting or a snapshot session has suspended the
// live monitors). See the nbReady field for the rationale.
func (e *Engine) SetReadyFunc(f func() bool) {
	e.nbReady = f
}

// Schema returns the schema for all writable tables.
func (e *Engine) Schema() []TableSchema {
	return e.registry.Schema()
}

// Preview validates operations, reads current state, computes diffs,
// takes a snapshot, generates an HMAC token, and stores the plan in cache.
func (e *Engine) Preview(ctx context.Context, ops []WriteOperation) (*Plan, error) {
	if e.rateLimiter != nil && !e.rateLimiter.allow() {
		return nil, ErrRateLimited
	}
	if err := e.validateOps(ctx, ops); err != nil {
		return nil, err
	}

	diffs, err := e.computeDiffs(ctx, ops)
	if err != nil {
		return nil, fmt.Errorf("computing diffs: %w", err)
	}

	snap, err := e.collector.TakeSnapshot(ctx, "write-preview", "")
	if err != nil {
		return nil, fmt.Errorf("taking snapshot: %w", err)
	}

	planID := generateID()
	token := e.generateToken(planID, snap.ID)

	plan := &Plan{
		ID:         planID,
		CreatedAt:  time.Now().UTC(),
		Operations: ops,
		Diffs:      diffs,
		SnapshotID: snap.ID,
		Status:     "pending",
		ApplyToken: token,
		Impact:     e.computeImpact(ctx, ops),
	}

	e.plans.Store(plan)
	return plan, nil
}

// Apply verifies the token, locks, takes a fresh snapshot, builds libovsdb
// operations, transacts, and records an audit entry.
func (e *Engine) Apply(ctx context.Context, planID, token, actor string) (*AuditEntry, error) {
	if e.rateLimiter != nil && !e.rateLimiter.allow() {
		return nil, ErrRateLimited
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Retrieve and remove plan from cache under the lock to prevent races.
	plan, ok := e.plans.Get(planID)
	if !ok {
		return nil, fmt.Errorf("plan %q not found or expired", planID)
	}
	e.plans.Delete(planID)

	expectedToken := e.generateToken(plan.ID, plan.SnapshotID)
	if !hmac.Equal([]byte(token), []byte(expectedToken)) {
		return nil, fmt.Errorf("invalid apply token")
	}

	// Re-validate against the live database: the plan may have been previewed
	// up to the TTL ago and referenced rows could have changed since.
	if err := e.validateOps(ctx, plan.Operations); err != nil {
		return e.failPlan(ctx, plan, actor, 0, err)
	}

	// Take a fresh snapshot before applying
	snap, err := e.collector.TakeSnapshot(ctx, "write-apply", "")
	if err != nil {
		return nil, fmt.Errorf("taking pre-apply snapshot: %w", err)
	}

	// Determine the target client and transact.
	c, err := e.clientForOps(plan.Operations)
	if err != nil {
		return e.failPlan(ctx, plan, actor, snap.ID, err)
	}
	if err := e.transactOps(ctx, c, plan); err != nil {
		return e.failPlan(ctx, plan, actor, snap.ID, err)
	}

	plan.Status = "applied"
	entry := e.recordAudit(ctx, plan, actor, snap.ID, "success", "")
	return entry, nil
}

// DryRun validates operations and computes diffs without taking a snapshot
// or storing the plan in cache.
func (e *Engine) DryRun(ctx context.Context, ops []WriteOperation) (*Plan, error) {
	if e.rateLimiter != nil && !e.rateLimiter.allow() {
		return nil, ErrRateLimited
	}
	if err := e.validateOps(ctx, ops); err != nil {
		return nil, err
	}

	diffs, err := e.computeDiffs(ctx, ops)
	if err != nil {
		return nil, fmt.Errorf("computing diffs: %w", err)
	}

	plan := &Plan{
		ID:         generateID(),
		CreatedAt:  time.Now().UTC(),
		Operations: ops,
		Diffs:      diffs,
		Status:     "dry-run",
		Impact:     e.computeImpact(ctx, ops),
	}
	return plan, nil
}

// Rollback generates a preview plan that restores changed fields on rows that
// still exist, using a snapshot as the source of truth.
//
// It is deliberately scoped to "restore changed fields only": rows created
// after the snapshot are left untouched, and rows deleted since the snapshot
// are NOT recreated (recreation would assign new server-side UUIDs, dangling
// every reference to the old UUID). Such rows are reported in the plan's
// Warnings so the operator knows the rollback is partial.
func (e *Engine) Rollback(ctx context.Context, snapshotID int64, actor, reason string) (*Plan, error) {
	if e.rateLimiter != nil && !e.rateLimiter.allow() {
		return nil, ErrRateLimited
	}

	if e.collector == nil {
		return nil, fmt.Errorf("rollback requires history collector")
	}

	// Get snapshot rows for NB tables only
	rows, err := e.collector.Store().GetSnapshotRows(ctx, snapshotID, "nb", "")
	if err != nil {
		return nil, fmt.Errorf("loading snapshot rows: %w", err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("snapshot %d not found or has no NB data", snapshotID)
	}

	// Group snapshot rows by table -> uuid -> data
	snapshotState := make(map[string]map[string]map[string]any)
	for _, r := range rows {
		if snapshotState[r.Table] == nil {
			snapshotState[r.Table] = make(map[string]map[string]any)
		}
		snapshotState[r.Table][r.UUID] = r.Data
	}

	// Build operations by comparing snapshot state to current live state
	var ops []WriteOperation
	var warnings []string

	for table, uuidMap := range snapshotState {
		// Only process writable tables
		if _, err := e.registry.Get(table); err != nil {
			continue
		}

		for uuid, snapData := range uuidMap {
			current, err := e.readCurrentState(ctx, table, uuid)
			if err != nil {
				if errors.Is(err, client.ErrNotFound) {
					warnings = append(warnings, fmt.Sprintf(
						"Rollback: %s/%s no longer exists; row recreation is not supported", table, uuid))
					continue
				}
				return nil, fmt.Errorf("reading current state for %s/%s: %w", table, uuid, err)
			}

			// Compare current to snapshot - generate update if different.
			// Snapshot data is JSON-typed (float64/[]any) while current is
			// Go-typed, so compare by JSON encoding rather than DeepEqual.
			var changed bool
			fields := make(map[string]any)
			for k, v := range snapData {
				if k == "_uuid" {
					continue
				}
				if !jsonEqual(v, current[k]) {
					fields[k] = v
					changed = true
				}
			}
			if changed {
				ops = append(ops, WriteOperation{
					Action: "update",
					Table:  table,
					UUID:   uuid,
					Fields: fields,
					Reason: fmt.Sprintf("Rollback: restore %s/%s from snapshot %d", table, uuid, snapshotID),
				})
			}
		}
	}

	if len(ops) == 0 {
		return &Plan{
			ID:        generateID(),
			CreatedAt: time.Now().UTC(),
			Status:    "no-changes",
			Warnings:  warnings,
		}, nil
	}

	// Use Preview to get the full plan with diffs, snapshot, token.
	plan, err := e.Preview(ctx, ops)
	if err != nil {
		return nil, err
	}
	plan.Warnings = warnings
	return plan, nil
}

// jsonEqual reports whether a and b encode to identical JSON. It is used to
// compare snapshot values (decoded from SQLite as float64/[]any/map[string]any)
// against live model values (Go-typed) without spurious differences.
func jsonEqual(a, b any) bool {
	ba, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return reflect.DeepEqual(a, b)
	}
	return bytes.Equal(ba, bb)
}

// GetPlan retrieves a cached plan by ID.
func (e *Engine) GetPlan(id string) (*Plan, bool) {
	return e.plans.Get(id)
}

// CancelPlan removes a plan from cache.
func (e *Engine) CancelPlan(id string) bool {
	plan, ok := e.plans.Get(id)
	if !ok {
		return false
	}
	plan.Status = "expired"
	e.plans.Delete(id)
	return true
}

// QueryAudit returns recent audit entries.
func (e *Engine) QueryAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	return e.auditStore.Query(ctx, limit)
}

// GetAuditEntry returns a single audit entry by ID.
func (e *Engine) GetAuditEntry(ctx context.Context, id int64) (*AuditEntry, error) {
	return e.auditStore.GetByID(ctx, id)
}

// computeDiffs reads current state and computes what each operation would change.
func (e *Engine) computeDiffs(ctx context.Context, ops []WriteOperation) ([]PlanDiff, error) {
	var diffs []PlanDiff

	for _, op := range ops {
		diff := PlanDiff{
			Action: op.Action,
			Table:  op.Table,
			UUID:   op.UUID,
		}

		switch op.Action {
		case "create":
			diff.After = op.Fields

		case "update":
			current, err := e.readCurrentState(ctx, op.Table, op.UUID)
			if err != nil {
				return nil, fmt.Errorf("reading current state for %s/%s: %w", op.Table, op.UUID, err)
			}
			diff.Before = current
			after := make(map[string]any)
			for k, v := range current {
				after[k] = v
			}
			for k, v := range op.Fields {
				after[k] = v
			}
			diff.After = after

			var changes []FieldChange
			for field, newVal := range op.Fields {
				changes = append(changes, FieldChange{
					Field:    field,
					OldValue: current[field],
					NewValue: newVal,
				})
			}
			sort.Slice(changes, func(i, j int) bool {
				return changes[i].Field < changes[j].Field
			})
			diff.Fields = changes

		case "delete":
			current, err := e.readCurrentState(ctx, op.Table, op.UUID)
			if err != nil {
				return nil, fmt.Errorf("reading current state for %s/%s: %w", op.Table, op.UUID, err)
			}
			diff.Before = current
		}

		diffs = append(diffs, diff)
	}

	return diffs, nil
}

// clientForTable returns the appropriate OVSDB client for the given table.
func (e *Engine) clientForTable(table string) (client.Client, error) {
	spec, err := e.registry.Get(table)
	if err != nil {
		return nil, err
	}
	if spec.Database == "sb" {
		if e.sbClient == nil {
			return nil, fmt.Errorf("SB client not available for table %q", table)
		}
		return e.sbClient, nil
	}
	return e.nbClient, nil
}

// readCurrentState fetches the current row from the cache by UUID.
func (e *Engine) readCurrentState(ctx context.Context, table, uuid string) (map[string]any, error) {
	spec, err := e.registry.Get(table)
	if err != nil {
		return nil, err
	}

	c, err := e.clientForTable(table)
	if err != nil {
		return nil, err
	}

	// Create a model instance with the UUID set for lookup.
	modelPtr := reflect.New(spec.ModelType)
	model := modelPtr.Elem()

	// Find and set the UUID field (ovsdb:"_uuid").
	for i := 0; i < spec.ModelType.NumField(); i++ {
		tag := spec.ModelType.Field(i).Tag.Get("ovsdb")
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

	return ovndb.ModelToMap(modelPtr.Interface()), nil
}

// buildOVSDBOps converts WriteOperations into ovsdb.Operation structs using the
// libovsdb client's typed mapper, so map/set/reference columns are wire-encoded
// as RFC 7047 ["map",...]/["set",...]/["uuid",...] rather than plain JSON. Row
// construction is delegated to MapToModel + the client's Create/Update/Delete.
func (e *Engine) buildOVSDBOps(c client.Client, ops []WriteOperation) ([]ovsdb.Operation, error) {
	var ovsdbOps []ovsdb.Operation

	for _, op := range ops {
		spec, err := e.registry.Get(op.Table)
		if err != nil {
			return nil, err
		}

		switch op.Action {
		case "create":
			m, err := MapToModel(op.Fields, spec.ModelType)
			if err != nil {
				return nil, fmt.Errorf("building %s row: %w", op.Table, err)
			}
			created, err := c.Create(m)
			if err != nil {
				return nil, fmt.Errorf("encoding create for %s: %w", op.Table, err)
			}
			ovsdbOps = append(ovsdbOps, created...)

		case "update":
			m, err := hydrateWithUUID(op.Fields, op.UUID, spec.ModelType)
			if err != nil {
				return nil, fmt.Errorf("building %s row: %w", op.Table, err)
			}
			ptrs, err := fieldPointersByTag(m, fieldTags(op.Fields))
			if err != nil {
				return nil, fmt.Errorf("resolving update fields for %s: %w", op.Table, err)
			}
			updated, err := c.Where(m).Update(m, ptrs...)
			if err != nil {
				return nil, fmt.Errorf("encoding update for %s/%s: %w", op.Table, op.UUID, err)
			}
			ovsdbOps = append(ovsdbOps, updated...)

		case "delete":
			m, err := hydrateWithUUID(nil, op.UUID, spec.ModelType)
			if err != nil {
				return nil, fmt.Errorf("building %s row: %w", op.Table, err)
			}
			deleted, err := c.Where(m).Delete()
			if err != nil {
				return nil, fmt.Errorf("encoding delete for %s/%s: %w", op.Table, op.UUID, err)
			}
			ovsdbOps = append(ovsdbOps, deleted...)

		default:
			return nil, fmt.Errorf("unknown action %q", op.Action)
		}
	}

	return ovsdbOps, nil
}

// hydrateWithUUID builds a model from fields plus the given _uuid, so libovsdb's
// Where(model) matches the row by UUID for update/delete operations.
func hydrateWithUUID(fields map[string]any, uuid string, modelType reflect.Type) (any, error) {
	merged := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		merged[k] = v
	}
	merged["_uuid"] = uuid
	return MapToModel(merged, modelType)
}

// fieldTags returns the ovsdb column tags (map keys) of an operation's fields.
func fieldTags(fields map[string]any) []string {
	tags := make([]string, 0, len(fields))
	for k := range fields {
		tags = append(tags, k)
	}
	return tags
}

// recordAudit persists an audit entry. A failed insert is logged rather than
// dropped silently: a mutation may have already been applied, so losing its
// audit trail must at least leave a diagnosable log line.
func (e *Engine) recordAudit(ctx context.Context, plan *Plan, actor string, snapshotID int64, result, errMsg string) *AuditEntry {
	var reason string
	for _, op := range plan.Operations {
		if op.Reason != "" {
			reason = op.Reason
			break
		}
	}
	entry := AuditEntry{
		Timestamp:  time.Now().UTC(),
		PlanID:     plan.ID,
		Actor:      actor,
		Reason:     reason,
		Operations: plan.Operations,
		SnapshotID: snapshotID,
		Result:     result,
		Error:      errMsg,
	}
	if err := e.auditStore.Insert(ctx, entry); err != nil {
		slog.Error("recording write audit entry failed", "plan", plan.ID, "result", result, "err", err)
	}
	return &entry
}

// generateToken creates an HMAC-SHA256 token over planID and snapshotID.
func (e *Engine) generateToken(planID string, snapshotID int64) string {
	mac := hmac.New(sha256.New, e.secret)
	mac.Write([]byte(planID))
	mac.Write([]byte(strconv.FormatInt(snapshotID, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// validateOps runs basic and reference validation for a set of operations.
// Structural and single-database failures are always user input errors (400);
// reference validation may surface either an *InputError (400) or an
// infrastructure error (500), so its error chain is preserved with %w.
func (e *Engine) validateOps(ctx context.Context, ops []WriteOperation) error {
	for i, op := range ops {
		if err := ValidateOperation(op, e.registry); err != nil {
			return &InputError{Message: fmt.Sprintf("operation %d: %s", i, err)}
		}
	}
	if err := ValidateSingleDatabase(ops, e.registry); err != nil {
		return &InputError{Message: err.Error()}
	}
	// The cache is authoritative for reference existence only when the NB
	// monitors are live; during a reconnect or a suspended snapshot session the
	// cache is empty/frozen, so a missing referenced row would be a false
	// negative. When not authoritative, defer existence to OVSDB's server-side
	// referential integrity rather than rejecting a valid write with a 400.
	cacheAuthoritative := e.nbReady == nil || e.nbReady()
	for i, op := range ops {
		spec, err := e.registry.Get(op.Table)
		if err != nil {
			return &InputError{Message: fmt.Sprintf("operation %d: %s", i, err)}
		}
		if spec.Database == "sb" {
			continue
		}
		if err := ValidateReferences(ctx, op, spec, e.nbClient, cacheAuthoritative); err != nil {
			return fmt.Errorf("operation %d: %w", i, err)
		}
	}
	return nil
}

// failPlan marks a plan as failed, records an audit entry, and returns the error.
func (e *Engine) failPlan(ctx context.Context, plan *Plan, actor string, snapshotID int64, err error) (*AuditEntry, error) {
	plan.Status = "failed"
	entry := e.recordAudit(ctx, plan, actor, snapshotID, "error", err.Error())
	return entry, err
}

// clientForOps returns the appropriate OVSDB client for a set of operations.
// All operations must target the same database (enforced by ValidateSingleDatabase).
func (e *Engine) clientForOps(ops []WriteOperation) (client.Client, error) {
	if len(ops) == 0 {
		return e.nbClient, nil
	}
	return e.clientForTable(ops[0].Table)
}

// transactOps transacts a plan's operations, prefixed with RFC 7047 wait
// operations that assert each target row still holds the values captured at
// preview time. If a wait fails, the whole transaction aborts and Apply reports
// ErrStaleState instead of silently no-op-ing an update against a changed row.
func (e *Engine) transactOps(ctx context.Context, c client.Client, plan *Plan) error {
	waitOps, err := e.buildWaitOps(c, plan)
	if err != nil {
		return fmt.Errorf("building wait operations: %w", err)
	}
	mutationOps, err := e.buildOVSDBOps(c, plan.Operations)
	if err != nil {
		return fmt.Errorf("building OVSDB operations: %w", err)
	}

	allOps := append(waitOps, mutationOps...)
	reply, err := c.Transact(ctx, allOps...)
	if err != nil {
		return fmt.Errorf("transact: %w", err)
	}
	// A failed wait op (results are 1:1 with ops) means a targeted row changed
	// or was deleted since preview.
	for i := 0; i < len(waitOps) && i < len(reply); i++ {
		if reply[i].Error != "" {
			return fmt.Errorf("%w: %s", ErrStaleState, reply[i].Error)
		}
	}
	if _, err = ovsdb.CheckOperationResults(reply, allOps); err != nil {
		return fmt.Errorf("operation failed: %w", err)
	}
	return nil
}

// buildWaitOps returns the wait operations that assert each update/delete
// target still matches the values captured in the plan's diffs. Create
// operations have no precondition. Diffs are 1:1 with Operations by
// construction in computeDiffs.
func (e *Engine) buildWaitOps(c client.Client, plan *Plan) ([]ovsdb.Operation, error) {
	timeout0 := 0
	var waitOps []ovsdb.Operation

	for i, op := range plan.Operations {
		spec, err := e.registry.Get(op.Table)
		if err != nil {
			return nil, err
		}

		var before map[string]any
		if i < len(plan.Diffs) {
			before = plan.Diffs[i].Before
		}

		switch op.Action {
		case "update":
			// Assert the columns being changed still hold their pre-apply values.
			subset := map[string]any{"_uuid": op.UUID}
			tags := make([]string, 0, len(op.Fields))
			for k := range op.Fields {
				subset[k] = before[k]
				tags = append(tags, k)
			}
			m, err := MapToModel(subset, spec.ModelType)
			if err != nil {
				return nil, fmt.Errorf("hydrating wait row for %s/%s: %w", op.Table, op.UUID, err)
			}
			ptrs, err := fieldPointersByTag(m, tags)
			if err != nil {
				return nil, err
			}
			ops, err := c.Where(m).Wait(ovsdb.WaitConditionEqual, &timeout0, m, ptrs...)
			if err != nil {
				return nil, fmt.Errorf("building wait op for %s/%s: %w", op.Table, op.UUID, err)
			}
			waitOps = append(waitOps, ops...)

		case "delete":
			// Assert the row still exists (matches its own _uuid).
			m, err := hydrateWithUUID(nil, op.UUID, spec.ModelType)
			if err != nil {
				return nil, fmt.Errorf("hydrating wait row for %s/%s: %w", op.Table, op.UUID, err)
			}
			ptrs, err := fieldPointersByTag(m, []string{"_uuid"})
			if err != nil {
				return nil, err
			}
			ops, err := c.Where(m).Wait(ovsdb.WaitConditionEqual, &timeout0, m, ptrs...)
			if err != nil {
				return nil, fmt.Errorf("building wait op for %s/%s: %w", op.Table, op.UUID, err)
			}
			waitOps = append(waitOps, ops...)
		}
	}

	return waitOps, nil
}

// computeImpact runs impact analysis for delete operations if a resolver is configured.
func (e *Engine) computeImpact(ctx context.Context, ops []WriteOperation) []ImpactEntry {
	if e.resolver == nil {
		return nil
	}

	var entries []ImpactEntry
	for i, op := range ops {
		if op.Action != "delete" || op.UUID == "" {
			continue
		}
		spec, err := e.registry.Get(op.Table)
		if err != nil {
			continue
		}
		result, err := e.resolver.Resolve(ctx, spec.Database, op.Table, op.UUID)
		if err != nil || result.Summary.TotalAffected == 0 {
			continue
		}
		entries = append(entries, ImpactEntry{
			OperationIndex: i,
			Result:         result,
		})
	}
	return entries
}

// generateID creates a short random hex ID.
// crypto/rand.Read failure indicates an unrecoverable system state
// (e.g. exhausted entropy on an init-system-misconfigured host) and
// nothing meaningful can be done with the error this deep in the call
// chain, so we panic.
func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}
