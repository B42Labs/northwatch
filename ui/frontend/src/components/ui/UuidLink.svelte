<script lang="ts">
  import { link } from '../../lib/router';

  let {
    uuid,
    href = '',
    short = false,
    label,
  }: {
    uuid: string;
    href?: string;
    short?: boolean;
    label?: string;
  } = $props();

  // A resolved reference label (e.g. the target row's name) replaces the raw
  // UUID as the link text; the full UUID stays in the title for hovering.
  let displayText = $derived(label ? label : short ? uuid.slice(0, 8) : uuid);
  let resolvedHref = $derived(href ? link(href) : link(`/search?q=${uuid}`));
</script>

<a href={resolvedHref} class="link font-mono text-sm link-primary" title={uuid}>
  {displayText}
</a>
