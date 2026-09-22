<script lang="ts">
  import CopyButton from './CopyButton.svelte';
  import {
    maskEnabled,
    maskSensitive,
    renderJSON,
    renderMarkdown,
    viewContext,
  } from '../../lib/copyView';

  // Copies the whole view for pasting into an external AI assistant: Markdown
  // with a context header for a chat, raw JSON for a tool.
  let {
    title,
    view,
    disabled = false,
  }: {
    title: string;
    /** Returns the object to copy. Called at click time, never earlier. */
    view: () => unknown;
    disabled?: boolean;
  } = $props();

  function payload(): unknown {
    const raw = view();
    return $maskEnabled ? maskSensitive(raw) : raw;
  }

  function asMarkdown(): string {
    return renderMarkdown(viewContext(title, $maskEnabled), payload());
  }

  function asJSON(): string {
    return renderJSON(payload());
  }
</script>

<label
  class="flex cursor-pointer items-center gap-1 font-mono text-2xs tracking-wider text-base-content/50 uppercase"
  title="Replace IP, MAC and hostname values with placeholders before copying"
>
  <input
    type="checkbox"
    class="checkbox checkbox-xs"
    bind:checked={$maskEnabled}
  />
  Mask
</label>
<CopyButton
  text={asMarkdown}
  label="Copy MD"
  ariaLabel="Copy view as Markdown"
  {disabled}
/>
<CopyButton
  text={asJSON}
  label="Copy JSON"
  ariaLabel="Copy view as JSON"
  {disabled}
/>
