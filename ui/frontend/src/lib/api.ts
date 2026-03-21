import { clusterPath } from './clusterStore';

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

export async function get<T>(path: string): Promise<T> {
  const res = await fetch(clusterPath(path));
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body.error || res.statusText);
  }
  return res.json();
}

// Raw table endpoints
export function listTable(
  db: string,
  table: string,
): Promise<Record<string, unknown>[]> {
  return get(`/api/v1/${db}/${table}`);
}

export function getEntity(
  db: string,
  table: string,
  uuid: string,
): Promise<Record<string, unknown>> {
  return get(`/api/v1/${db}/${table}/${uuid}`);
}

// Search
export interface SearchResponse {
  query: string;
  query_type: string;
  results: SearchResultGroup[];
}

export interface SearchResultGroup {
  database: string;
  table: string;
  matches: Record<string, unknown>[];
}

export function search(query: string): Promise<SearchResponse> {
  return get(`/api/v1/search?q=${encodeURIComponent(query)}`);
}

// Capabilities
export async function getCapabilities(): Promise<string[]> {
  const data = await get<{ capabilities: string[] }>('/api/v1/capabilities');
  return data.capabilities;
}

// Correlated response types
export type OvsdbEntity = Record<string, unknown>;

export interface Enrichment {
  display_name?: string;
  project_name?: string;
  project_id?: string;
  device_owner?: string;
  device_id?: string;
  device_name?: string;
  extra?: Record<string, string>;
}

export interface PortBindingChain {
  logical_switch_port?: OvsdbEntity & { enrichment?: Enrichment };
  logical_router_port?: OvsdbEntity;
  port_binding?: OvsdbEntity;
  chassis?: OvsdbEntity;
  encaps?: OvsdbEntity[];
  datapath_binding?: OvsdbEntity;
  logical_switch?: OvsdbEntity;
  logical_router?: OvsdbEntity;
}

export interface SwitchCorrelated {
  logical_switch: OvsdbEntity & { enrichment?: Enrichment };
  datapath_binding?: OvsdbEntity;
  ports: PortBindingChain[];
}

export interface RouterCorrelated {
  logical_router: OvsdbEntity & { enrichment?: Enrichment };
  datapath_binding?: OvsdbEntity;
  ports: PortBindingChain[];
  nats: (OvsdbEntity & { enrichment?: Enrichment })[];
}

export interface ChassisCorrelated {
  chassis: OvsdbEntity;
  chassis_private?: OvsdbEntity;
  encaps: OvsdbEntity[];
  port_bindings: OvsdbEntity[];
}

// Correlated endpoints
export function listCorrelatedSwitches(): Promise<SwitchCorrelated[]> {
  return get('/api/v1/correlated/logical-switches');
}

export function getCorrelatedSwitch(uuid: string): Promise<SwitchCorrelated> {
  return get(`/api/v1/correlated/logical-switches/${uuid}`);
}

export function listCorrelatedRouters(): Promise<RouterCorrelated[]> {
  return get('/api/v1/correlated/logical-routers');
}

export function getCorrelatedRouter(uuid: string): Promise<RouterCorrelated> {
  return get(`/api/v1/correlated/logical-routers/${uuid}`);
}

export function listCorrelatedChassis(): Promise<ChassisCorrelated[]> {
  return get('/api/v1/correlated/chassis');
}

export function getCorrelatedChassis(uuid: string): Promise<ChassisCorrelated> {
  return get(`/api/v1/correlated/chassis/${uuid}`);
}

export function getCorrelatedLSP(uuid: string): Promise<PortBindingChain> {
  return get(`/api/v1/correlated/logical-switch-ports/${uuid}`);
}

export function getCorrelatedLRP(uuid: string): Promise<PortBindingChain> {
  return get(`/api/v1/correlated/logical-router-ports/${uuid}`);
}

export function getCorrelatedPortBinding(
  uuid: string,
): Promise<PortBindingChain> {
  return get(`/api/v1/correlated/port-bindings/${uuid}`);
}

// Topology
export interface TopologyNode {
  id: string;
  type: string;
  label: string;
  group?: string;
  metadata?: Record<string, string>;
}

export interface TopologyEdge {
  source: string;
  target: string;
  type: string;
}

export interface TopologyResponse {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
}

export function getTopology(opts?: {
  vms?: boolean;
}): Promise<TopologyResponse> {
  const params = new URLSearchParams();
  if (opts?.vms) params.set('vms', 'true');
  const qs = params.toString();
  return get(`/api/v1/topology${qs ? '?' + qs : ''}`);
}

// Flow Pipeline
export interface FlowEntry {
  uuid: string;
  priority: number;
  match: string;
  actions: string;
  external_ids?: Record<string, string>;
}

export interface FlowTableGroup {
  table_id: number;
  table_name?: string;
  flows: FlowEntry[];
}

export interface FlowPipelineResponse {
  datapath_uuid: string;
  datapath_name: string;
  ingress: FlowTableGroup[];
  egress: FlowTableGroup[];
}

export function getFlows(datapathUuid: string): Promise<FlowPipelineResponse> {
  return get(`/api/v1/flows?datapath=${encodeURIComponent(datapathUuid)}`);
}

export function listDatapathBindings(): Promise<Record<string, unknown>[]> {
  return get('/api/v1/sb/datapath-bindings');
}

// Debug: Port Diagnostics
export interface DiagnosticCheck {
  name: string;
  status: 'healthy' | 'warning' | 'error';
  message: string;
}

export interface PortDiagnostic {
  port_uuid: string;
  port_name: string;
  port_type: string;
  switch_name?: string;
  overall: 'healthy' | 'warning' | 'error';
  checks: DiagnosticCheck[];
}

export interface PortDiagnosticsSummary {
  total: number;
  healthy: number;
  warning: number;
  error: number;
  ports: PortDiagnostic[];
}

export function getPortDiagnostics(): Promise<PortDiagnosticsSummary> {
  return get('/api/v1/debug/port-diagnostics');
}

export function getPortDiagnostic(uuid: string): Promise<PortDiagnostic> {
  return get(`/api/v1/debug/port-diagnostics/${uuid}`);
}

// Debug: Connectivity Checker
export interface ConnectivityCheck {
  name: string;
  category: string;
  status: 'pass' | 'fail' | 'warning' | 'skipped';
  message: string;
  details?: unknown;
}

export interface PortInfo {
  uuid: string;
  name: string;
  type?: string;
  switch_name?: string;
  bound_chassis?: string;
  addresses?: string[];
}

export interface ConnectivityResult {
  source: PortInfo;
  destination: PortInfo;
  overall: 'pass' | 'fail' | 'warning' | 'skipped';
  checks: ConnectivityCheck[];
}

export function checkConnectivity(
  srcUuid: string,
  dstUuid: string,
): Promise<ConnectivityResult> {
  return get(
    `/api/v1/debug/connectivity?src=${encodeURIComponent(srcUuid)}&dst=${encodeURIComponent(dstUuid)}`,
  );
}

// Debug: Packet Trace
export interface TraceFlowEntry {
  uuid: string;
  priority: number;
  match: string;
  actions: string;
  hint: string;
  selected: boolean;
}

export interface TraceStage {
  pipeline: string;
  table_id: number;
  table_name?: string;
  flows: TraceFlowEntry[];
}

export interface TraceResponse {
  port_uuid: string;
  port_name: string;
  datapath_uuid: string;
  datapath_name: string;
  dst_ip?: string;
  protocol?: string;
  stages: TraceStage[];
}

export function getTrace(
  portUuid: string,
  opts?: { dstIp?: string; protocol?: string },
): Promise<TraceResponse> {
  const params = new URLSearchParams();
  params.set('port', portUuid);
  if (opts?.dstIp) params.set('dst_ip', opts.dstIp);
  if (opts?.protocol) params.set('protocol', opts.protocol);
  return get(`/api/v1/debug/trace?${params.toString()}`);
}

// Debug: Flow Diff
export interface FlowChange {
  timestamp: number;
  type: 'insert' | 'update' | 'delete';
  uuid: string;
  old_row?: Record<string, unknown>;
  new_row?: Record<string, unknown>;
  datapath?: string;
}

export interface FlowDiffResponse {
  changes: FlowChange[];
  count: number;
}

export function getFlowDiff(opts?: {
  datapath?: string;
  since?: number;
}): Promise<FlowDiffResponse> {
  const params = new URLSearchParams();
  if (opts?.datapath) params.set('datapath', opts.datapath);
  if (opts?.since) params.set('since', String(opts.since));
  const qs = params.toString();
  return get(`/api/v1/debug/flow-diff${qs ? '?' + qs : ''}`);
}

// Port bindings listing (for trace port selector)
export function listPortBindings(): Promise<Record<string, unknown>[]> {
  return get('/api/v1/sb/port-bindings');
}

// Logical switch ports listing (for connectivity port selector)
export function listLogicalSwitchPorts(): Promise<Record<string, unknown>[]> {
  return get('/api/v1/nb/logical-switch-ports');
}

// --- History & Snapshots ---

export async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(clusterPath(path), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const b = await res.json().catch(() => ({}));
    throw new ApiError(res.status, b.error || res.statusText);
  }
  return res.json();
}

export async function del(path: string): Promise<void> {
  const res = await fetch(clusterPath(path), { method: 'DELETE' });
  if (!res.ok) {
    const b = await res.json().catch(() => ({}));
    throw new ApiError(res.status, b.error || res.statusText);
  }
}

export interface SnapshotMeta {
  id: number;
  timestamp: string;
  trigger: string;
  label: string;
  row_counts: Record<string, number>;
  size_bytes: number;
}

export interface SnapshotRow {
  database: string;
  table: string;
  uuid: string;
  data: Record<string, unknown>;
}

export interface FieldChange {
  field: string;
  old_value: unknown;
  new_value: unknown;
}

export interface RowDiff {
  uuid: string;
  fields: FieldChange[];
}

export interface TableDiff {
  database: string;
  table: string;
  added: Record<string, unknown>[];
  removed: Record<string, unknown>[];
  modified: RowDiff[];
}

export interface DiffResult {
  from_id: number;
  to_id: number;
  tables: TableDiff[];
}

export interface EventRecord {
  id: number;
  timestamp: string;
  type: string;
  database: string;
  table: string;
  uuid: string;
  row?: Record<string, unknown>;
  old_row?: Record<string, unknown>;
}

export function listSnapshots(): Promise<SnapshotMeta[]> {
  return get('/api/v1/snapshots');
}

export function createSnapshot(label?: string): Promise<SnapshotMeta> {
  return post('/api/v1/snapshots', label ? { label } : {});
}

export function getSnapshotDetail(id: number): Promise<SnapshotMeta> {
  return get(`/api/v1/snapshots/${id}`);
}

export function getSnapshotRows(
  id: number,
  opts?: { database?: string; table?: string },
): Promise<SnapshotRow[]> {
  const params = new URLSearchParams();
  if (opts?.database) params.set('database', opts.database);
  if (opts?.table) params.set('table', opts.table);
  const qs = params.toString();
  return get(`/api/v1/snapshots/${id}/rows${qs ? '?' + qs : ''}`);
}

export function deleteSnapshot(id: number): Promise<void> {
  return del(`/api/v1/snapshots/${id}`);
}

export function diffSnapshots(
  from: number,
  to: number,
  table?: string,
): Promise<DiffResult> {
  const params = new URLSearchParams();
  params.set('from', String(from));
  params.set('to', String(to));
  if (table) params.set('table', table);
  return get(`/api/v1/snapshots/diff?${params.toString()}`);
}

export function queryEvents(opts?: {
  since?: string;
  until?: string;
  database?: string;
  table?: string;
  type?: string;
  limit?: number;
}): Promise<EventRecord[]> {
  const params = new URLSearchParams();
  if (opts?.since) params.set('since', opts.since);
  if (opts?.until) params.set('until', opts.until);
  if (opts?.database) params.set('database', opts.database);
  if (opts?.table) params.set('table', opts.table);
  if (opts?.type) params.set('type', opts.type);
  if (opts?.limit) params.set('limit', String(opts.limit));
  const qs = params.toString();
  return get(`/api/v1/events${qs ? '?' + qs : ''}`);
}
