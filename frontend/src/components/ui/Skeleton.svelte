<script lang="ts">
  let { width = '100%', height = '12px', radius = 'var(--r-sm)', circle = false }: { width?: string; height?: string; radius?: string; circle?: boolean } = $props()
</script>

<span class="sk" aria-hidden="true" style:width style:height style:border-radius={circle ? '50%' : radius}></span>

<style>
  .sk {
    display: block; flex: none;
    background: linear-gradient(90deg, var(--surface-3) 0%, var(--surface-2) 40%, var(--surface-3) 80%);
    background-size: 200% 100%;
    animation: shimmer 1.6s linear infinite;
  }
</style>
