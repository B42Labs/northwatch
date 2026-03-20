<script lang="ts">
  let { text, variant = 'neutral' }: { text: string; variant?: string } =
    $props();

  const classes: Record<string, string> = {
    neutral: 'badge-neutral',
    primary: 'badge-primary',
    success: 'badge-success',
    warning: 'badge-warning',
    error: 'badge-error',
    info: 'badge-info',
    ghost: 'badge-ghost',
  };
</script>

<span class="badge badge-sm {classes[variant] || 'badge-neutral'}">{text}</span>
