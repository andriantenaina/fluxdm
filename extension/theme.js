// FluxDM — applies the theme to extension pages before first paint.
// Preference: chrome.storage.local prefs.theme = 'system' | 'light' | 'dark',
// mirrored in localStorage so this synchronous script avoids a flash.
;(() => {
  'use strict'
  const mq = window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null
  let pref = 'system'
  try { pref = localStorage.getItem('fdm-theme') || 'system' } catch (e) { /* storage blocked */ }

  function apply() {
    const dark = pref === 'dark' || (pref !== 'light' && !!(mq && mq.matches))
    document.documentElement.dataset.theme = dark ? 'dark' : 'light'
  }
  apply()
  if (mq && mq.addEventListener) mq.addEventListener('change', apply)

  try {
    chrome.storage.local.get('prefs').then((r) => {
      const t = (r.prefs && r.prefs.theme) || 'system'
      if (t !== pref) { pref = t; apply() }
      try { localStorage.setItem('fdm-theme', t) } catch (e) { /* */ }
    })
    chrome.storage.onChanged.addListener((ch, area) => {
      if (area !== 'local' || !ch.prefs) return
      pref = (ch.prefs.newValue && ch.prefs.newValue.theme) || 'system'
      try { localStorage.setItem('fdm-theme', pref) } catch (e) { /* */ }
      apply()
    })
  } catch (e) { /* not in an extension page */ }

  try { document.documentElement.lang = chrome.i18n.getUILanguage().split('-')[0] } catch (e) { /* */ }
})()
