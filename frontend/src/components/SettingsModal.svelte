<script lang="ts">
  import { onMount, tick } from 'svelte'
  import Dialog from './ui/Dialog.svelte'
  import Button from './ui/Button.svelte'
  import IconButton from './ui/IconButton.svelte'
  import Switch from './ui/Switch.svelte'
  import Tabs, { tabId, panelId, type TabDef } from './ui/Tabs.svelte'
  import Skeleton from './ui/Skeleton.svelte'
  import Progress from './ui/Progress.svelte'
  import Icon from './Icon.svelte'
  import RemotePanel from './RemotePanel.svelte'
  import { api, errMsg, on } from '../lib/api'
  import { t, setLang, type I18nKey } from '../lib/i18n.svelte'
  import { store, applyTheme } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import type { Settings, ToolStatus, UpdateInfo } from '../lib/types'

  let { onclose }: { onclose: () => void } = $props()

  type Tab = 'general' | 'speed' | 'connection' | 'files' | 'browser' | 'video' | 'torrent' | 'iphone' | 'sites' | 'about'
  let tab = $state<Tab>('general')
  let s = $state<Settings | null>(null)
  let error = $state('')
  let saving = $state(false)
  let saved = false
  let tools = $state<ToolStatus[]>([])
  let toolProgress = $state<Record<string, { downloaded: number; size: number; done: boolean; error?: string }>>({})
  let hosters = $state<{ names: string[]; version: number; ytdlp: string[] }>({ names: [], version: 0, ytdlp: [] })
  // text helpers
  let captureText = $state('')
  let skipText = $state('')
  let videoSkipText = $state('')
  let limitKB = $state(0)
  let minKB = $state(0)
  let query = $state('')
  let flash = $state('')
  let checking = $state(false)
  let rulesBusy = $state(false)
  let info = $state('')

  // Settings added by newer backends: shown with their defaults, sent only when
  // the backend knows them or the user changed them.
  const OPT_DEFAULTS = { maxConnsPerHost: 16, maxActivePerHost: 0, autoActive: true, maxActiveGlobal: 0, fairShare: true, nativeTitleBar: false }
  type OptKey = keyof typeof OPT_DEFAULTS
  let opt = $state({ ...OPT_DEFAULTS })
  let present = new Set<OptKey>()
  let hasVideoSkip = $state(false)

  onMount(() => {
    api.settings().then((x) => {
      if (!x.credentials) x.credentials = []
      if (!x.categories) x.categories = []
      s = x
      captureText = (x.captureExts ?? []).join(' ')
      skipText = (x.skipSites ?? []).join('\n')
      hasVideoSkip = Array.isArray(x.extVideoSkipSites)
      videoSkipText = (x.extVideoSkipSites ?? []).join('\n')
      limitKB = Math.round((x.speedLimit ?? 0) / 1024)
      minKB = Math.round((x.captureMinSize ?? 0) / 1024)
      for (const k of Object.keys(OPT_DEFAULTS) as OptKey[]) {
        if (x[k] !== undefined && x[k] !== null) {
          present.add(k)
          ;(opt as Record<OptKey, number | boolean>)[k] = x[k] as number | boolean
        }
      }
    }).catch((e) => (error = errMsg(e)))
    loadTools()
    api.hosters().then((h) => (hosters = h)).catch(() => {})
    return on<any>('fdm:tool', (p) => {
      toolProgress[p.tool] = p
      if (p.done) loadTools()
    })
  })

  async function loadTools() {
    try { tools = await api.tools() } catch { /* optional */ }
  }

  function close() {
    // revert the live theme preview when the dialog is dismissed
    if (!saved && store.settings) applyTheme(store.settings.theme)
    onclose()
  }

  function snapshot(): Settings {
    const out = $state.snapshot(s) as Settings
    out.captureExts = captureText.split(/[\s,;]+/).map((x) => x.replace(/^\./, '').toLowerCase()).filter(Boolean)
    out.skipSites = skipText.split(/\s+/).filter(Boolean)
    if (hasVideoSkip) out.extVideoSkipSites = videoSkipText.split(/\s+/).filter(Boolean)
    out.speedLimit = Math.max(0, limitKB || 0) * 1024
    out.captureMinSize = Math.max(0, minKB || 0) * 1024
    for (const k of Object.keys(OPT_DEFAULTS) as OptKey[]) {
      if (present.has(k) || opt[k] !== OPT_DEFAULTS[k]) (out as unknown as Record<string, unknown>)[k] = opt[k]
    }
    return out
  }

  async function save() {
    if (!s) return
    saving = true
    error = ''
    try {
      const out = snapshot()
      await api.saveSettings(out)
      store.settings = await api.settings()
      setLang(out.language)
      applyTheme(out.theme)
      saved = true
      onclose()
    } catch (e) {
      error = errMsg(e)
    } finally {
      saving = false
    }
  }

  async function reset() {
    try {
      const d = await api.defaultSettings()
      if (!d.credentials) d.credentials = []
      s = d
      captureText = (d.captureExts ?? []).join(' ')
      skipText = ''
      videoSkipText = ''
      limitKB = 0
      minKB = 0
      for (const k of Object.keys(OPT_DEFAULTS) as OptKey[]) {
        ;(opt as Record<OptKey, number | boolean>)[k] = (d[k] ?? OPT_DEFAULTS[k]) as number | boolean
      }
      applyTheme(d.theme)
    } catch (e) { error = errMsg(e) }
  }

  async function browse() {
    if (!s) return
    const d = await api.chooseFolder(s.downloadDir)
    if (d) s.downloadDir = d
  }

  async function browseAV() {
    if (!s) return
    const p = await api.chooseFile(t('avPath'), t('stProgram'), store.platform === 'windows' ? '*.exe' : '*')
    if (p) s.antivirusPath = p
  }

  function setDefender() {
    if (!s) return
    s.antivirusPath = 'C:\\Program Files\\Windows Defender\\MpCmdRun.exe'
    s.antivirusArgs = '-Scan -ScanType 3 -File {file}'
  }

  let update = $state<UpdateInfo | null>(null)
  async function checkUpdate() {
    error = ''
    checking = true
    try {
      if (s) await api.saveSettings(snapshot())
      update = await api.checkUpdate()
    } catch (e) { error = errMsg(e) } finally { checking = false }
  }

  async function updateRules() {
    rulesBusy = true
    info = ''
    try {
      const ok = await api.updateRules()
      hosters = await api.hosters()
      info = ok ? t('stRulesUpdated') : t('stRulesUpToDate')
    } catch (e) { error = errMsg(e) } finally { rulesBusy = false }
  }

  const tabDefs = $derived<TabDef[]>([
    { id: 'general', label: t('tabGeneral'), icon: 'gear' },
    { id: 'speed', label: t('stTabSpeed'), icon: 'flash' },
    { id: 'connection', label: t('tabConnection'), icon: 'globe' },
    { id: 'files', label: t('tabFiles'), icon: 'folder' },
    { id: 'browser', label: t('tabBrowser'), icon: 'link' },
    { id: 'video', label: t('tabVideo'), icon: 'video' },
    { id: 'torrent', label: t('tabTorrent'), icon: 'magnet' },
    { id: 'iphone', label: t('stTabIphone'), icon: 'phone' },
    { id: 'sites', label: t('tabSites'), icon: 'shield' },
    { id: 'about', label: t('tabAbout'), icon: 'info' },
  ])

  // Search registry: one entry per setting row (label + extra keywords).
  type Entry = { id: string; tab: Tab; label: I18nKey; kw?: string }
  const ENTRIES: Entry[] = [
    { id: 'language', tab: 'general', label: 'language', kw: 'langue language english français' },
    { id: 'theme', tab: 'general', label: 'theme', kw: 'sombre clair dark light' },
    { id: 'nativeTitleBar', tab: 'general', label: 'stNativeTitleBar', kw: 'cadre frame' },
    { id: 'launchAtStartup', tab: 'general', label: 'launchAtStartup' },
    { id: 'startMinimized', tab: 'general', label: 'startMinimized', kw: 'tray' },
    { id: 'closeToTray', tab: 'general', label: 'closeToTray', kw: 'tray' },
    { id: 'showProgressWin', tab: 'general', label: 'showProgressWin' },
    { id: 'showDoneDialog', tab: 'general', label: 'showDoneDialog' },
    { id: 'clipboardMonitor', tab: 'general', label: 'clipboardMonitor', kw: 'clipboard' },
    { id: 'antivirus', tab: 'general', label: 'antivirus', kw: 'defender virus' },
    { id: 'maxConns', tab: 'speed', label: 'maxConns', kw: 'connections segments' },
    { id: 'autoConns', tab: 'speed', label: 'stAutoConns' },
    { id: 'maxConnsPerHost', tab: 'speed', label: 'stMaxConnsPerHost', kw: 'serveur host server' },
    { id: 'maxActive', tab: 'speed', label: 'maxActive', kw: 'simultaneous parallel' },
    { id: 'autoActive', tab: 'speed', label: 'stAutoActive' },
    { id: 'maxActiveGlobal', tab: 'speed', label: 'stMaxActiveGlobal' },
    { id: 'maxActivePerHost', tab: 'speed', label: 'stMaxActivePerHost', kw: 'serveur host server' },
    { id: 'speedLimit', tab: 'speed', label: 'stSpeedLimit', kw: 'limite bande bandwidth' },
    { id: 'fairShare', tab: 'speed', label: 'stFairShare', kw: 'équitable fair' },
    { id: 'allowHttp2', tab: 'speed', label: 'allowHttp2', kw: 'http2' },
    { id: 'proxy', tab: 'connection', label: 'proxy', kw: 'socks' },
    { id: 'userAgent', tab: 'connection', label: 'userAgent' },
    { id: 'insecureTls', tab: 'connection', label: 'insecureTls', kw: 'tls ssl certificat' },
    { id: 'downloadDir', tab: 'files', label: 'downloadDir', kw: 'dossier folder' },
    { id: 'categories', tab: 'files', label: 'categories', kw: 'extensions' },
    { id: 'showAddDialog', tab: 'browser', label: 'showAddDialog', kw: 'extension' },
    { id: 'videoButton', tab: 'browser', label: 'videoButton', kw: 'extension' },
    { id: 'captureExts', tab: 'browser', label: 'captureExts' },
    { id: 'captureMinSize', tab: 'browser', label: 'captureMinSize' },
    { id: 'skipSites', tab: 'browser', label: 'skipSites' },
    { id: 'videoQuality', tab: 'video', label: 'videoQuality', kw: '1080 720 qualité' },
    { id: 'hlsOutput', tab: 'video', label: 'hlsOutput', kw: 'mp4 mkv' },
    { id: 'hlsConcurrency', tab: 'video', label: 'hlsConcurrency' },
    { id: 'cookiesBrowser', tab: 'video', label: 'cookiesBrowser', kw: 'yt-dlp cookies' },
    { id: 'accelerateYoutube', tab: 'video', label: 'stAccelYoutube', kw: 'youtube' },
    { id: 'autoUpdateTools', tab: 'video', label: 'autoUpdateTools', kw: 'yt-dlp ffmpeg' },
    { id: 'tools', tab: 'video', label: 'tools', kw: 'yt-dlp ffmpeg' },
    { id: 'torrentPort', tab: 'torrent', label: 'torrentPort' },
    { id: 'torrentSeed', tab: 'torrent', label: 'torrentSeed', kw: 'seed' },
    { id: 'remote', tab: 'iphone', label: 'stTabIphone', kw: 'remote qr wifi' },
    { id: 'credentials', tab: 'sites', label: 'credentials', kw: 'login password mot de passe' },
    { id: 'hosters', tab: 'sites', label: 'hosters' },
    { id: 'rulesUrl', tab: 'sites', label: 'rulesUrl' },
    { id: 'updateRepo', tab: 'about', label: 'stUpdateRepo', kw: 'github update mise à jour' },
  ]

  const norm = (x: string) => x.toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g, '')
  let results = $derived.by(() => {
    const q = norm(query.trim())
    if (!q) return []
    return ENTRIES.filter((e) => !(e.id === 'nativeTitleBar' && store.platform !== 'windows'))
      .filter((e) => norm((t(e.label) as string) + ' ' + (e.kw ?? '') + ' ' + e.id).includes(q))
  })
  let grouped = $derived(tabDefs.map((d) => ({ tab: d, items: results.filter((r) => r.tab === d.id) })).filter((g) => g.items.length))

  async function goto(e: Entry) {
    query = ''
    tab = e.tab
    flash = e.id
    await tick()
    const el = document.querySelector<HTMLElement>(`[data-setting="${e.id}"]`)
    el?.scrollIntoView({ block: 'center' })
    el?.querySelector<HTMLElement>('input, select, textarea, button')?.focus({ preventScroll: true })
    setTimeout(() => { if (flash === e.id) flash = '' }, 1600)
  }
</script>

<Dialog title={t('settings')} onclose={close} width={900} height={640} padded={false}>
  <div class="wrap">
    <nav aria-label={t('settings')}>
      <div class="search">
        <Icon name="search" size={14} />
        <input type="search" placeholder={t('stSearch')} aria-label={t('stSearch')} bind:value={query} data-autofocus />
      </div>
      <Tabs tabs={tabDefs} bind:value={tab} prefix="set" label={t('settings')} orientation="vertical" variant="nav" onchange={() => (query = '')} />
    </nav>
    <section class="pane">
      {#if query.trim()}
        <div class="results" aria-live="polite">
          {#each grouped as g (g.tab.id)}
            <h3>{g.tab.label}</h3>
            {#each g.items as r (r.id)}
              <button class="result" onclick={() => goto(r)}><span>{t(r.label)}</span><Icon name="chevronRight" size={14} /></button>
            {/each}
          {:else}
            <p class="muted">{t('stNoMatch')}</p>
          {/each}
        </div>
      {:else if !s}
        <div class="sk" aria-busy="true">
          {#if error}<p class="err" role="alert">{error}</p>{:else}
            <Skeleton width="40%" height="24px" /><Skeleton height="32px" /><Skeleton height="32px" /><Skeleton width="70%" height="20px" /><Skeleton width="60%" height="20px" />
          {/if}
        </div>
      {:else}
        <div role="tabpanel" id={panelId('set', tab)} aria-labelledby={tabId('set', tab)} tabindex="-1" class="panel">
          {#if tab === 'general'}
            <h3>{t('stAppearance')}</h3>
            <div class="row">
              <div class="field grow set" data-setting="language" class:flash={flash === 'language'}><label for="st-lang">{t('language')}</label>
                <select id="st-lang" bind:value={s.language}><option value="fr">Français</option><option value="en">English</option></select></div>
              <div class="field grow set" data-setting="theme" class:flash={flash === 'theme'}><label for="st-theme">{t('theme')}</label>
                <select id="st-theme" bind:value={s.theme} onchange={() => applyTheme(s!.theme)}><option value="system">{t('themeSystem')}</option><option value="light">{t('themeLight')}</option><option value="dark">{t('themeDark')}</option></select></div>
            </div>
            {#if store.platform === 'windows'}
              <div class="set" data-setting="nativeTitleBar" class:flash={flash === 'nativeTitleBar'}>
                <Switch bind:checked={opt.nativeTitleBar} label={t('stNativeTitleBar')} description={t('stNativeTitleBarHint')} />
              </div>
            {/if}
            <h3>{t('stStartup')}</h3>
            <div class="set" data-setting="launchAtStartup" class:flash={flash === 'launchAtStartup'}><Switch bind:checked={s.launchAtStartup} label={t('launchAtStartup')} /></div>
            <div class="set" data-setting="startMinimized" class:flash={flash === 'startMinimized'}><Switch bind:checked={s.startMinimized} label={t('startMinimized')} /></div>
            <div class="set" data-setting="closeToTray" class:flash={flash === 'closeToTray'}><Switch bind:checked={s.closeToTray} label={t('closeToTray')} /></div>
            <h3>{t('stWindows')}</h3>
            <div class="set" data-setting="showProgressWin" class:flash={flash === 'showProgressWin'}><Switch bind:checked={s.showProgressWin} label={t('showProgressWin')} /></div>
            <div class="set" data-setting="showDoneDialog" class:flash={flash === 'showDoneDialog'}><Switch bind:checked={s.showDoneDialog} label={t('showDoneDialog')} /></div>
            <div class="set" data-setting="clipboardMonitor" class:flash={flash === 'clipboardMonitor'}><Switch bind:checked={s.clipboardMonitor} label={t('clipboardMonitor')} /></div>
            <h3>{t('antivirus')}</h3>
            <div class="set" data-setting="antivirus" class:flash={flash === 'antivirus'}>
              <div class="field"><label for="st-av">{t('avPath')}</label>
                <div class="row"><input id="st-av" class="grow" bind:value={s.antivirusPath} /><Button icon="folder" onclick={browseAV}>{t('browse')}</Button>
                  {#if store.platform === 'windows'}<Button icon="shield" onclick={setDefender}>Windows Defender</Button>{/if}</div></div>
              <div class="field"><label for="st-avargs">{t('avArgs')}</label><input id="st-avargs" bind:value={s.antivirusArgs} /></div>
            </div>
          {:else if tab === 'speed'}
            <h3>{t('stConnections')}</h3>
            <div class="row top">
              <div class="field grow set" data-setting="maxConns" class:flash={flash === 'maxConns'}><label for="st-mc">{t('maxConns')}</label>
                <select id="st-mc" bind:value={s.maxConns}>{#each [1, 2, 4, 6, 8, 12, 16, 24, 32] as n}<option value={n}>{n}</option>{/each}</select>
                <span class="hint">{t('stMaxConnsHint')}</span></div>
              <div class="field grow set" data-setting="maxConnsPerHost" class:flash={flash === 'maxConnsPerHost'}><label for="st-mch">{t('stMaxConnsPerHost')}</label>
                <input id="st-mch" type="number" min="1" max="128" bind:value={opt.maxConnsPerHost} />
                <span class="hint">{t('stMaxConnsPerHostHint')}</span></div>
            </div>
            <div class="set" data-setting="autoConns" class:flash={flash === 'autoConns'}><Switch bind:checked={s.autoConns} label={t('stAutoConns')} description={t('stAutoConnsHint')} /></div>
            <h3>{t('stConcurrency')}</h3>
            <div class="set" data-setting="autoActive" class:flash={flash === 'autoActive'}><Switch bind:checked={opt.autoActive} label={t('stAutoActive')} description={t('stAutoActiveHint')} /></div>
            <div class="row top">
              <div class="field grow set" data-setting="maxActive" class:flash={flash === 'maxActive'}><label for="st-ma">{opt.autoActive ? t('stMaxActiveCap') : t('maxActive')}</label>
                <input id="st-ma" type="number" min="1" max="20" bind:value={s.maxActive} /></div>
              <div class="field grow set" data-setting="maxActiveGlobal" class:flash={flash === 'maxActiveGlobal'}><label for="st-mag">{t('stMaxActiveGlobal')}</label>
                <input id="st-mag" type="number" min="0" max="50" bind:value={opt.maxActiveGlobal} /><span class="hint">{t('stZeroNoCap')}</span></div>
              <div class="field grow set" data-setting="maxActivePerHost" class:flash={flash === 'maxActivePerHost'}><label for="st-mah">{t('stMaxActivePerHost')}</label>
                <input id="st-mah" type="number" min="0" max="20" bind:value={opt.maxActivePerHost} /><span class="hint">{t('stZeroUnlimited')}</span></div>
            </div>
            <h3>{t('stBandwidth')}</h3>
            <div class="field set narrow" data-setting="speedLimit" class:flash={flash === 'speedLimit'}><label for="st-lim">{t('stSpeedLimit')}</label>
              <div class="unit"><input id="st-lim" type="number" min="0" step="128" bind:value={limitKB} /><span>{fmt.unit(1)}/s</span></div>
              <span class="hint">{t('stZeroUnlimited')}</span></div>
            <div class="set" data-setting="fairShare" class:flash={flash === 'fairShare'}><Switch bind:checked={opt.fairShare} label={t('stFairShare')} description={t('stFairShareHint')} /></div>
            <div class="set" data-setting="allowHttp2" class:flash={flash === 'allowHttp2'}><Switch bind:checked={s.allowHttp2} label={t('allowHttp2')} /></div>
          {:else if tab === 'connection'}
            <div class="field set" data-setting="proxy" class:flash={flash === 'proxy'}><label for="st-proxy">{t('proxy')}</label><input id="st-proxy" bind:value={s.proxy} placeholder="socks5://127.0.0.1:1080" /></div>
            <div class="field set" data-setting="userAgent" class:flash={flash === 'userAgent'}><label for="st-ua">{t('userAgent')}</label><input id="st-ua" bind:value={s.userAgent} placeholder="Mozilla/5.0 …" /></div>
            <div class="set" data-setting="insecureTls" class:flash={flash === 'insecureTls'}><Switch bind:checked={s.insecureTls} label={t('insecureTls')} /></div>
          {:else if tab === 'files'}
            <div class="field set" data-setting="downloadDir" class:flash={flash === 'downloadDir'}><label for="st-dir">{t('downloadDir')}</label>
              <div class="row"><input id="st-dir" class="grow" bind:value={s.downloadDir} /><Button icon="folder" onclick={browse}>{t('browse')}</Button></div></div>
            <div class="set" data-setting="categories" class:flash={flash === 'categories'}>
              <h3>{t('categories')}</h3>
              <table class="grid cats">
                <thead><tr><th>{t('catName')}</th><th>{t('catExts')}</th><th>{t('catDir')}</th><th><span class="sr-only">{t('delete')}</span></th></tr></thead>
                <tbody>
                  {#each s.categories as c, i}
                    <tr>
                      <td><input bind:value={c.name} aria-label={t('catName')} /></td>
                      <td><input value={(c.exts ?? []).join(' ')} aria-label={t('catExts')} onchange={(e) => (c.exts = e.currentTarget.value.split(/[\s,;]+/).filter(Boolean))} /></td>
                      <td><div class="row"><input class="grow" bind:value={c.dir} aria-label={t('catDir')} />
                        <IconButton icon="folder" size="sm" label={t('browse')} onclick={async () => { const d = await api.chooseFolder(c.dir || s!.downloadDir); if (d) c.dir = d }} /></div></td>
                      <td><IconButton icon="trash" size="sm" tone="danger" label={t('stDeleteCategory')} onclick={() => s!.categories.splice(i, 1)} /></td>
                    </tr>
                  {/each}
                </tbody>
              </table>
              <Button icon="plus" size="sm" onclick={() => s!.categories.push({ name: t('stNewCategory'), exts: [], dir: '', key: '' })}>{t('addCategory')}</Button>
            </div>
          {:else if tab === 'browser'}
            <p class="muted">{t('extensionHelp')}</p>
            <div class="set" data-setting="showAddDialog" class:flash={flash === 'showAddDialog'}><Switch bind:checked={s.showAddDialog} label={t('showAddDialog')} /></div>
            <div class="set" data-setting="videoButton" class:flash={flash === 'videoButton'}><Switch bind:checked={s.videoButton} label={t('videoButton')} /></div>
            <div class="field set" data-setting="captureExts" class:flash={flash === 'captureExts'}><label for="st-cap">{t('captureExts')}</label><textarea id="st-cap" rows="4" bind:value={captureText}></textarea></div>
            <div class="field set narrow" data-setting="captureMinSize" class:flash={flash === 'captureMinSize'}><label for="st-min">{t('captureMinSize')}</label><input id="st-min" type="number" min="0" bind:value={minKB} /></div>
            <div class="field set" data-setting="skipSites" class:flash={flash === 'skipSites'}><label for="st-skip">{t('skipSites')}</label><textarea id="st-skip" rows="3" bind:value={skipText}></textarea></div>
            {#if hasVideoSkip}
              <div class="field"><label for="st-vskip">{t('stVideoSkipSites')}</label><textarea id="st-vskip" rows="3" bind:value={videoSkipText}></textarea></div>
            {/if}
          {:else if tab === 'video'}
            <div class="row">
              <div class="field grow set" data-setting="videoQuality" class:flash={flash === 'videoQuality'}><label for="st-vq">{t('videoQuality')}</label>
                <select id="st-vq" bind:value={s.videoQuality}>
                  <option value="best">{t('best')}</option>
                  {#each ['2160', '1440', '1080', '720', '480', '360'] as q}<option value={q}>{q}p</option>{/each}
                  <option value="audio">{t('audioOnly')}</option>
                </select></div>
              <div class="field grow set" data-setting="hlsOutput" class:flash={flash === 'hlsOutput'}><label for="st-ho">{t('hlsOutput')}</label>
                <select id="st-ho" bind:value={s.hlsOutput}><option value="mp4">MP4</option><option value="mkv">MKV</option><option value="ts">TS</option></select></div>
              <div class="field grow set" data-setting="hlsConcurrency" class:flash={flash === 'hlsConcurrency'}><label for="st-hc">{t('hlsConcurrency')}</label><input id="st-hc" type="number" min="1" max="32" bind:value={s.hlsConcurrency} /></div>
            </div>
            <div class="field set" data-setting="cookiesBrowser" class:flash={flash === 'cookiesBrowser'}><label for="st-cb">{t('cookiesBrowser')}</label>
              <select id="st-cb" bind:value={s.ytdlpCookiesBrowser}>
                <option value="">{t('none')}</option>
                {#each ['opera', 'chrome', 'edge', 'brave', 'vivaldi', 'firefox', 'safari'] as b}<option value={b}>{b}</option>{/each}
              </select></div>
            <div class="set" data-setting="accelerateYoutube" class:flash={flash === 'accelerateYoutube'}><Switch bind:checked={s.accelerateYoutube} label={t('stAccelYoutube')} /></div>
            <div class="set" data-setting="autoUpdateTools" class:flash={flash === 'autoUpdateTools'}><Switch bind:checked={s.autoUpdateTools} label={t('autoUpdateTools')} /></div>
            <h3 data-setting="tools" class:flash={flash === 'tools'}>{t('tools')}</h3>
            <table class="grid">
              <tbody>
                {#each tools as tool (tool.name)}
                  {@const tp = toolProgress[tool.name]}
                  <tr>
                    <td><b>{tool.name}</b></td>
                    <td class={tool.installed ? 'ok' : 'muted'}>{tool.installed ? tool.version || t('installed') : t('notInstalled')}</td>
                    <td class="muted ellipsis small path" title={tool.path}>{tool.path}</td>
                    <td class="act">
                      {#if tp && !tp.done}
                        <Progress value={fmt.percent(tp.downloaded, tp.size)} indeterminate={!(tp.size > 0)} label={tool.name} />
                      {:else}
                        <Button size="sm" icon={tool.installed ? 'refresh' : 'download'} onclick={() => api.installTool(tool.name)}>{tool.installed ? t('update') : t('install')}</Button>
                        {#if tp?.error}<span class="err small" role="alert">{tp.error}</span>{/if}
                      {/if}
                    </td>
                  </tr>
                {:else}
                  <tr><td colspan="4"><Skeleton height="20px" /></td></tr>
                {/each}
              </tbody>
            </table>
          {:else if tab === 'torrent'}
            <div class="field set narrow" data-setting="torrentPort" class:flash={flash === 'torrentPort'}><label for="st-tp">{t('torrentPort')}</label><input id="st-tp" type="number" min="0" max="65535" bind:value={s.torrentPort} /></div>
            <div class="set" data-setting="torrentSeed" class:flash={flash === 'torrentSeed'}><Switch bind:checked={s.torrentSeed} label={t('torrentSeed')} /></div>
          {:else if tab === 'iphone'}
            <div data-setting="remote"><RemotePanel /></div>
          {:else if tab === 'sites'}
            <div class="set" data-setting="credentials" class:flash={flash === 'credentials'}>
              <h3>{t('credentials')}</h3>
              <table class="grid">
                <thead><tr><th>{t('host')}</th><th>{t('user')}</th><th>{t('password')}</th><th><span class="sr-only">{t('delete')}</span></th></tr></thead>
                <tbody>
                  {#each s.credentials as c, i}
                    <tr>
                      <td><input bind:value={c.host} aria-label={t('host')} /></td>
                      <td><input bind:value={c.user} autocomplete="off" aria-label={t('user')} /></td>
                      <td><input type="password" bind:value={c.password} autocomplete="off" aria-label={t('password')} /></td>
                      <td><IconButton icon="trash" size="sm" tone="danger" label={t('stDeleteCred')} onclick={() => s!.credentials.splice(i, 1)} /></td>
                    </tr>
                  {:else}
                    <tr><td colspan="4" class="muted small">{t('stNoCreds')}</td></tr>
                  {/each}
                </tbody>
              </table>
              <Button icon="plus" size="sm" onclick={() => s!.credentials.push({ host: '', user: '', password: '' })}>{t('addCred')}</Button>
            </div>
            <h3 data-setting="hosters" class:flash={flash === 'hosters'}>{t('hosters')}</h3>
            <p class="small">{hosters.names.join(' · ')}</p>
            {#if hosters.ytdlp.length}<p class="small muted">yt-dlp : {hosters.ytdlp.slice(0, 40).join(', ')}…</p>{/if}
            <p class="muted small">{t('rulesVersion')} : <span class="num">{hosters.version}</span></p>
            <div class="field set" data-setting="rulesUrl" class:flash={flash === 'rulesUrl'}><label for="st-rules">{t('rulesUrl')}</label>
              <div class="row"><input id="st-rules" class="grow" bind:value={s.rulesUrl} placeholder="https://raw.githubusercontent.com/…/hosters.json" />
                <Button icon="refresh" loading={rulesBusy} onclick={updateRules}>{t('updateRules')}</Button></div>
              {#if info}<span class="hint" role="status">{info}</span>{/if}</div>
          {:else}
            <div class="about">
              <svg width="56" height="56" viewBox="0 0 24 24" aria-hidden="true"><rect width="24" height="24" rx="6" fill="var(--accent)" /><path d="M12 5.5v8.5M8 10.5l4 4 4-4M7 18h10" stroke="var(--accent-fg)" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round" /></svg>
              <h2>FluxDM</h2>
              <p class="num">{t('version')} {store.version}</p>
              <p class="muted">{t('about')}</p>
              <p class="small muted">Go · Wails · Svelte · yt-dlp · ffmpeg · anacrolix/torrent</p>
            </div>
            <div class="field set" data-setting="updateRepo" class:flash={flash === 'updateRepo'}><label for="st-repo">{t('stUpdateRepo')}</label>
              <div class="row"><input id="st-repo" class="grow" bind:value={s.updateRepo} placeholder="owner/fluxdm" />
                <Button icon="refresh" loading={checking} onclick={checkUpdate}>{t('stCheckUpdate')}</Button></div></div>
            {#if update}
              <div class="card upd" role="status">
                <Icon name={update.newer ? 'sparkle' : 'checkCircle'} size={20} />
                <span class="grow">{update.newer ? t('stNewVersion').replace('{v}', update.latest) : t('stUpToDate').replace('{v}', update.latest)}</span>
                {#if update.newer}<Button variant="primary" icon="download" onclick={() => api.installUpdate().catch((e) => (error = errMsg(e)))}>{t('install')}</Button>{/if}
              </div>
            {/if}
          {/if}
        </div>
      {/if}
    </section>
  </div>
  {#snippet footer()}
    {#if error && s}<span class="err grow" role="alert">{error}</span>{/if}
    <Button variant="subtle" icon="refresh" disabled={!s} onclick={reset}>{t('reset')}</Button>
    <span class="spacer"></span>
    <Button onclick={close}>{t('cancel')}</Button>
    <Button variant="primary" loading={saving} disabled={!s} onclick={save}>{t('save')}</Button>
  {/snippet}
</Dialog>

<style>
  .wrap { display: flex; height: 100%; min-height: 0; }
  nav { width: 210px; flex-shrink: 0; display: flex; flex-direction: column; gap: 8px; padding: 6px 12px 16px 16px; overflow-y: auto; border-right: 1px solid var(--border); }
  .search { display: flex; align-items: center; gap: 6px; color: var(--muted); background: var(--surface-1); border: 1px solid var(--border); border-bottom-color: var(--border-strong); border-radius: var(--r-sm); padding: 0 8px; }
  .search:focus-within { border-bottom-color: var(--accent); box-shadow: inset 0 -1px 0 var(--accent); }
  .search input { border: none !important; box-shadow: none !important; background: none !important; padding: 5px 2px; min-height: 30px; }
  .pane { flex: 1; min-width: 0; overflow-y: auto; padding: 4px 24px 24px; }
  .panel:focus { outline: none; }
  h3 { font-size: var(--fs-body-lg); font-weight: 600; margin: 20px 0 8px; }
  .panel > h3:first-child, .results > h3:first-child { margin-top: 4px; }
  .set { border-radius: var(--r-md); transition: background var(--dur-slow), box-shadow var(--dur-slow); }
  .flash { background: var(--accent-soft); box-shadow: 0 0 0 6px var(--accent-soft); }
  .row.top { align-items: flex-start; }
  .narrow { max-width: 300px; }
  .unit { display: flex; align-items: center; gap: 6px; }
  .unit span { color: var(--muted); font-size: var(--fs-caption); white-space: nowrap; }
  .cats input { min-height: 28px; padding: 3px 6px; }
  .cats td, .grid td { vertical-align: middle; }
  .path { max-width: 240px; }
  .act { width: 180px; }
  .results { display: flex; flex-direction: column; gap: 2px; }
  .result { display: flex; align-items: center; justify-content: space-between; text-align: left; background: none; border-color: transparent; }
  :global(:root[data-theme='dark']) .result { background: none; }
  .result:hover { background: var(--surface-3) !important; }
  .sk { display: flex; flex-direction: column; gap: 14px; padding-top: 8px; }
  .about { text-align: center; padding: 24px 0 12px; display: flex; flex-direction: column; align-items: center; }
  .about h2 { margin: 10px 0 2px; font-size: var(--fs-title); }
  .about p { margin: 2px 0; }
  .upd { display: flex; align-items: center; gap: 10px; color: var(--accent-text); }
  .upd span { color: var(--fg); }
  .spacer { flex: 1; }
</style>
