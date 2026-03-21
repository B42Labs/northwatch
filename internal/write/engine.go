package write

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/history"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
)

// Engine orchestrates safe writes to the OVN Northbound database.
type Engine struct {
	nbClient    client.Client
	registry    *Registry
	collector   *history.Collector
	auditStore  *AuditStore
	plans       *PlanCache
	secret      []byte
	mu          sync.Mutex
	rateLimiter *rateLimiter
}

// NewEngine creates a new write Engine with the given rate limit (operations per minute, 0 = unlimited).
func NewEngine(nbClient client.Client, registry *Registry, collector *history.Collector, auditStore *AuditStore, planTTL time.Duration, rateLimit int) *Engine {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	cache := NewPlanCache(planTTL)
	e := &Engine{
		nbClient:   nbClient,
		registry:   registry,
		collector:  collector,
		auditStore: auditStore,
		plans:      cache,
		secret:     secret,
	}
	if rateLimit > 0 {
		e.rateLimiter = newRateLimiter(rateLimit)
	}
	go cache.StartCleanup(planTTL)
	return e
}

// Schema returns the schema for all writable tables.
func (e *Engine) Schema() []TableSchema {
	return e.registry.Schema()
}

// Preview validates operations, reads current state, computes diffs,
// takes a snapshot, generates an HMAC token, and stores the plan in cache.
func (e *Engine) Preview(ctx context.Context, ops []WriteOperation) (*Plan, error) {
	if e.rateLimiter != nil && !e.rateLimiter.allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	for i, op := range ops {
		if err := ValidateOperation(op, e.registry); err != nil {
			return nil, fmt.Errorf("operation %d: %w", i, err)
		}
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
	}

	e.plans.Store(plan)
	return plan, nil
}

// Apply verifies the token, locks, takes a fresh snapshot, builds libovsdb
// operations, transacts, and records an audit entry.
func (e *Engine) Apply(ctx context.Context, planID, token, actor string) (*AuditEntry, error) {
	if e.rateLimiter != nil && !e.rateLimiter.allow() {
		return nil, fmt.Errorf("rate limit exceeded")
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

	// Take a fresh snapshot before applying
	snap, err := e.collector.TakeSnapshot(ctx, "write-apply", "")
	if err != nil {
		return nil, fmt.Errorf("taking pre-apply snapshot: %w", err)
	}

	ovsdbOps, err := e.buildOVSDBOps(plan.Operations)
	if err != nil {
		plan.Status = "failed"
		entry := e.recordAudit(ctx, plan, actor, snap.ID, "error", err.Error())
		return entry, fmt.Errorf("building OVSDB operations: %w", err)
	}

	reply, err := e.nbClient.Transact(ctx, ovsdbOps...)
	if err != nil {
		plan.Status = "failed"
		entry := e.recordAudit(ctx, plan, actor, snap.ID, "error", err.Error())
		return entry, fmt.Errorf("transacting: %w", err)
	}

	_, err = ovsdb.CheckOperationResults(reply, ovsdbOps)
	if err != nil {
		plan.Status = "failed"
		entry := e.recordAudit(ctx, plan, actor, snap.ID, "error", err.Error())
		return entry, fmt.Errorf("operation failed: %w", err)
	}

	plan.Status = "applied"
	entry := e.recordAudit(ctx, plan, actor, snap.ID, "success", "")
	return entry, nil
}

// DryRun validates operations and computes diffs without taking a snapshot
// or storing the plan in cache.
func (e *Engine) DryRun(ctx context.Context, ops []WriteOperation) (*Plan, error) {
	for i, op := range ops {
		if err := ValidateOperation(op, e.registry); err != nil {
			return nil, fmt.Errorf("operation %d: %w", i, err)
		}
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
	}
	return plan, nil
}

// Rollback is not yet implemented. It will generate a preview plan that
// reverses the changes since a given snapshot by diffing snapshot rows
// against live state.
func (e *Engine) Rollback(_ context.Context, _ int64, _, _ string) (*Plan, error) {
	return nil, fmt.Errorf("rollback is not yet implemented")
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

// readCurrentState fetches the current row from the NB cache by UUID.
func (e *Engine) readCurrentState(ctx context.Context, table, uuid string) (map[string]any, error) {
	spec, err := e.registry.Get(table)
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

	if err := e.nbClient.Get(ctx, modelPtr.Interface()); err != nil {
		return nil, fmt.Errorf("row not found: %w", err)
	}

	return api.ModelToMap(modelPtr.Interface()), nil
}

// buildOVSDBOps converts WriteOperations into raw ovsdb.Operation structs.
func (e *Engine) buildOVSDBOps(ops []WriteOperation) ([]ovsdb.Operation, error) {
	var ovsdbOps []ovsdb.Operation

	for _, op := range ops {
		switch op.Action {
		case "create":
			row := make(map[string]interface{})
			for k, v := range op.Fields {
				row[k] = v
			}
			ovsdbOps = append(ovsdbOps, ovsdb.Operation{
				Op:    "insert",
				Table: op.Table,
				Row:   row,
			})

		case "update":
			row := make(map[string]interface{})
			for k, v := range op.Fields {
				row[k] = v
			}
			ovsdbOps = append(ovsdbOps, ovsdb.Operation{
				Op:    "update",
				Table: op.Table,
				Where: []ovsdb.Condition{{
					Column:   "_uuid",
					Function: ovsdb.ConditionEqual,
					Value:    ovsdb.UUID{GoUUID: op.UUID},
				}},
				Row: row,
			})

		case "delete":
			ovsdbOps = append(ovsdbOps, ovsdb.Operation{
				Op:    "delete",
				Table: op.Table,
				Where: []ovsdb.Condition{{
					Column:   "_uuid",
					Function: ovsdb.ConditionEqual,
					Value:    ovsdb.UUID{GoUUID: op.UUID},
				}},
			})

		default:
			return nil, fmt.Errorf("unknown action %q", op.Action)
		}
	}

	return ovsdbOps, nil
}

// recordAudit persists an audit entry (best-effort).
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
	_ = e.auditStore.Insert(ctx, entry)
	return &entry
}

// generateToken creates an HMAC-SHA256 token over planID and snapshotID.
func (e *Engine) generateToken(planID string, snapshotID int64) string {
	mac := hmac.New(sha256.New, e.secret)
	mac.Write([]byte(planID))
	mac.Write([]byte(strconv.FormatInt(snapshotID, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// generateID creates a short random hex ID.
func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}
