import { describe, it, expect } from 'vitest';
import { matchRoute, resolveRoute } from './router';

describe('matchRoute', () => {
  it('matches a simple path', () => {
    const result = matchRoute('/nb/:table', '/nb/logical-switches');
    expect(result).toEqual({
      params: { table: 'logical-switches' },
      query: {},
    });
  });

  it('matches multiple params', () => {
    const result = matchRoute(
      '/nb/:table/:uuid',
      '/nb/logical-switches/abc-123',
    );
    expect(result).toEqual({
      params: { table: 'logical-switches', uuid: 'abc-123' },
      query: {},
    });
  });

  it('returns null on segment count mismatch', () => {
    expect(matchRoute('/nb/:table', '/nb/logical-switches/extra')).toBeNull();
    expect(matchRoute('/nb/:table/:uuid', '/nb/logical-switches')).toBeNull();
  });

  it('returns null on literal segment mismatch', () => {
    expect(matchRoute('/nb/:table', '/sb/logical-switches')).toBeNull();
  });

  it('parses query parameters', () => {
    const result = matchRoute('/search', '/search?q=10.0.0.1&type=ip');
    expect(result).toEqual({
      params: {},
      query: { q: '10.0.0.1', type: 'ip' },
    });
  });

  it('decodes URI-encoded path segments', () => {
    const result = matchRoute('/nb/:table', '/nb/logical%2Dswitches');
    expect(result).toEqual({
      params: { table: 'logical-switches' },
      query: {},
    });
  });

  it('decodes URI-encoded query values', () => {
    const result = matchRoute('/search', '/search?q=hello%20world');
    expect(result).toEqual({ params: {}, query: { q: 'hello world' } });
  });

  it('handles empty query value', () => {
    const result = matchRoute('/search', '/search?q=');
    expect(result).toEqual({ params: {}, query: { q: '' } });
  });
});

describe('resolveRoute', () => {
  it('resolves root to home', () => {
    expect(resolveRoute('/')).toMatchObject({ component: 'home' });
    expect(resolveRoute('')).toMatchObject({ component: 'home' });
  });

  it('resolves search', () => {
    const route = resolveRoute('/search?q=test');
    expect(route.component).toBe('search');
    expect(route.query.q).toBe('test');
  });

  it('resolves correlated switch list', () => {
    expect(resolveRoute('/correlated/logical-switches')).toMatchObject({
      component: 'switch-list',
    });
  });

  it('resolves correlated switch profile', () => {
    const route = resolveRoute('/correlated/logical-switches/some-uuid');
    expect(route.component).toBe('switch-profile');
    expect(route.params.uuid).toBe('some-uuid');
  });

  it('resolves correlated router list and profile', () => {
    expect(resolveRoute('/correlated/logical-routers')).toMatchObject({
      component: 'router-list',
    });
    expect(resolveRoute('/correlated/logical-routers/uuid-1')).toMatchObject({
      component: 'router-profile',
      params: { uuid: 'uuid-1' },
    });
  });

  it('resolves correlated chassis list and profile', () => {
    expect(resolveRoute('/correlated/chassis')).toMatchObject({
      component: 'chassis-list',
    });
    expect(resolveRoute('/correlated/chassis/uuid-1')).toMatchObject({
      component: 'chassis-profile',
      params: { uuid: 'uuid-1' },
    });
  });

  it('resolves correlated port profiles', () => {
    expect(resolveRoute('/correlated/logical-switch-ports/u1')).toMatchObject({
      component: 'lsp-profile',
    });
    expect(resolveRoute('/correlated/logical-router-ports/u1')).toMatchObject({
      component: 'lrp-profile',
    });
    expect(resolveRoute('/correlated/port-bindings/u1')).toMatchObject({
      component: 'port-binding-profile',
    });
  });

  it('resolves generic NB table browser', () => {
    const route = resolveRoute('/nb/acls');
    expect(route.component).toBe('table-browser');
    expect(route.params.table).toBe('acls');
    expect(route.db).toBe('nb');
  });

  it('resolves generic SB table detail', () => {
    const route = resolveRoute('/sb/chassis/some-uuid');
    expect(route.component).toBe('raw-detail');
    expect(route.params).toEqual({ table: 'chassis', uuid: 'some-uuid' });
    expect(route.db).toBe('sb');
  });

  it('resolves correlated routes before generic routes', () => {
    // /correlated/logical-switches/uuid should NOT match /nb/:table/:uuid
    const route = resolveRoute('/correlated/logical-switches/uuid-1');
    expect(route.component).toBe('switch-profile');
  });

  it('resolves write builder', () => {
    const route = resolveRoute('/write');
    expect(route.component).toBe('write-builder');
  });

  it('resolves write builder with query params', () => {
    const route = resolveRoute('/write?action=update&table=Logical_Switch');
    expect(route.component).toBe('write-builder');
    expect(route.query.action).toBe('update');
    expect(route.query.table).toBe('Logical_Switch');
  });

  it('resolves audit log', () => {
    expect(resolveRoute('/write/audit')).toMatchObject({
      component: 'audit-log',
    });
  });

  it('resolves audit detail', () => {
    const route = resolveRoute('/write/audit/42');
    expect(route.component).toBe('audit-detail');
    expect(route.params.id).toBe('42');
  });

  it('returns not-found for unknown paths', () => {
    expect(resolveRoute('/unknown/path/here')).toMatchObject({
      component: 'not-found',
    });
  });
});
