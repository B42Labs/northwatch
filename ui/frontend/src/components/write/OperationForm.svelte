<script lang="ts">
  import { writableTables } from '../../lib/writableTables';
  import type { WriteOperation } from '../../lib/writeApi';
  import Card from '../ui/Card.svelte';
  import FormField from '../ui/FormField.svelte';
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

  // Force action to 'delete' when a delete-only table is selected.
  $effect(() => {
    if (selectedTable?.deleteOnly && action !== 'delete') {
      action = 'delete';
    }
  });

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

<Card title="Add Operation">
  <div class="flex flex-col gap-3">
    <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
      <!-- Action -->
      <FormField label="Action" forId="op-action">
        <select
          id="op-action"
          class="select bg-base-200/60 font-mono select-sm"
          bind:value={action}
          disabled={selectedTable?.deleteOnly}
        >
          {#if selectedTable?.deleteOnly}
            <option value="delete">Delete</option>
          {:else}
            <option value="create">Create</option>
            <option value="update">Update</option>
            <option value="delete">Delete</option>
          {/if}
        </select>
      </FormField>

      <!-- Table -->
      <FormField label="Table" forId="op-table">
        <select
          id="op-table"
          class="select bg-base-200/60 font-mono select-sm"
          bind:value={table}
        >
          <option value="">-- select table --</option>
          {#each writableTables as t (t.ovsdbName)}
            <option value={t.ovsdbName}>{t.label} ({t.ovsdbName})</option>
          {/each}
        </select>
      </FormField>
    </div>

    <!-- UUID (update/delete) -->
    {#if needsUuid && selectedTable}
      <FormField label="Entity UUID">
        <EntityPicker
          tableSlug={selectedTable.slug}
          value={uuid}
          onSelect={(v) => (uuid = v)}
        />
      </FormField>
    {/if}

    <!-- Fields (create/update) -->
    {#if needsFields}
      <FormField label="Fields">
        <FieldsEditor
          value={fieldsJson}
          onChange={(v) => (fieldsJson = v)}
          {action}
          tableName={table}
          tableSlug={selectedTable?.slug}
          {uuid}
        />
      </FormField>
    {/if}

    <!-- Reason -->
    <FormField label="Reason (optional)" forId="op-reason">
      <input
        id="op-reason"
        type="text"
        class="input font-mono input-sm"
        placeholder="Why this change?"
        bind:value={reason}
      />
    </FormField>

    {#if jsonError}
      <div
        role="alert"
        class="rounded border border-warning/40 bg-warning/10 px-3 py-2 font-mono text-sm text-warning"
      >
        {jsonError}
      </div>
    {/if}

    <button
      class="btn self-start font-mono btn-primary btn-sm"
      onclick={handleAdd}
    >
      Add to Batch
    </button>
  </div>
</Card>
