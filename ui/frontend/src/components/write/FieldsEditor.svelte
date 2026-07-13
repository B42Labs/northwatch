<script lang="ts">
  import { getEntity } from '../../lib/api';
  import {
    getWriteSchema,
    getTableSchema,
    type TableSchema,
  } from '../../lib/writeApi';
  import SegmentedControl from '../ui/SegmentedControl.svelte';

  let {
    value,
    onChange,
    action = 'create',
    tableName,
    tableSlug,
    uuid,
  }: {
    value: string;
    onChange: (json: string) => void;
    action?: string;
    tableName?: string;
    tableSlug?: string;
    uuid?: string;
  } = $props();

  interface FieldRow {
    key: string;
    value: string;
    type?: string;
    original?: string;
  }

  let mode: 'form' | 'json' = $state('form');
  let rows: FieldRow[] = $state([{ key: '', value: '' }]);
  let entityFields = $state<Record<string, unknown> | null>(null);
  let loadingEntity = $state(false);
  let schemas: TableSchema[] = $state([]);
  let currentSchema = $state<TableSchema | undefined>(undefined);

  // Load schema once
  $effect(() => {
    loadSchema();
  });

  async function loadSchema() {
    try {
      schemas = await getWriteSchema();
    } catch {
      schemas = [];
    }
  }

  // Update current schema when table changes
  $effect(() => {
    if (tableName && schemas.length > 0) {
      currentSchema = getTableSchema(tableName, schemas);
    } else {
      currentSchema = undefined;
    }
  });

  function defaultForType(type: string): string {
    if (type.startsWith('map<')) return '{}';
    if (type.startsWith('set<')) return '[]';
    if (type === 'boolean') return 'false';
    if (type === 'integer') return '0';
    return '';
  }

  function isMultiline(row: FieldRow): boolean {
    const t = row.type || '';
    if (t.startsWith('map<') || t.startsWith('set<')) return true;
    const v = row.value.trim();
    return (
      (v.startsWith('{') && v.endsWith('}')) ||
      (v.startsWith('[') && v.endsWith(']'))
    );
  }

  function loadSchemaFields() {
    if (!currentSchema) return;
    const newRows: FieldRow[] = [];
    for (const field of currentSchema.fields) {
      if (field.read_only) continue;
      newRows.push({
        key: field.name,
        value: defaultForType(field.type),
        type: field.type,
      });
    }
    rows = newRows.length > 0 ? newRows : [{ key: '', value: '' }];
    syncToParent();
  }

  // Parse JSON into rows
  function jsonToRows(json: string): FieldRow[] {
    try {
      const parsed = JSON.parse(json);
      if (
        typeof parsed !== 'object' ||
        parsed === null ||
        Array.isArray(parsed)
      )
        return [{ key: '', value: '' }];
      const entries = Object.entries(parsed);
      if (entries.length === 0) return [{ key: '', value: '' }];
      return entries.map(([k, v]) => ({
        key: k,
        value: typeof v === 'string' ? v : JSON.stringify(v),
        type: fieldType(k),
      }));
    } catch {
      return [{ key: '', value: '' }];
    }
  }

  function fieldType(name: string): string | undefined {
    return currentSchema?.fields.find((f) => f.name === name)?.type;
  }

  // Convert rows to JSON and notify parent
  function syncToParent() {
    const obj: Record<string, unknown> = {};
    for (const row of rows) {
      if (!row.key.trim()) continue;
      obj[row.key.trim()] = parseValue(row.value);
    }
    const json = JSON.stringify(obj, null, 2);
    onChange(json);
  }

  function parseValue(v: string): unknown {
    const trimmed = v.trim();
    if (trimmed === '') return '';
    if (trimmed === 'true') return true;
    if (trimmed === 'false') return false;
    if (trimmed === 'null') return null;
    if (/^-?\d+$/.test(trimmed)) return parseInt(trimmed, 10);
    if (/^-?\d+\.\d+$/.test(trimmed)) return parseFloat(trimmed);
    if (
      (trimmed.startsWith('{') && trimmed.endsWith('}')) ||
      (trimmed.startsWith('[') && trimmed.endsWith(']'))
    ) {
      try {
        return JSON.parse(trimmed);
      } catch {
        return trimmed;
      }
    }
    return trimmed;
  }

  function formatValue(v: unknown): string {
    if (v === null || v === undefined) return '';
    if (typeof v === 'string') return v;
    if (typeof v === 'object') return JSON.stringify(v, null, 2);
    return JSON.stringify(v);
  }

  function addRow() {
    rows = [...rows, { key: '', value: '' }];
  }

  function removeRow(index: number) {
    rows = rows.filter((_, i) => i !== index);
    if (rows.length === 0) rows = [{ key: '', value: '' }];
    syncToParent();
  }

  function handleRowChange() {
    syncToParent();
  }

  function switchMode(newMode: string) {
    const next = newMode as 'form' | 'json';
    if (next === mode) return;
    if (next === 'form') {
      rows = jsonToRows(value);
    } else {
      syncToParent();
    }
    mode = next;
  }

  // Load entity fields for update mode
  $effect(() => {
    if (action === 'update' && tableSlug && uuid && uuid.length > 8) {
      loadEntity(tableSlug, uuid);
    } else {
      entityFields = null;
    }
  });

  async function loadEntity(slug: string, entityUuid: string) {
    loadingEntity = true;
    try {
      const data = await getEntity('nb', slug, entityUuid);
      entityFields = data;
    } catch {
      entityFields = null;
    } finally {
      loadingEntity = false;
    }
  }

  function loadFieldsFromEntity() {
    if (!entityFields) return;
    const newRows: FieldRow[] = [];
    for (const [k, v] of Object.entries(entityFields)) {
      if (k === '_uuid') continue;
      const formatted = formatValue(v);
      newRows.push({
        key: k,
        value: formatted,
        type: fieldType(k),
        original: formatted,
      });
    }
    newRows.sort((a, b) => a.key.localeCompare(b.key));
    rows = newRows.length > 0 ? newRows : [{ key: '', value: '' }];
    syncToParent();
  }

  let writableSchemaFields = $derived(
    currentSchema?.fields.filter((f) => !f.read_only) ?? [],
  );
</script>

<div class="flex flex-col gap-2">
  <!-- Mode toggle + actions -->
  <div class="flex items-center justify-between">
    <SegmentedControl
      options={[
        { value: 'form', label: 'Form' },
        { value: 'json', label: 'JSON' },
      ]}
      value={mode}
      onchange={switchMode}
      size="xs"
    />

    <div class="flex gap-1">
      {#if mode === 'form' && currentSchema}
        <button
          class="btn border-base-300 btn-ghost font-mono btn-xs"
          onclick={loadSchemaFields}
          title="Pre-fill all writable fields from the table schema"
        >
          Load Schema Fields
        </button>
      {/if}
      {#if mode === 'form' && action === 'update' && entityFields}
        <button
          class="btn border-base-300 btn-ghost font-mono btn-xs"
          onclick={loadFieldsFromEntity}
          title="Load current field values from entity"
        >
          Load Current Values
        </button>
      {/if}
    </div>
  </div>

  {#if loadingEntity}
    <span class="font-mono text-xs text-base-content/50">Loading entity...</span
    >
  {/if}

  {#if mode === 'form'}
    <!-- Structured field rows -->
    <table class="w-full border-separate border-spacing-y-1">
      <tbody>
        {#each rows as row, i (i)}
          <tr>
            <td style="width: 11rem; min-width: 11rem" class="align-top">
              {#if writableSchemaFields.length > 0}
                <select
                  class="select w-full bg-base-200/60 font-mono select-xs"
                  bind:value={row.key}
                  onchange={handleRowChange}
                >
                  <option value="">-- field --</option>
                  {#each writableSchemaFields as f (f.name)}
                    <option value={f.name}>{f.name}</option>
                  {/each}
                </select>
              {:else}
                <input
                  type="text"
                  class="input w-full font-mono input-xs"
                  placeholder="field_name"
                  bind:value={row.key}
                  oninput={handleRowChange}
                />
              {/if}
            </td>
            <td class="w-full align-top">
              {#if isMultiline(row)}
                <textarea
                  class="textarea w-full font-mono text-xs leading-relaxed textarea-xs"
                  rows="3"
                  placeholder={row.type ? `(${row.type})` : 'value'}
                  bind:value={row.value}
                  oninput={handleRowChange}></textarea>
              {:else}
                <input
                  type="text"
                  class="input w-full font-mono input-xs"
                  placeholder={row.type ? `(${row.type})` : 'value'}
                  bind:value={row.value}
                  oninput={handleRowChange}
                />
              {/if}
            </td>
            <td class="pl-1 align-top whitespace-nowrap">
              {#if row.original !== undefined && row.value !== row.original}
                <span class="badge badge-xs badge-warning" title="modified"
                  >M</span
                >
              {/if}
              {#if row.type}
                <span class="font-mono text-xs text-base-content/30"
                  >{row.type}</span
                >
              {/if}
            </td>
            <td class="pl-0.5 align-top">
              <button
                class="btn min-h-0 btn-ghost text-error btn-xs"
                onclick={() => removeRow(i)}
                title="Remove field"
              >
                &times;
              </button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>

    <button
      class="btn self-start border-base-300 btn-ghost font-mono btn-xs"
      onclick={addRow}
    >
      + Add Field
    </button>

    {#if action === 'update' && rows.some((r) => r.original !== undefined)}
      <p class="font-prose text-xs text-base-content/40">
        Only modified fields will be sent. Values are auto-parsed (JSON
        objects/arrays, numbers, booleans).
      </p>
    {/if}
  {:else}
    <!-- JSON textarea -->
    <textarea
      class="textarea font-mono text-xs"
      rows="6"
      placeholder={'{"name": "my-switch", "external_ids": {"owner": "admin"}}'}
      {value}
      oninput={(e) => onChange(e.currentTarget.value)}></textarea>
  {/if}
</div>
