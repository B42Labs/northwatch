import { clusterPath } from './clusterStore';
import { getApiToken } from './authStore';

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

/**
 * apiFetch is the single exit point to the API. It attaches the configured
 * bearer token, which mutating endpoints require, and turns a 401 into a
 * message that says what to do about it rather than a bare "Unauthorized".
 */
async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const token = getApiToken();
  if (!token) {
    return init ? fetch(path, init) : fetch(path);
  }
  return fetch(path, {
    ...init,
    headers: { ...init?.headers, Authorization: `Bearer ${token}` },
  });
}

/** failed builds the ApiError for a non-2xx response. */
async function failed(res: Response): Promise<ApiError> {
  const body = await res.json().catch(() => ({}));
  const message =
    res.status === 401
      ? 'authentication required — set an API token'
      : body.error || res.statusText;
  return new ApiError(res.status, message);
}

export async function get<T>(path: string): Promise<T> {
  const res = await apiFetch(clusterPath(path));
  if (!res.ok) throw await failed(res);
  return res.json();
}

// Global requests target the main mux directly, bypassing clusterPath(). Use
// them for resources that exist only at the top level — history snapshots,
// events, capabilities, the cluster list — which a snapshot cluster's sub-mux
// does not serve (it would return 404 when such a cluster is active).
async function getGlobal<T>(path: string): Promise<T> {
  const res = await apiFetch(path);
  if (!res.ok) throw await failed(res);
  return res.json();
}

async function postGlobal<T>(path: string, body: unknown): Promise<T> {
  const res = await apiFetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw await failed(res);
  return res.json();
}

async function delGlobal(path: string): Promise<void> {
  const res = await apiFetch(path, { method: 'DELETE' });
  if (!res.ok) throw await failed(res);
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
  // Set when the per-table or total match cap dropped further matches — the
  // result set is a sample, not the whole answer.
  truncated?: boolean;
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
export interface SnapshotInfo {
  createdAt?: string;
  nbAddr?: string;
  sbAddr?: string;
}

export interface CapabilitiesResponse {
  capabilities: string[];
  mode: 'live' | 'snapshot';
  snapshot?: SnapshotInfo;
}

export async function getCapabilities(): Promise<CapabilitiesResponse> {
  const data = await getGlobal<Partial<CapabilitiesResponse>>(
    '/api/v1/capabilities',
  );
  return {
    capabilities: data.capabilities ?? [],
    mode: data.mode ?? 'live',
    snapshot: data.snapshot,
  };
}

// --- Per-chassis OVS visibility ---
// The OVS endpoints are served on the top-level mux (default cluster only), so
// they use the global fetch helper rather than the cluster-scoped get().

/** Connection status of one chassis in the OVS pool. */
export interface OvsMemberStatus {
  system_id: string;
  connected: boolean;
}

/** The read-only OVS tables the backend exposes, as URL slug → display label.
 * Keep in sync with the `ovsTables` whitelist in internal/api/handler/ovs.go. */
export const OVS_TABLES: { slug: string; label: string }[] = [
  { slug: 'interface', label: 'Interface' },
  { slug: 'port', label: 'Port' },
  { slug: 'bridge', label: 'Bridge' },
  { slug: 'open-vswitch', label: 'Open_vSwitch' },
  { slug: 'controller', label: 'Controller' },
  { slug: 'manager', label: 'Manager' },
  // Monitoring / export-config tables.
  { slug: 'ipfix', label: 'IPFIX' },
  { slug: 'sflow', label: 'sFlow' },
  { slug: 'netflow', label: 'NetFlow' },
  { slug: 'mirror', label: 'Mirror' },
  { slug: 'qos', label: 'QoS' },
  { slug: 'queue', label: 'Queue' },
  // Conntrack tables.
  { slug: 'ct-zone', label: 'CT_Zone' },
  { slug: 'ct-timeout-policy', label: 'CT_Timeout_Policy' },
  { slug: 'datapath', label: 'Datapath' },
  // Remaining tables. The ssl row omits private_key (redacted server-side).
  { slug: 'flow-table', label: 'Flow_Table' },
  { slug: 'flow-sample-collector-set', label: 'Flow_Sample_Collector_Set' },
  { slug: 'ssl', label: 'SSL' },
  { slug: 'autoattach', label: 'AutoAttach' },
];

export async function listOvsMembers(): Promise<OvsMemberStatus[]> {
  // The list is always a JSON array, but stay defensive against a null body.
  const data = await getGlobal<OvsMemberStatus[] | null>('/api/v1/ovs');
  return data ?? [];
}

/** Per-chassis OVS health breakdown. For an unreachable chassis (connected
 * false) every count is zero — it is excluded from the fleet totals. */
export interface OvsChassisHealth {
  system_id: string;
  connected: boolean;
  bridges: number;
  ports: number;
  interfaces: number;
  down_interfaces: number;
  error_interfaces: number;
  drop_interfaces: number;
}

/** Aggregated fleet-wide OVS health: connection counts, summed
 * bridge/port/interface totals across connected chassis, the down/erroring
 * interface counts, and the per-chassis breakdown. Unreachable chassis are
 * counted in `chassis`/`unreachable` but excluded from every other total. */
export interface OvsFleetHealth {
  chassis: number;
  connected: number;
  unreachable: number;
  bridges: number;
  ports: number;
  interfaces: number;
  down_interfaces: number;
  error_interfaces: number;
  drop_interfaces: number;
  members: OvsChassisHealth[];
}

export async function getOvsFleetHealth(): Promise<OvsFleetHealth> {
  const data = await getGlobal<OvsFleetHealth>('/api/v1/ovs/health');
  // The backend marshals an empty (nil) members slice to JSON null; normalize
  // so callers can always iterate a real array.
  return { ...data, members: data.members ?? [] };
}

export async function listOvsTable(
  chassis: string,
  table: string,
): Promise<Record<string, unknown>[]> {
  const data = await getGlobal<Record<string, unknown>[] | null>(
    `/api/v1/ovs/${encodeURIComponent(chassis)}/${encodeURIComponent(table)}`,
  );
  return data ?? [];
}

export function getOvsEntity(
  chassis: string,
  table: string,
  uuid: string,
): Promise<Record<string, unknown>> {
  return getGlobal(
    `/api/v1/ovs/${encodeURIComponent(chassis)}/${encodeURIComponent(table)}/${encodeURIComponent(uuid)}`,
  );
}

/** One SB Port_Binding realizing an OVS interface (from the correlation
 * endpoint), with its chassis and datapath resolved to labels. */
export interface OvsBinding {
  uuid: string;
  logical_port: string;
  type?: string;
  up?: boolean;
  chassis?: string;
  bound_here: boolean;
  datapath?: string;
  datapath_uuid?: string;
}

/** Correlation of a live OVS interface with OVN intent. bound is false — with no
 * binding — when the interface carries no iface-id or no SB Port_Binding matches
 * it; drift flags where SB reports the port up but the interface is down or
 * erroring. */
export interface OvsInterfaceCorrelation {
  iface_id?: string;
  bound: boolean;
  binding?: OvsBinding;
  drift?: string[];
}

export function getOvsInterfaceCorrelation(
  chassis: string,
  uuid: string,
): Promise<OvsInterfaceCorrelation> {
  return getGlobal(
    `/api/v1/ovs/${encodeURIComponent(chassis)}/interface/${encodeURIComponent(uuid)}/correlation`,
  );
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

export async function getTopology(opts?: {
  vms?: boolean;
}): Promise<TopologyResponse> {
  const params = new URLSearchParams();
  if (opts?.vms) params.set('vms', 'true');
  const qs = params.toString();
  const data = await get<TopologyResponse>(
    `/api/v1/topology${qs ? '?' + qs : ''}`,
  );
  // The backend marshals empty (nil) slices to JSON null, so an OVN
  // deployment with no logical topology returns {"nodes":null,"edges":null}.
  // Normalize here so every caller can rely on real arrays.
  return { nodes: data.nodes ?? [], edges: data.edges ?? [] };
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

// Gateway (chassisredirect) health — desired vs. actual gateway ownership.
export interface GatewayCheck {
  name: string;
  status: 'healthy' | 'warning' | 'error';
  message: string;
}

export interface GatewayMember {
  chassis_uuid?: string;
  name: string;
  hostname?: string;
  priority: number;
  present: boolean;
  stale: boolean;
  gateway_capable: boolean;
  alive: boolean;
  active: boolean;
  desired: boolean;
}

export interface GatewayHealth {
  cr_port: string;
  port_binding_uuid: string;
  lrp_name?: string;
  router_uuid?: string;
  router_name?: string;
  ha_group_uuid?: string;
  ha_group_name?: string;
  external_networks?: string[];
  served_ips?: string[];
  desired_chassis?: string;
  actual_chassis?: string;
  members: GatewayMember[];
  status: string;
  overall: 'healthy' | 'warning' | 'error';
  checks: GatewayCheck[];
}

export interface GatewayConflict {
  external_ip: string;
  chassis: string[];
  cr_ports: string[];
}

export interface GatewayHealthReport {
  total: number;
  healthy: number;
  warning: number;
  error: number;
  gateways: GatewayHealth[];
  conflicts?: GatewayConflict[];
}

export async function getGatewayHealth(): Promise<GatewayHealthReport> {
  const data = await get<GatewayHealthReport>('/api/v1/topology/gateway');
  // The backend marshals nil slices to JSON null; normalize to arrays.
  return {
    ...data,
    gateways: data.gateways ?? [],
    conflicts: data.conflicts ?? [],
  };
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
  const res = await apiFetch(clusterPath(path), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw await failed(res);
  return res.json();
}

export async function del(path: string): Promise<void> {
  const res = await apiFetch(clusterPath(path), { method: 'DELETE' });
  if (!res.ok) throw await failed(res);
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
  return getGlobal('/api/v1/snapshots');
}

export function createSnapshot(label?: string): Promise<SnapshotMeta> {
  return postGlobal('/api/v1/snapshots', label ? { label } : {});
}

export function getSnapshotDetail(id: number): Promise<SnapshotMeta> {
  return getGlobal(`/api/v1/snapshots/${id}`);
}

export function getSnapshotRows(
  id: number,
  opts?: { database?: string; table?: string },
): Promise<SnapshotRow[]> {
  const params = new URLSearchParams();
  if (opts?.database) params.set('database', opts.database);
  if (opts?.table) params.set('table', opts.table);
  const qs = params.toString();
  return getGlobal(`/api/v1/snapshots/${id}/rows${qs ? '?' + qs : ''}`);
}

export function deleteSnapshot(id: number): Promise<void> {
  return delGlobal(`/api/v1/snapshots/${id}`);
}

export interface LoadedSnapshot {
  cluster: string;
  label: string;
  mode: 'snapshot';
  snapshot?: {
    sourceId: number;
    createdAt?: string;
    nbAddr?: string;
    sbAddr?: string;
  };
}

// loadSnapshot / unloadSnapshot manage snapshot clusters, a global action that
// always targets the main mux. They deliberately bypass clusterPath() so they
// are not rewritten onto whichever cluster happens to be active.
export async function loadSnapshot(id: number): Promise<LoadedSnapshot> {
  const res = await apiFetch(`/api/v1/snapshots/${id}/load`, {
    method: 'POST',
  });
  if (!res.ok) throw await failed(res);
  return res.json();
}

export async function unloadSnapshot(id: number): Promise<void> {
  const res = await apiFetch(`/api/v1/snapshots/${id}/unload`, {
    method: 'POST',
  });
  if (!res.ok) throw await failed(res);
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
  return getGlobal(`/api/v1/snapshots/diff?${params.toString()}`);
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
  return getGlobal(`/api/v1/events${qs ? '?' + qs : ''}`);
}

// --- Propagation Timeline ---

export interface PropagationEvent {
  generation: number;
  nb_timestamp_ms: number;
  chassis: string;
  hostname: string;
  chassis_timestamp_ms: number;
  latency_ms: number;
  recorded_at: number;
}

export interface PropagationTimelineResponse {
  events: PropagationEvent[];
  current_generation: number;
  count: number;
}

export interface ChassisPropSummary {
  chassis: string;
  hostname: string;
  count: number;
  avg_ms: number;
  p50_ms: number;
  p95_ms: number;
  p99_ms: number;
  max_ms: number;
  min_ms: number;
}

export interface PropagationHeatmapResponse {
  chassis: ChassisPropSummary[];
  current_generation: number;
  since: number;
}

export function getPropagationTimeline(opts?: {
  chassis?: string;
  since?: number;
  limit?: number;
}): Promise<PropagationTimelineResponse> {
  const params = new URLSearchParams();
  if (opts?.chassis) params.set('chassis', opts.chassis);
  if (opts?.since) params.set('since', String(opts.since));
  if (opts?.limit) params.set('limit', String(opts.limit));
  const qs = params.toString();
  return get(`/api/v1/telemetry/propagation/timeline${qs ? '?' + qs : ''}`);
}

export function getPropagationHeatmap(opts?: {
  since?: number;
}): Promise<PropagationHeatmapResponse> {
  const params = new URLSearchParams();
  if (opts?.since) params.set('since', String(opts.since));
  const qs = params.toString();
  return get(`/api/v1/telemetry/propagation/heatmap${qs ? '?' + qs : ''}`);
}

// --- Chassis Inventory ---
// Aggregated, chassis-centric view computed from the Southbound cache: identity,
// tunnel encaps, nb_cfg-derived liveness, and per-chassis workload distribution.

export interface EncapInfo {
  type: string;
  ip: string;
}

export interface ChassisLiveness {
  in_sync: boolean;
  alive: boolean;
  // stale: out-of-sync and lagging the current nb_cfg generation beyond the
  // configured threshold. Informational; alive/down is driven by `alive`.
  stale: boolean;
  nb_cfg: number;
  sb_nb_cfg: number;
  nb_cfg_timestamp: number;
  age_ms: number;
}

export interface ChassisPortSummary {
  total: number;
  up: number;
  by_type: Record<string, number>;
}

export interface ChassisBoundPort {
  logical_port: string;
  type: string;
  up?: boolean;
  tunnel_key: number;
}

export interface ChassisInventoryEntry {
  name: string;
  hostname: string;
  // The backend marshals an empty encap slice to JSON null; normalize on read.
  encaps: EncapInfo[] | null;
  bridge_mappings?: string;
  liveness: ChassisLiveness;
  ports: ChassisPortSummary;
}

export interface ChassisInventoryDetail extends ChassisInventoryEntry {
  other_config?: Record<string, string>;
  bound_ports: ChassisBoundPort[];
}

export async function listChassisInventory(): Promise<ChassisInventoryEntry[]> {
  // The list is always a JSON array, but stay defensive against a null body.
  const data = await get<ChassisInventoryEntry[] | null>(
    '/api/v1/sb/chassis-inventory',
  );
  return data ?? [];
}

export function getChassisInventory(
  name: string,
): Promise<ChassisInventoryDetail> {
  return get(`/api/v1/sb/chassis-inventory/${encodeURIComponent(name)}`);
}
