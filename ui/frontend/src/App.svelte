<script lang="ts">
  import { location, resolveRoute } from './lib/router';
  import AppShell from './components/layout/AppShell.svelte';
  import Home from './routes/Home.svelte';
  import NotFound from './routes/NotFound.svelte';
  import TableBrowser from './routes/TableBrowser.svelte';
  import RawDetail from './routes/RawDetail.svelte';
  import SearchResults from './routes/SearchResults.svelte';
  import SwitchList from './routes/correlated/SwitchList.svelte';
  import SwitchProfile from './routes/correlated/SwitchProfile.svelte';
  import RouterList from './routes/correlated/RouterList.svelte';
  import RouterProfile from './routes/correlated/RouterProfile.svelte';
  import ChassisList from './routes/correlated/ChassisList.svelte';
  import ChassisProfile from './routes/correlated/ChassisProfile.svelte';
  import LSPProfile from './routes/correlated/LSPProfile.svelte';
  import LRPProfile from './routes/correlated/LRPProfile.svelte';
  import PortBindingProfile from './routes/correlated/PortBindingProfile.svelte';
  import Topology from './routes/Topology.svelte';
  import FlowPipeline from './routes/FlowPipeline.svelte';

  let route = $derived(resolveRoute($location));
</script>

<AppShell>
  {#if route.component === 'home'}
    <Home />
  {:else if route.component === 'topology'}
    <Topology />
  {:else if route.component === 'flow-pipeline'}
    <FlowPipeline />
  {:else if route.component === 'table-browser'}
    <TableBrowser db={route.db!} table={route.params.table} />
  {:else if route.component === 'raw-detail'}
    <RawDetail
      db={route.db!}
      table={route.params.table}
      uuid={route.params.uuid}
    />
  {:else if route.component === 'search'}
    <SearchResults query={route.query.q || ''} />
  {:else if route.component === 'switch-list'}
    <SwitchList />
  {:else if route.component === 'switch-profile'}
    <SwitchProfile uuid={route.params.uuid} />
  {:else if route.component === 'router-list'}
    <RouterList />
  {:else if route.component === 'router-profile'}
    <RouterProfile uuid={route.params.uuid} />
  {:else if route.component === 'chassis-list'}
    <ChassisList />
  {:else if route.component === 'chassis-profile'}
    <ChassisProfile uuid={route.params.uuid} />
  {:else if route.component === 'lsp-profile'}
    <LSPProfile uuid={route.params.uuid} />
  {:else if route.component === 'lrp-profile'}
    <LRPProfile uuid={route.params.uuid} />
  {:else if route.component === 'port-binding-profile'}
    <PortBindingProfile uuid={route.params.uuid} />
  {:else}
    <NotFound />
  {/if}
</AppShell>
