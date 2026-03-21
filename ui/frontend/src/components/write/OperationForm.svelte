<script lang="ts">
  import { writableTables } from '../../lib/writableTables';
  import type { WriteOperation } from '../../lib/writeApi';
  import EntityPicker from './EntityPicker.svelte';
  import FieldsEditor from './FieldsEditor.svelte';

  let {
    onAdd,
    initialAction,
    initialTable,
    initialUuid,
  }: {
    onAdd: (op: WriteOperation) => void;
    initialAction?: string;
    initialTable?: string;
    initialUuid?: string;
  } = $props();

  let action = $state<'create' | 'update' | 'delete'>('update');
  let table = $state('');
  let uuid = $state('');
  let initialized = false;

  $effect(() => {
    if (!initialized) {
      if (initialAction)
        action = initialAction as 'create' | 'update' | 'delete';
      if (initialTable) table = initialTable;
      if (initialUuid) uuid = initialUuid;
      initialized = true;
    }
  });
  let fieldsJson = $state('{\n  \n}');
  let reason = $state('');
  let jsonError = $state('');

  let selectedTable = $derived(
    writableTables.find((t) => t.ovsdbName === table),
  );
  let needsUuid = $derived(action === 'update' || action === 'delete');
  let needsFields = $derived(action === 'create' || action === 'update');

  function validate(): string | null {
    if (!table) return 'Select a table';
    if (needsUuid && !uuid) return 'UUID is required for ' + action;
    if (needsFields) {
      try {
        const parsed = JSON.parse(fieldsJson);
        if (
          typeof parsed !== 'object' ||
          parsed === null ||
          Array.isArray(parsed)
        )
          return 'Fields must be a JSON object';
        if (Object.keys(parsed).length === 0)
          return 'At least one field is required';
      } catch {
        return 'Invalid JSON — switch to JSON mode to check syntax';
      }
    }
    return null;
  }

  function handleAdd() {
    const err = validate();
    if (err) {
      jsonError = err;
      return;
    }
    jsonError = '';

    const op: WriteOperation = { action, table };
    if (needsUuid) op.uuid = uuid;
    if (needsFields) op.fields = JSON.parse(fieldsJson);
    if (reason) op.reason = reason;

    onAdd(op);

    // Reset form (keep table selection)
    uuid = '';
    fieldsJson = '{\n  \n}';
    reason = '';
  }
</script>

<div class="card bg-base-100 shadow-sm">
  <div class="card-body gap-3 p-4">
    <h3 class="card-title text-sm">Add Operation</h3>

    <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
      <!-- Action -->
      <div class="form-control">
        <label class="label py-0.5" for="op-action">
          <span class="label-text text-xs">Action</span>
        </label>
        <select
          id="op-action"
          class="select select-bordered select-sm"
          bind:value={action}
        >
          <option value="create">Create</option>
          <option value="update">Update</option>
          <option value="delete">Delete</option>
        </select>
      </div>

      <!-- Table -->
      <div class="form-control">
        <label class="label py-0.5" for="op-table">
          <span class="label-text text-xs">Table</span>
        </label>
        <select
          id="op-table"
          class="select select-bordered select-sm"
          bind:value={table}
        >
          <option value="">-- select table --</option>
          {#each writableTables as t}
            <option value={t.ovsdbName}>{t.label} ({t.ovsdbName})</option>
          {/each}
        </select>
      </div>
    </div>

    <!-- UUID (update/delete) -->
    {#if needsUuid && selectedTable}
      <div class="form-control">
        <span class="label py-0.5">
          <span class="label-text text-xs">Entity UUID</span>
        </span>
        <EntityPicker
          tableSlug={selectedTable.slug}
          value={uuid}
          onSelect={(v) => (uuid = v)}
        />
      </div>
    {/if}

    <!-- Fields (create/update) -->
    {#if needsFields}
      <div class="form-control">
        <span class="label py-0.5">
          <span class="label-text text-xs">Fields</span>
        </span>
        <FieldsEditor
          value={fieldsJson}
          onChange={(v) => (fieldsJson = v)}
          {action}
          tableName={table}
          tableSlug={selectedTable?.slug}
          {uuid}
        />
      </div>
    {/if}

    <!-- Reason -->
    <div class="form-control">
      <label class="label py-0.5" for="op-reason">
        <span class="label-text text-xs">Reason (optional)</span>
      </label>
      <input
        id="op-reason"
        type="text"
        class="input input-sm input-bordered"
        placeholder="Why this change?"
        bind:value={reason}
      />
    </div>

    {#if jsonError}
      <div role="alert" class="alert alert-warning py-2 text-xs">
        {jsonError}
      </div>
    {/if}

    <button class="btn btn-primary btn-sm self-start" onclick={handleAdd}>
      Add to Batch
    </button>
  </div>
</div>
