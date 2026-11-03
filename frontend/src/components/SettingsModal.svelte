<script lang="ts">
  import { onMount } from 'svelte'
  import Modal from './Modal.svelte'
  import { api, errMsg, on } from '../lib/api'
  import { t, setLang } from '../lib/i18n.svelte'
  import { store, applyTheme } from '../lib/state.svelte'
  import * as fmt from '../lib/format'
  import type { Settings, ToolStatus, UpdateInfo } from '../lib/types'

  let { onclose }: { onclose: () => void } = $props()

  type Tab = 'general' | 'connection' | 'files' | 'browser' | 'video' | 'torrent' | 'sites' | 'about'
  let tab = $state<Tab>('general')
  let s = $state<Settings | null>(null)
  let error = $state('')
  let saving = $state(false)
  let tools = $state<ToolStatus[]>([])
  let toolProgress = $state<Record<string, { downloaded: number; size: number; done: boolean; error?: string }>>({})
  let hosters = $state<{ names: string[]; version: number; ytdlp: string[] }>({ names: [], version: 0, ytdlp: [] })
  // text helpers
  let captureText = $state('')
  let skipText = $state('')
  let limitKB = $state(0)
  let minKB = $state(0)

  onMount(() => {
    api.settings().then((x) => {
      s = x
      captureText = (x.captureExts ?? []).join(' ')
      skipText = (x.skipSites ?? []).join('\n')
      limitKB = Math.round((x.speedLimit ?? 0) / 1024)
      minKB = Math.round((x.captureMinSize ?? 0) / 1024)
      if (!s.credentials) s.credentials = []
    })
    loadTools()
    api.hosters().then((h) => (hosters = h))
    return on<any>('fdm:tool', (p) => {
      toolProgress[p.tool] = p
      if (p.done) loadTools()
    })
  })

  async function loadTools() {
    tools = await api.tools()
  }

  async function save() {
    if (!s) return
    saving = true
    error = ''
    s.captureExts = captureText.split(/[\s,;]+/).map((x) => x.replace(/^\./, '').toLowerCase()).filter(Boolean)
    s.skipSites = skipText.split(/\s+/).filter(Boolean)
    s.speedLimit = Math.max(0, limitKB) * 1024
    s.captureMinSize = Math.max(0, minKB) * 1024
    try {
      await api.saveSettings($state.snapshot(s) as Settings)
      store.settings = await api.settings()
      setLang(s.language)
      applyTheme(s.theme)
      onclose()
    } catch (e) {
      error = errMsg(e)
    } finally {
      saving = false
    }
  }

  async function reset() {
    const d = await api.defaultSettings()
    s = d
    captureText = d.captureExts.join(' ')
    skipText = ''
    limitKB = 0
    minKB = 0
  }

  async function browse(field: 'downloadDir') {
    if (!s) return
    const d = await api.chooseFolder(s[field])
    if (d) s[field] = d
  }

  async function browseAV() {
    if (!s) return
    const p = await api.chooseFile(t('avPath'), 'Programme', store.platform === 'windows' ? '*.exe' : '*')
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
    if (s) await api.saveSettings($state.snapshot(s) as Settings)
    try { update = await api.checkUpdate() } catch (e) { error = errMsg(e) }
  }

  async function updateRules() {
    try {
      const ok = await api.updateRules()
      hosters = await api.hosters()
      error = ok ? '' : 'Règles déjà à jour'
    } catch (e) { error = errMsg(e) }
  }

  const tabs: [Tab, string][] = [
    ['general', 'tabGeneral'], ['connection', 'tabConnection'], ['files', 'tabFiles'], ['browser', 'tabBrowser'],
    ['video', 'tabVideo'], ['torrent', 'tabTorrent'], ['sites', 'tabSites'], ['about', 'tabAbout'],
  ]
</script>

<Modal title={t('settings')} {onclose} width={820}>
  {#if s}
    <div class="wrap">
      <nav>
        {#each tabs as [k, label]}
          <button class:active={tab === k} onclick={() => (tab = k)}>{t(label as 'tabGeneral')}</button>
        {/each}
      </nav>
      <section>
        {#if tab === 'general'}
          <div class="row">
            <div class="field grow"><span>{t('language')}</span>
              <select bind:value={s.language}><option value="fr">Français</option><option value="en">English</option></select></div>
            <div class="field grow"><span>{t('theme')}</span>
              <select bind:value={s.theme}><option value="system">{t('themeSystem')}</option><option value="light">{t('themeLight')}</option><option value="dark">{t('themeDark')}</option></select></div>
          </div>
          <label class="check"><input type="checkbox" bind:checked={s.launchAtStartup} /> {t('launchAtStartup')}</label>
          <label class="check"><input type="checkbox" bind:checked={s.startMinimized} /> {t('startMinimized')}</label>
          <label class="check"><input type="checkbox" bind:checked={s.closeToTray} /> {t('closeToTray')}</label>
          <label class="check"><input type="checkbox" bind:checked={s.showProgressWin} /> {t('showProgressWin')}</label>
          <label class="check"><input type="checkbox" bind:checked={s.showDoneDialog} /> {t('showDoneDialog')}</label>
          <label class="check"><input type="checkbox" bind:checked={s.clipboardMonitor} /> {t('clipboardMonitor')}</label>
          <h4>{t('antivirus')}</h4>
          <div class="field"><span>{t('avPath')}</span>
            <div class="row"><input class="grow" bind:value={s.antivirusPath} /><button onclick={browseAV}>{t('browse')}</button>
              {#if store.platform === 'windows'}<button onclick={setDefender}>Windows Defender</button>{/if}</div></div>
          <div class="field"><span>{t('avArgs')}</span><input bind:value={s.antivirusArgs} /></div>
        {:else if tab === 'connection'}
          <div class="row">
            <div class="field grow"><span>{t('maxConns')}</span>
              <select bind:value={s.maxConns}>{#each [1, 2, 4, 6, 8, 12, 16, 24, 32] as n}<option value={n}>{n}</option>{/each}</select></div>
            <div class="field grow"><span>{t('maxActive')}</span><input type="number" min="1" max="20" bind:value={s.maxActive} /></div>
            <div class="field grow"><span>{t('speedLimit')}</span><input type="number" min="0" bind:value={limitKB} /></div>
          </div>
          <div class="field"><span>{t('proxy')}</span><input bind:value={s.proxy} placeholder="socks5://127.0.0.1:1080" /></div>
          <div class="field"><span>{t('userAgent')}</span><input bind:value={s.userAgent} placeholder="Mozilla/5.0 …" /></div>
          <label class="check"><input type="checkbox" bind:checked={s.allowHttp2} /> {t('allowHttp2')}</label>
          <label class="check"><input type="checkbox" bind:checked={s.insecureTls} /> {t('insecureTls')}</label>
        {:else if tab === 'files'}
          <div class="field"><span>{t('downloadDir')}</span>
            <div class="row"><input class="grow" bind:value={s.downloadDir} /><button onclick={() => browse('downloadDir')}>{t('browse')}</button></div></div>
          <table class="grid cats">
            <thead><tr><th>{t('catName')}</th><th>{t('catExts')}</th><th>{t('catDir')}</th><th></th></tr></thead>
            <tbody>
              {#each s.categories as c, i}
                <tr>
                  <td><input bind:value={c.name} /></td>
                  <td><input value={(c.exts ?? []).join(' ')} onchange={(e) => (c.exts = (e.currentTarget as HTMLInputElement).value.split(/[\s,;]+/).filter(Boolean))} /></td>
                  <td><div class="row"><input class="grow" bind:value={c.dir} /><button onclick={async () => { const d = await api.chooseFolder(c.dir || s!.downloadDir); if (d) c.dir = d }}>…</button></div></td>
                  <td><button class="link danger" onclick={() => s!.categories.splice(i, 1)}>✕</button></td>
                </tr>
              {/each}
            </tbody>
          </table>
          <button onclick={() => s!.categories.push({ name: 'Nouvelle', exts: [], dir: '' })}>{t('addCategory')}</button>
        {:else if tab === 'browser'}
          <p class="muted">{t('extensionHelp')}</p>
          <label class="check"><input type="checkbox" bind:checked={s.showAddDialog} /> {t('showAddDialog')}</label>
          <label class="check"><input type="checkbox" bind:checked={s.videoButton} /> {t('videoButton')}</label>
          <div class="field"><span>{t('captureExts')}</span><textarea rows="4" bind:value={captureText}></textarea></div>
          <div class="field" style="width:260px"><span>{t('captureMinSize')}</span><input type="number" min="0" bind:value={minKB} /></div>
          <div class="field"><span>{t('skipSites')}</span><textarea rows="3" bind:value={skipText}></textarea></div>
        {:else if tab === 'video'}
          <div class="row">
            <div class="field grow"><span>{t('videoQuality')}</span>
              <select bind:value={s.videoQuality}>
                <option value="best">{t('best')}</option>
                {#each ['2160', '1440', '1080', '720', '480', '360'] as q}<option value={q}>{q}p</option>{/each}
                <option value="audio">{t('audioOnly')}</option>
              </select></div>
            <div class="field grow"><span>{t('hlsOutput')}</span>
              <select bind:value={s.hlsOutput}><option value="mp4">MP4</option><option value="mkv">MKV</option><option value="ts">TS</option></select></div>
            <div class="field grow"><span>{t('hlsConcurrency')}</span><input type="number" min="1" max="32" bind:value={s.hlsConcurrency} /></div>
          </div>
          <div class="field"><span>{t('cookiesBrowser')}</span>
            <select bind:value={s.ytdlpCookiesBrowser}>
              <option value="">{t('none')}</option>
              {#each ['opera', 'chrome', 'edge', 'brave', 'vivaldi', 'firefox', 'safari'] as b}<option value={b}>{b}</option>{/each}
            </select></div>
          <label class="check"><input type="checkbox" bind:checked={s.autoUpdateTools} /> {t('autoUpdateTools')}</label>
          <h4>{t('tools')}</h4>
          <table class="grid">
            <tbody>
              {#each tools as tool}
                {@const tp = toolProgress[tool.name]}
                <tr>
                  <td><b>{tool.name}</b></td>
                  <td class={tool.installed ? 'ok' : 'muted'}>{tool.installed ? tool.version || t('installed') : t('notInstalled')}</td>
                  <td class="muted ellipsis small" title={tool.path}>{tool.path}</td>
                  <td style="width:170px">
                    {#if tp && !tp.done}
                      <div class="bar"><div style:width="{fmt.percent(tp.downloaded, tp.size)}%"></div></div>
                    {:else}
                      <button onclick={() => api.installTool(tool.name)}>{tool.installed ? t('update') : t('install')}</button>
                      {#if tp?.error}<span class="err small">{tp.error}</span>{/if}
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {:else if tab === 'torrent'}
          <div class="field" style="width:260px"><span>{t('torrentPort')}</span><input type="number" min="0" max="65535" bind:value={s.torrentPort} /></div>
          <label class="check"><input type="checkbox" bind:checked={s.torrentSeed} /> {t('torrentSeed')}</label>
        {:else if tab === 'sites'}
          <h4>{t('credentials')}</h4>
          <table class="grid">
            <thead><tr><th>{t('host')}</th><th>{t('user')}</th><th>{t('password')}</th><th></th></tr></thead>
            <tbody>
              {#each s.credentials as c, i}
                <tr>
                  <td><input bind:value={c.host} /></td>
                  <td><input bind:value={c.user} autocomplete="off" /></td>
                  <td><input type="password" bind:value={c.password} autocomplete="off" /></td>
                  <td><button class="link danger" onclick={() => s!.credentials.splice(i, 1)}>✕</button></td>
                </tr>
              {/each}
            </tbody>
          </table>
          <button onclick={() => s!.credentials.push({ host: '', user: '', password: '' })}>{t('addCred')}</button>
          <h4>{t('hosters')}</h4>
          <p class="small">{hosters.names.join(' · ')}</p>
          <p class="small muted">yt-dlp : {hosters.ytdlp.slice(0, 40).join(', ')}…</p>
          <div class="row">
            <span class="muted small">{t('rulesVersion')} : {hosters.version}</span>
          </div>
          <div class="field"><span>{t('rulesUrl')}</span>
            <div class="row"><input class="grow" bind:value={s.rulesUrl} placeholder="https://raw.githubusercontent.com/…/hosters.json" /><button onclick={updateRules}>{t('updateRules')}</button></div></div>
        {:else}
          <div class="about">
            <h2>FluxDM</h2>
            <p>{t('version')} {store.version}</p>
            <p class="muted">{t('about')}</p>
            <p class="small muted">Go · Wails · Svelte · yt-dlp · ffmpeg · anacrolix/torrent</p>
          </div>
          <div class="field"><span>Dépôt GitHub des mises à jour (propriétaire/nom)</span>
            <div class="row"><input class="grow" bind:value={s.updateRepo} placeholder="mon-compte/fluxdm" />
              <button onclick={checkUpdate}>Rechercher les mises à jour</button></div></div>
          {#if update}
            <p>{update.newer ? `Nouvelle version ${update.latest} disponible.` : `FluxDM est à jour (${update.latest}).`}
              {#if update.newer}<button class="primary" onclick={() => api.installUpdate().catch((e) => (error = errMsg(e)))}>Installer</button>{/if}</p>
          {/if}
        {/if}
      </section>
    </div>
  {/if}
  {#snippet footer()}
    {#if error}<span class="err grow">{error}</span>{/if}
    <button onclick={reset}>{t('reset')}</button>
    <button onclick={onclose}>{t('cancel')}</button>
    <button class="primary" disabled={saving} onclick={save}>{t('save')}</button>
  {/snippet}
</Modal>

<style>
  .wrap { display: flex; gap: 14px; min-height: 420px; }
  nav { display: flex; flex-direction: column; gap: 2px; width: 140px; flex-shrink: 0; }
  nav button { text-align: left; background: none; border: none; padding: 7px 10px; border-radius: 6px; }
  nav button.active { background: var(--sel); color: var(--accent); }
  section { flex: 1; min-width: 0; }
  h4 { margin: 16px 0 8px; font-size: 13px; }
  .cats input { padding: 4px 6px; }
  .small { font-size: 12px; }
  .grow { flex: 1; }
  .danger { color: var(--err); }
  .about { text-align: center; padding-top: 40px; }
  .about h2 { margin: 0 0 6px; }
</style>
