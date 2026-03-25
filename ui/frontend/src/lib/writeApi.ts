import { get, post, del } from './api';

// --- Types ---

export interface WriteOperation {
  action: 'create' | 'update' | 'delete';
  table: string;
  uuid?: string;
  fields?: Record<string, unknown>;
  reason?: string;
}

export interface FieldChange {
  field: string;
  old_value?: unknown;
  new_value?: unknown;
}

export interface PlanDiff {
  action: string;
  table: string;
  uuid?: string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
  fields: FieldChange[];
}

export interface ImpactNode {
  database: string;
  table: string;
  uuid: string;
  name?: string;
  ref_type: 'root' | 'strong' | 'weak' | 'reverse' | 'correlation';
  column?: string;
  children?: ImpactNode[];
}

export interface ImpactSummary {
  total_affected: number;
  by_table: Record<string, number>;
  by_ref_type: Record<string, number>;
  max_depth: number;
  truncated: boolean;
}

export interface ImpactResult {
  root: ImpactNode;
  summary: ImpactSummary;
}

export interface ImpactEntry {
  operation_index: number;
  result: ImpactResult;
}

export interface Plan {
  id: string;
  created_at: string;
  expires_at: string;
  operations: WriteOperation[];
  diffs: PlanDiff[];
  snapshot_id: number;
  status: 'pending' | 'applied' | 'expired' | 'failed' | 'dry-run';
  apply_token: string;
  impact?: ImpactEntry[];
}

export interface AuditEntry {
  id: number;
  timestamp: string;
  plan_id: string;
  actor: string;
  reason: string;
  operations: WriteOperation[];
  snapshot_id: number;
  result: 'success' | 'error';
  error?: string;
}

export interface SchemaFieldInfo {
  name: string;
  type: string;
  optional: boolean;
  read_only: boolean;
}

export interface TableSchema {
  table: string;
  fields: SchemaFieldInfo[];
}

// --- API Functions ---

let schemaCache: TableSchema[] | null = null;

export async function getWriteSchema(): Promise<TableSchema[]> {
  if (schemaCache) return schemaCache;
  const data = await get<{ tables: TableSchema[] }>('/api/v1/write/schema');
  schemaCache = data.tables;
  return data.tables;
}

export function getTableSchema(
  tableName: string,
  schemas: TableSchema[],
): TableSchema | undefined {
  return schemas.find((s) => s.table === tableName);
}

export function previewOperations(
  operations: WriteOperation[],
  reason?: string,
): Promise<Plan> {
  return post('/api/v1/write/preview', { operations, reason });
}

export function dryRunOperations(
  operations: WriteOperation[],
  reason?: string,
): Promise<Plan> {
  return post('/api/v1/write/dry-run', { operations, reason });
}

export function getPlan(id: string): Promise<Plan> {
  return get(`/api/v1/write/plans/${id}`);
}

export function applyPlan(
  id: string,
  applyToken: string,
  actor?: string,
): Promise<AuditEntry> {
  return post(`/api/v1/write/plans/${id}/apply`, {
    apply_token: applyToken,
    actor,
  });
}

export function cancelPlan(id: string): Promise<void> {
  return del(`/api/v1/write/plans/${id}`);
}

export function listAuditEntries(limit?: number): Promise<AuditEntry[]> {
  const qs = limit ? `?limit=${limit}` : '';
  return get(`/api/v1/write/audit${qs}`);
}

export function getAuditEntry(id: number): Promise<AuditEntry> {
  return get(`/api/v1/write/audit/${id}`);
}

// --- Impact API ---

export function getImpact(
  db: string,
  table: string,
  uuid: string,
): Promise<ImpactResult> {
  return get(`/api/v1/impact/${db}/${table}/${uuid}`);
}

// --- Failover API ---

export interface FailoverRequest {
  group_name: string;
  target_chassis: string;
}

export interface EvacuateRequest {
  chassis_name: string;
}

export function requestFailover(req: FailoverRequest): Promise<Plan> {
  return post('/api/v1/write/failover', req);
}

export function requestEvacuate(req: EvacuateRequest): Promise<Plan> {
  return post('/api/v1/write/evacuate', req);
}

export interface RestoreRequest {
  chassis_name: string;
}

export function requestRestore(req: RestoreRequest): Promise<Plan> {
  return post('/api/v1/write/restore', req);
}
