import { writable } from 'svelte/store';

function getPath(): string {
  return window.location.hash.slice(1) || '/';
}

export const location = writable(getPath());

window.addEventListener('hashchange', () => {
  location.set(getPath());
});

export function push(path: string) {
  window.location.hash = '#' + path;
}

export function link(path: string): string {
  return '#' + path;
}

export interface RouteMatch {
  params: Record<string, string>;
  query: Record<string, string>;
}

export function matchRoute(pattern: string, path: string): RouteMatch | null {
  const [pathname, queryString] = path.split('?');
  const query: Record<string, string> = {};
  if (queryString) {
    for (const param of queryString.split('&')) {
      const [key, value] = param.split('=');
      query[decodeURIComponent(key)] = decodeURIComponent(value || '');
    }
  }

  const patternParts = pattern.split('/').filter(Boolean);
  const pathParts = pathname.split('/').filter(Boolean);

  if (patternParts.length !== pathParts.length) return null;

  const params: Record<string, string> = {};
  for (let i = 0; i < patternParts.length; i++) {
    if (patternParts[i].startsWith(':')) {
      params[patternParts[i].slice(1)] = decodeURIComponent(pathParts[i]);
    } else if (patternParts[i] !== pathParts[i]) {
      return null;
    }
  }

  return { params, query };
}

interface ResolvedRoute {
  component: string;
  params: Record<string, string>;
  query: Record<string, string>;
  db?: string;
}

export function resolveRoute(path: string): ResolvedRoute {
  const empty = { params: {}, query: {} };
  let m: RouteMatch | null;

  if (path === '/' || path === '') return { component: 'home', ...empty };

  // Visualization routes
  if ((m = matchRoute('/topology', path)))
    return { component: 'topology', ...m };
  if ((m = matchRoute('/flows', path)))
    return { component: 'flow-pipeline', ...m };
  if ((m = matchRoute('/security', path)))
    return { component: 'security-policy', ...m };
  if ((m = matchRoute('/nat', path)))
    return { component: 'nat-overview', ...m };
  if ((m = matchRoute('/ha', path))) return { component: 'ha-failover', ...m };
  if ((m = matchRoute('/mac-table', path)))
    return { component: 'mac-table', ...m };
  if ((m = matchRoute('/load-balancers', path)))
    return { component: 'load-balancers', ...m };

  // Debug routes
  if ((m = matchRoute('/debug/trace', path)))
    return { component: 'packet-trace', ...m };
  if ((m = matchRoute('/debug/flow-diff', path)))
    return { component: 'flow-diff', ...m };
  if ((m = matchRoute('/debug/connectivity', path)))
    return { component: 'connectivity-checker', ...m };
  if ((m = matchRoute('/debug/port-diagnostics', path)))
    return { component: 'port-diagnostics', ...m };
  if ((m = matchRoute('/debug/acl-audit', path)))
    return { component: 'acl-audit', ...m };
  if ((m = matchRoute('/debug/stale-entries', path)))
    return { component: 'stale-entries', ...m };

  // History & Events
  if ((m = matchRoute('/history', path))) return { component: 'history', ...m };
  if ((m = matchRoute('/events', path))) return { component: 'events', ...m };

  // Monitoring
  if ((m = matchRoute('/raft-health', path)))
    return { component: 'raft-health', ...m };
  if ((m = matchRoute('/propagation-timeline', path)))
    return { component: 'propagation-timeline', ...m };

  // Write
  if ((m = matchRoute('/write/audit/:id', path)))
    return { component: 'audit-detail', ...m };
  if ((m = matchRoute('/write/audit', path)))
    return { component: 'audit-log', ...m };
  if ((m = matchRoute('/write', path)))
    return { component: 'write-builder', ...m };

  // Search
  if ((m = matchRoute('/search', path))) return { component: 'search', ...m };

  // Correlated detail routes (must come before generic /nb/ /sb/ routes)
  if ((m = matchRoute('/correlated/logical-switches/:uuid', path)))
    return { component: 'switch-profile', ...m };
  if ((m = matchRoute('/correlated/logical-switches', path)))
    return { component: 'switch-list', ...m };
  if ((m = matchRoute('/correlated/logical-routers/:uuid', path)))
    return { component: 'router-profile', ...m };
  if ((m = matchRoute('/correlated/logical-routers', path)))
    return { component: 'router-list', ...m };
  if ((m = matchRoute('/correlated/chassis/:uuid', path)))
    return { component: 'chassis-profile', ...m };
  if ((m = matchRoute('/correlated/chassis', path)))
    return { component: 'chassis-list', ...m };
  if ((m = matchRoute('/correlated/logical-switch-ports/:uuid', path)))
    return { component: 'lsp-profile', ...m };
  if ((m = matchRoute('/correlated/logical-router-ports/:uuid', path)))
    return { component: 'lrp-profile', ...m };
  if ((m = matchRoute('/correlated/port-bindings/:uuid', path)))
    return { component: 'port-binding-profile', ...m };

  // Generic table detail
  if ((m = matchRoute('/nb/:table/:uuid', path)))
    return { component: 'raw-detail', ...m, db: 'nb' };
  if ((m = matchRoute('/sb/:table/:uuid', path)))
    return { component: 'raw-detail', ...m, db: 'sb' };

  // Generic table browser
  if ((m = matchRoute('/nb/:table', path)))
    return { component: 'table-browser', ...m, db: 'nb' };
  if ((m = matchRoute('/sb/:table', path)))
    return { component: 'table-browser', ...m, db: 'sb' };

  return { component: 'not-found', ...empty };
}
