<script lang="ts">
  import { apiToken, setApiToken, clearApiToken } from '../../lib/authStore';

  // Mutating endpoints require a bearer token, so without one the write, HA,
  // snapshot and silence flows all answer 401. This lets an operator paste the
  // token once instead of proxying every call by hand.
  let open = $state(false);
  let draft = $state('');

  function toggle() {
    open = !open;
    if (open) draft = $apiToken;
  }

  function save() {
    setApiToken(draft);
    open = false;
  }

  function clear() {
    clearApiToken();
    draft = '';
    open = false;
  }
</script>

<div class="relative">
  <button
    type="button"
    class="btn btn-ghost font-mono btn-sm"
    onclick={toggle}
    title={$apiToken
      ? 'API token set — mutating requests are authorized'
      : 'No API token — mutating requests will be rejected'}
    aria-label="API token"
  >
    <span class={$apiToken ? 'text-success' : 'text-base-content/40'}>◆</span>
    <span class="hidden text-2xs sm:inline">token</span>
  </button>

  {#if open}
    <div
      class="absolute top-full right-0 z-30 mt-1 w-72 rounded border border-base-300 bg-base-100 p-3 shadow-lg"
    >
      <label
        class="font-mono text-2xs tracking-wider text-base-content/60 uppercase"
        for="api-token-input">API token</label
      >
      <input
        id="api-token-input"
        type="password"
        class="input mt-1 w-full font-mono input-sm"
        placeholder="paste your token"
        bind:value={draft}
        onkeydown={(e) => e.key === 'Enter' && save()}
      />
      <p class="mt-1 font-prose text-2xs text-base-content/40">
        Required for write, HA, snapshot and silence actions. Stored in this
        browser.
      </p>
      <div class="mt-2 flex gap-2">
        <button class="btn font-mono btn-primary btn-xs" onclick={save}
          >Save</button
        >
        <button
          class="btn border-base-300 btn-ghost font-mono btn-xs"
          onclick={clear}
          disabled={!$apiToken}>Clear</button
        >
      </div>
    </div>
  {/if}
</div>
