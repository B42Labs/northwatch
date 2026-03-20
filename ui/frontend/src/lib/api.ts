export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path);
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
export function getCapabilities(): Promise<string[]> {
  return get('/api/v1/capabilities');
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
}

export interface FlowTableGroup {
  table_id: number;
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
