import { describe, it, expect, vi, afterEach } from 'vitest';
import { ApiError } from './api';
import {
  requestFailover,
  requestEvacuate,
  requestRestore,
  previewOperations,
  dryRunOperations,
  applyPlan,
  cancelPlan,
  getPlan,
  getImpact,
  listAuditEntries,
  getTableSchema,
  type WriteOperation,
  type TableSchema,
} from './writeApi';

// The write helpers go through api.ts's get/post/del, which run the path
// through clusterPath(). With no active cluster selected (the default) that is
// the identity, so fetch is called with the bare path asserted below.

const JSON_POST = { 'Content-Type': 'application/json' };

function okFetch(body: unknown) {
  const spy = vi.fn(async () => ({ ok: true, json: async () => body }));
  vi.stubGlobal('fetch', spy);
  return spy;
}

function errFetch(status: number, statusText: string, body: unknown) {
  const spy = vi.fn(async () => ({
    ok: false,
    status,
    statusText,
    json: async () => body,
  }));
  vi.stubGlobal('fetch', spy);
  return spy;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('requestFailover', () => {
  it('POSTs the group and target chassis and returns the plan', async () => {
    const plan = { id: 'p1', status: 'pending' };
    const spy = okFetch(plan);

    const result = await requestFailover({
      group_name: 'gw1',
      target_chassis: 'node-b',
    });

    expect(spy).toHaveBeenCalledWith('/api/v1/write/failover', {
      method: 'POST',
      headers: JSON_POST,
      body: JSON.stringify({ group_name: 'gw1', target_chassis: 'node-b' }),
    });
    expect(result).toEqual(plan);
  });

  it('propagates a non-ok response as an ApiError', async () => {
    errFetch(409, 'Conflict', { error: 'group is being modified' });

    const err = await requestFailover({
      group_name: 'gw1',
      target_chassis: 'node-b',
    }).catch((e) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      status: 409,
      message: 'group is being modified',
    });
  });
});

describe('requestEvacuate', () => {
  it('POSTs the chassis name', async () => {
    const spy = okFetch({ id: 'p2' });

    await requestEvacuate({ chassis_name: 'node-a' });

    expect(spy).toHaveBeenCalledWith('/api/v1/write/evacuate', {
      method: 'POST',
      headers: JSON_POST,
      body: JSON.stringify({ chassis_name: 'node-a' }),
    });
  });

  it('falls back to statusText when the error body has no message', async () => {
    errFetch(500, 'Internal Server Error', {});

    const err = await requestEvacuate({ chassis_name: 'node-a' }).catch(
      (e) => e,
    );

    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      status: 500,
      message: 'Internal Server Error',
    });
  });
});

describe('requestRestore', () => {
  it('POSTs the chassis name', async () => {
    const spy = okFetch({ id: 'p3' });

    await requestRestore({ chassis_name: 'node-c' });

    expect(spy).toHaveBeenCalledWith('/api/v1/write/restore', {
      method: 'POST',
      headers: JSON_POST,
      body: JSON.stringify({ chassis_name: 'node-c' }),
    });
  });

  it('propagates a non-ok response as an ApiError', async () => {
    errFetch(404, 'Not Found', { error: 'chassis not drained' });

    const err = await requestRestore({ chassis_name: 'node-c' }).catch(
      (e) => e,
    );

    expect(err).toMatchObject({ status: 404, message: 'chassis not drained' });
  });
});

describe('previewOperations / dryRunOperations', () => {
  const ops: WriteOperation[] = [
    {
      action: 'update',
      table: 'HA_Chassis',
      uuid: 'u1',
      fields: { priority: 0 },
    },
  ];

  it('previews to /write/preview with operations and reason', async () => {
    const spy = okFetch({ id: 'plan-1', status: 'pending' });

    await previewOperations(ops, 'drain node-a');

    expect(spy).toHaveBeenCalledWith('/api/v1/write/preview', {
      method: 'POST',
      headers: JSON_POST,
      body: JSON.stringify({ operations: ops, reason: 'drain node-a' }),
    });
  });

  it('dry-runs to /write/dry-run and omits an undefined reason', async () => {
    const spy = okFetch({ id: 'plan-2', status: 'dry-run' });

    await dryRunOperations(ops);

    expect(spy).toHaveBeenCalledWith('/api/v1/write/dry-run', {
      method: 'POST',
      headers: JSON_POST,
      body: JSON.stringify({ operations: ops, reason: undefined }),
    });
  });

  it('propagates a validation error from preview', async () => {
    errFetch(400, 'Bad Request', { error: 'unknown table' });

    const err = await previewOperations(ops).catch((e) => e);

    expect(err).toMatchObject({ status: 400, message: 'unknown table' });
  });
});

describe('applyPlan', () => {
  it('forwards the apply_token and actor in the body', async () => {
    const spy = okFetch({ id: 1, result: 'success' });

    const result = await applyPlan('plan-1', 'tok-abc', 'alice');

    expect(spy).toHaveBeenCalledWith('/api/v1/write/plans/plan-1/apply', {
      method: 'POST',
      headers: JSON_POST,
      body: JSON.stringify({ apply_token: 'tok-abc', actor: 'alice' }),
    });
    expect(result).toEqual({ id: 1, result: 'success' });
  });

  it('omits an undefined actor while still sending the apply_token', async () => {
    const spy = okFetch({ id: 2, result: 'success' });

    await applyPlan('plan-2', 'tok-xyz');

    expect(spy).toHaveBeenCalledWith('/api/v1/write/plans/plan-2/apply', {
      method: 'POST',
      headers: JSON_POST,
      body: JSON.stringify({ apply_token: 'tok-xyz' }),
    });
  });

  it('propagates an invalid-token rejection as an ApiError', async () => {
    errFetch(403, 'Forbidden', { error: 'apply token mismatch' });

    const err = await applyPlan('plan-1', 'wrong', 'alice').catch((e) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 403, message: 'apply token mismatch' });
  });
});

describe('cancelPlan', () => {
  it('DELETEs the plan and resolves to void', async () => {
    const spy = okFetch(undefined);

    await expect(cancelPlan('plan-9')).resolves.toBeUndefined();
    expect(spy).toHaveBeenCalledWith('/api/v1/write/plans/plan-9', {
      method: 'DELETE',
    });
  });

  it('propagates a non-ok delete as an ApiError', async () => {
    errFetch(404, 'Not Found', { error: 'plan not found' });

    const err = await cancelPlan('missing').catch((e) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 404, message: 'plan not found' });
  });
});

describe('getPlan', () => {
  it('GETs the plan by id', async () => {
    const spy = okFetch({ id: 'plan-1', status: 'pending' });

    const result = await getPlan('plan-1');

    expect(spy).toHaveBeenCalledWith('/api/v1/write/plans/plan-1');
    expect(result).toMatchObject({ id: 'plan-1' });
  });

  it('propagates a not-found plan as an ApiError', async () => {
    errFetch(404, 'Not Found', { error: 'expired' });
    const err = await getPlan('gone').catch((e) => e);
    expect(err).toMatchObject({ status: 404, message: 'expired' });
  });
});

describe('getImpact', () => {
  it('GETs the impact for a db/table/uuid triple', async () => {
    const spy = okFetch({ root: {}, summary: { total_affected: 3 } });

    await getImpact('nb', 'logical_switch', 'u-1');

    expect(spy).toHaveBeenCalledWith('/api/v1/impact/nb/logical_switch/u-1');
  });

  it('propagates a server error as an ApiError', async () => {
    errFetch(500, 'Internal Server Error', { error: 'graph too large' });
    const err = await getImpact('nb', 'logical_switch', 'u-1').catch((e) => e);
    expect(err).toMatchObject({ status: 500, message: 'graph too large' });
  });
});

describe('listAuditEntries', () => {
  it('appends the limit query when provided and omits it otherwise', async () => {
    const withLimit = okFetch([]);
    await listAuditEntries(25);
    expect(withLimit).toHaveBeenCalledWith('/api/v1/write/audit?limit=25');

    vi.unstubAllGlobals();
    const noLimit = okFetch([]);
    await listAuditEntries();
    expect(noLimit).toHaveBeenCalledWith('/api/v1/write/audit');
  });

  it('propagates a server error as an ApiError', async () => {
    errFetch(503, 'Service Unavailable', { error: 'audit store offline' });
    const err = await listAuditEntries().catch((e) => e);
    expect(err).toMatchObject({ status: 503, message: 'audit store offline' });
  });
});

describe('getTableSchema', () => {
  const schemas: TableSchema[] = [
    { table: 'HA_Chassis', fields: [] },
    { table: 'Logical_Switch', fields: [] },
  ];

  it('finds a table schema by name', () => {
    expect(getTableSchema('Logical_Switch', schemas)).toBe(schemas[1]);
  });

  it('returns undefined for an unknown table', () => {
    expect(getTableSchema('Missing', schemas)).toBeUndefined();
  });
});
