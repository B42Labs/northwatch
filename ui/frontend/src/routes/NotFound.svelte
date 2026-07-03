<script lang="ts">
  import { link } from '../lib/router';
</script>

<div class="flex flex-col items-center gap-3 py-20 text-center font-mono">
  <div class="text-sm text-base-content/55">
    <span class="text-primary">northwatch:~$</span> cd $PATH
  </div>
  <h1 class="text-6xl font-bold tracking-tight text-base-content">404</h1>
  <p class="text-base-content/50">
    <span class="text-base-content/30">//</span> route not found
  </p>
  <a href={link('/')} class="btn mt-1 btn-primary btn-sm">cd ~</a>
</div>
