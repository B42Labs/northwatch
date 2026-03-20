<script lang="ts">
  import { link } from '../../lib/router';

  let {
    uuid,
    href = '',
    short = false,
  }: { uuid: string; href?: string; short?: boolean } = $props();

  let displayText = $derived(short ? uuid.slice(0, 8) : uuid);
  let resolvedHref = $derived(href ? link(href) : link(`/search?q=${uuid}`));
</script>

<a href={resolvedHref} class="link link-primary font-mono text-sm" title={uuid}>
  {displayText}
</a>
