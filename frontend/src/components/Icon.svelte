<script lang="ts">
  let { name, size = 16 }: { name: string; size?: number } = $props()

  const paths: Record<string, string> = {
    plus: 'M12 5v14M5 12h14',
    play: 'M7 5l12 7-12 7z',
    pause: 'M8 5v14M16 5v14',
    stop: 'M6 6h12v12H6z',
    stopAll: 'M4 7h9v9H4zM10 4h10v10',
    trash: 'M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13',
    broom: 'M14 4l6 6M12 6l6 6-7 7-6-1-1-6z',
    gear: 'M12 9a3 3 0 100 6 3 3 0 000-6zM19.4 15a1.7 1.7 0 00.3 1.8l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.8-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 11-4 0v-.1a1.7 1.7 0 00-1.1-1.5 1.7 1.7 0 00-1.8.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.8 1.7 1.7 0 00-1.5-1H3a2 2 0 110-4h.1a1.7 1.7 0 001.5-1.1 1.7 1.7 0 00-.3-1.8l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.8.3H9a1.7 1.7 0 001-1.5V3a2 2 0 114 0v.1a1.7 1.7 0 001 1.5 1.7 1.7 0 001.8-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.8V9a1.7 1.7 0 001.5 1H21a2 2 0 110 4h-.1a1.7 1.7 0 00-1.5 1z',
    clock: 'M12 3a9 9 0 100 18 9 9 0 000-18zM12 7v5l3 2',
    globe: 'M12 3a9 9 0 100 18 9 9 0 000-18zM3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18',
    magnet: 'M6 4v7a6 6 0 0012 0V4h-4v7a2 2 0 01-4 0V4zM6 8h4M14 8h4',
    gauge: 'M12 14l4-4M4 18a9 9 0 1116 0',
    folder: 'M3 7a2 2 0 012-2h4l2 2h8a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2z',
    file: 'M6 3h8l4 4v14H6zM14 3v4h4',
    video: 'M4 6h11v12H4zM15 10l5-3v10l-5-3',
    music: 'M9 18V6l10-2v12M9 18a2 2 0 11-4 0 2 2 0 014 0zM19 16a2 2 0 11-4 0 2 2 0 014 0z',
    archive: 'M4 4h16v4H4zM5 8v12h14V8M10 12h4',
    doc: 'M6 3h8l4 4v14H6zM9 12h6M9 16h6',
    app: 'M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z',
    image: 'M4 5h16v14H4zM4 16l5-5 4 4 3-3 4 4M15 9h.01',
    check: 'M5 12l5 5 9-10',
    x: 'M6 6l12 12M18 6L6 18',
    alert: 'M12 4l9 16H3zM12 10v4M12 17h.01',
    list: 'M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01',
    search: 'M11 4a7 7 0 100 14 7 7 0 000-14zM20 20l-4-4',
    download: 'M12 4v11M7 10l5 5 5-5M5 20h14',
    link: 'M10 14a4 4 0 005.7 0l3-3a4 4 0 00-5.7-5.7l-1 1M14 10a4 4 0 00-5.7 0l-3 3a4 4 0 005.7 5.7l1-1',
    refresh: 'M20 11a8 8 0 10-2 6M20 4v7h-7',
    queue: 'M4 6h16M4 12h10M4 18h7M17 15l4 3-4 3z',
    spider: 'M12 8a3 3 0 100 6 3 3 0 000-6zM12 14v6M5 5l4 4M19 5l-4 4M3 12h6M15 12h6M5 19l4-4M19 19l-4-4',
    export: 'M12 16V4M7 9l5-5 5 5M5 20h14',
    import: 'M12 4v12M7 11l5 5 5-5M5 20h14',
    chevron: 'M9 6l6 6-6 6',
  }
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d={paths[name] ?? paths.file} />
</svg>
