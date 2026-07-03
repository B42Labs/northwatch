<script lang="ts">
  import { badgeClass, type Variant } from '../../lib/status';

  let {
    text,
    variant = 'neutral',
    glyph,
    outline = false,
  }: {
    text: string;
    variant?: Variant;
    glyph?: string;
    outline?: boolean;
  } = $props();
</script>

<span
  class="badge gap-1 font-mono text-2xs badge-sm whitespace-nowrap {badgeClass[
    variant
  ]} {outline ? 'badge-outline' : ''}"
>
  {#if glyph}<span class="font-bold" aria-hidden="true">{glyph}</span
    >{/if}{text}
</span>
