// FluxDM options: app-side settings (POST /v1/settings through the background,
// saved locally when the app is unreachable) + extension preferences.
'use strict'
const F = self.FDM
const M = (k, subs) => F.msg(k, subs)
const $ = (id) => document.getElementById(id)

function send(msg) {
  return new Promise((resolve) => chrome.runtime.sendMessage(msg, (r) => { void chrome.runtime.lastError; resolve(r || { error: M('errUnreachable') }) }))
}

F.localize(document.body)
document.title = M('optionsTitle')

let loaded = null

function setStatus(node, kind, text) {
  node.className = 'status-line' + (kind ? ' ' + kind : '')
  node.textContent = ''
  if (!text) return
  if (kind === 'ok') node.insertAdjacentHTML('beforeend', F.icon('check', 14))
  if (kind === 'err') node.insertAdjacentHTML('beforeend', F.icon('error', 14))
  const s = document.createElement('span')
  s.textContent = text
  node.appendChild(s)
}

function fieldError(id, text) {
  const input = $(id)
  $(id + 'Err').textContent = text || ''
  if (text) input.setAttribute('aria-invalid', 'true')
  else input.removeAttribute('aria-invalid')
  return !text
}

function previewTheme(t) {
  const dark = t === 'dark' || (t !== 'light' && matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
}

async function load() {
  const st = await send({ type: 'status' })
  const s = st.settings || {}
  loaded = st
  $('connDot').className = 'dot ' + (st.connected ? 'ok' : 'ko')
  $('connText').textContent = st.connected ? M('connected') + (st.version ? ' · v' + st.version : '') : M('optOffline')
  $('captureEnabled').checked = st.captureEnabled !== false
  $('appCaptureOff').hidden = s.captureEnabled !== false
  $('captureExts').value = (s.captureExts || []).join(' ')
  $('captureMinSize').value = String(Math.round((s.captureMinSize || 0) / 1024))
  $('skipSites').value = (s.skipSites || []).join('\n')
  $('videoButton').checked = s.videoButton !== false
  $('videoSkipSites').value = (s.videoSkipSites || []).join('\n')
  const p = st.prefs || {}
  const theme = p.theme || 'system'
  for (const r of document.querySelectorAll('input[name=theme]')) r.checked = r.value === theme
  $('notifications').checked = p.notifications !== false
}

function validate() {
  const ex = F.parseExts($('captureExts').value)
  const sk = F.parseHosts($('skipSites').value)
  const vs = F.parseHosts($('videoSkipSites').value)
  const raw = $('captureMinSize').value.trim()
  const kb = raw === '' ? 0 : Number(raw)
  let ok = true
  ok = fieldError('captureExts', ex.invalid.length ? M('errInvalidExts', [ex.invalid.slice(0, 5).join(', ')]) : '') && ok
  ok = fieldError('captureMinSize', !Number.isFinite(kb) || kb < 0 || Math.floor(kb) !== kb ? M('errMinSize') : '') && ok
  ok = fieldError('skipSites', sk.invalid.length ? M('errInvalidHosts', [sk.invalid.slice(0, 5).join(', ')]) : '') && ok
  ok = fieldError('videoSkipSites', vs.invalid.length ? M('errInvalidHosts', [vs.invalid.slice(0, 5).join(', ')]) : '') && ok
  return ok ? {
    captureExts: ex.values, captureMinSize: kb * 1024, skipSites: sk.values,
    videoButton: $('videoButton').checked, videoSkipSites: vs.values,
  } : null
}

for (const id of ['captureExts', 'captureMinSize', 'skipSites', 'videoSkipSites']) {
  $(id).addEventListener('blur', () => validate())
  $(id).addEventListener('input', () => { if ($(id).getAttribute('aria-invalid')) validate() })
}
for (const r of document.querySelectorAll('input[name=theme]')) r.addEventListener('change', () => previewTheme(r.value))

$('form').addEventListener('submit', async (e) => {
  e.preventDefault()
  const values = validate()
  if (!values) {
    setStatus($('saveStatus'), 'err', M('errFixFields'))
    const bad = document.querySelector('[aria-invalid=true]')
    if (bad) bad.focus()
    return
  }
  const btn = $('save')
  btn.disabled = true
  setStatus($('saveStatus'), 'busy', M('saving'))
  try {
    // only send what changed: keeps app-side values the user did not touch
    const prev = (loaded && loaded.settings) || {}
    const partial = {}
    for (const k of Object.keys(values)) if (JSON.stringify(values[k]) !== JSON.stringify(prev[k])) partial[k] = values[k]
    let r = { ok: true }
    if (Object.keys(partial).length) r = await send({ type: 'saveSettings', settings: partial })
    const theme = (document.querySelector('input[name=theme]:checked') || {}).value || 'system'
    await chrome.storage.local.set({ prefs: { theme, notifications: $('notifications').checked } })
    try { localStorage.setItem('fdm-theme', theme) } catch (err) { /* */ }
    await send({ type: 'setCapture', value: $('captureEnabled').checked })
    if (r.error) setStatus($('saveStatus'), 'err', r.error)
    else {
      setStatus($('saveStatus'), 'ok', r.local ? M('savedLocallyLong') : M('saved'))
      await load()
    }
  } finally {
    btn.disabled = false
  }
})

load()
