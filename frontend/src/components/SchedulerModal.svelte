<script lang="ts">
  import { onMount } from 'svelte'
  import Modal from './Modal.svelte'
  import { api, errMsg } from '../lib/api'
  import { t } from '../lib/i18n.svelte'
  import type { QueueState, Schedule, SchedulerConfig } from '../lib/types'

  let { onclose }: { onclose: () => void } = $props()
  let queues = $state<QueueState[]>([])
  let cfg = $state<SchedulerConfig>({ schedules: [], quota: { enabled: false, mb: 1000, hours: 1 }, syncEveryMinutes: 0 })
  let used = $state(0)
  let current = $state('main')
  let newName = $state('')
  let error = $state('')

  onMount(load)

  async function load() {
    queues = await api.queues()
    const st = await api.scheduler()
    cfg = st.config
    if (!cfg.schedules) cfg.schedules = []
    if (!cfg.quota) cfg.quota = { enabled: false, mb: 1000, hours: 1 }
    used = st.quotaUsedMb
  }

  let sched = $derived(cfg.schedules.find((s) => s.queue === current))
  let queue = $derived(queues.find((q) => q.name === current))

  function ensureSched(): Schedule {
    let s = cfg.schedules.find((x) => x.queue === current)
    if (!s) {
      s = { queue: current, enabled: true, startTime: '02:00', stopTime: '', days: [], once: false, onDone: 'none' }
      cfg.schedules.push(s)
    }
    return cfg.schedules.find((x) => x.queue === current)!
  }

  function toggleDay(s: Schedule, d: number) {
    s.days = s.days.includes(d) ? s.days.filter((x) => x !== d) : [...s.days, d].sort()
  }

  async function addQueue() {
    const n = newName.trim()
    if (!n) return
    try {
      await api.saveQueue({ name: n, running: false, maxActive: 0 })
      newName = ''
      queues = await api.queues()
      current = n
    } catch (e) { error = errMsg(e) }
  }

  async function delQueue() {
    if (current === 'main') return
    await api.deleteQueue(current)
    cfg.schedules = cfg.schedules.filter((s) => s.queue !== current)
    current = 'main'
    queues = await api.queues()
  }

  async function save() {
    try {
      if (queue) await api.saveQueue($state.snapshot(queue) as QueueState)
      await api.saveScheduler($state.snapshot(cfg) as SchedulerConfig)
      onclose()
    } catch (e) { error = errMsg(e) }
  }
</script>

<Modal title={t('schedTitle')} {onclose} width={760}>
  <div class="wrap">
    <aside>
      <div class="h">{t('queues')}</div>
      {#each queues as q}
        <button class:active={current === q.name} onclick={() => (current = q.name)}>
          {q.name === 'main' ? 'Principale' : q.name}{#if cfg.schedules.find((s) => s.queue === q.name && s.enabled)} ⏰{/if}
        </button>
      {/each}
      <div class="row add"><input placeholder={t('queueName')} bind:value={newName} /><button onclick={addQueue}>+</button></div>
    </aside>
    <section>
      {#if queue}
        <div class="row">
          <div class="field grow"><span>{t('maxActiveQueue')}</span><input type="number" min="0" max="20" bind:value={queue.maxActive} /></div>
          <div class="row" style="padding-top:14px">
            <button onclick={() => api.startQueue(current)}>{t('startQueue')}</button>
            <button onclick={() => api.stopQueue(current)}>{t('stopQueue')}</button>
            {#if current !== 'main'}<button class="danger" onclick={delQueue}>{t('deleteQueue')}</button>{/if}
          </div>
        </div>
      {/if}

      {#if sched}
        <div class="card">
          <label class="check"><input type="checkbox" bind:checked={sched.enabled} /> <b>{t('enabled')}</b></label>
          <div class="row">
            <div class="field grow"><span>{t('startAt')}</span><input type="time" bind:value={sched.startTime} /></div>
            <div class="field grow"><span>{t('stopAt')}</span><input type="time" bind:value={sched.stopTime} /></div>
          </div>
          <div class="field"><span>{t('days')} {sched.days.length === 0 ? '(' + t('everyDay') + ')' : ''}</span>
            <div class="days">
              {#each t('dayNames') as dn, d}
                <button class:on={sched.days.includes(d)} onclick={() => toggleDay(sched!, d)}>{dn}</button>
              {/each}
            </div>
          </div>
          <label class="check"><input type="checkbox" bind:checked={sched.once} /> {t('once')}</label>
          <div class="field"><span>{t('whenDone')}</span>
            <select bind:value={sched.onDone}>
              <option value="none">{t('actNone')}</option><option value="quit">{t('actQuit')}</option>
              <option value="sleep">{t('actSleep')}</option><option value="hibernate">{t('actHibernate')}</option>
              <option value="shutdown">{t('actShutdown')}</option>
            </select></div>
        </div>
      {:else}
        <button onclick={ensureSched}>⏰ {t('addSchedule')}</button>
      {/if}

      <h4>{t('quota')}</h4>
      <label class="check"><input type="checkbox" bind:checked={cfg.quota.enabled} /> {t('enabled')}</label>
      <div class="row">
        <div class="field grow"><span>{t('quotaMb')}</span><input type="number" min="1" bind:value={cfg.quota.mb} /></div>
        <div class="field grow"><span>{t('quotaHours')}</span><input type="number" min="1" bind:value={cfg.quota.hours} /></div>
        <div class="muted small" style="padding-top:14px">{t('quotaUsed')} : {used.toFixed(1)} Mo</div>
      </div>
      <div class="field"><span>{t('syncEvery')}</span><input type="number" min="0" bind:value={cfg.syncEveryMinutes} style="width:140px" /></div>
    </section>
  </div>
  {#snippet footer()}
    {#if error}<span class="err grow">{error}</span>{/if}
    <button onclick={onclose}>{t('cancel')}</button>
    <button class="primary" onclick={save}>{t('save')}</button>
  {/snippet}
</Modal>

<style>
  .wrap { display: flex; gap: 14px; min-height: 380px; }
  aside { width: 180px; flex-shrink: 0; display: flex; flex-direction: column; gap: 2px; }
  aside .h { font-size: 11px; color: var(--muted); text-transform: uppercase; margin-bottom: 4px; }
  aside button { text-align: left; background: none; border: none; padding: 6px 10px; border-radius: 6px; }
  aside button.active { background: var(--sel); color: var(--accent); }
  .add { margin-top: 8px; }
  .add input { padding: 4px 6px; }
  section { flex: 1; min-width: 0; }
  .days { display: flex; gap: 4px; flex-wrap: wrap; }
  .days button { padding: 4px 9px; }
  .days button.on { background: var(--accent); color: var(--accent-fg); border-color: var(--accent); }
  h4 { margin: 16px 0 6px; }
  .small { font-size: 12px; }
  .danger { color: var(--err); }
  .grow { flex: 1; }
</style>
