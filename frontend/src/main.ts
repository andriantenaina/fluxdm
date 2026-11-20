import { mount } from 'svelte'
import './app.css'

const target = document.getElementById('app')!

// FluxDM Remote (served on the LAN to a phone) must not load the Wails runtime.
if (location.hash.startsWith('#/remote')) {
  import('./remote/RemoteApp.svelte').then(({ default: RemoteApp }) => mount(RemoteApp, { target }))
} else {
  import('./App.svelte').then(({ default: App }) => mount(App, { target }))
}
