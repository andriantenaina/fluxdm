<script lang="ts">
  import { onMount } from 'svelte'
  import Dialog from './ui/Dialog.svelte'
  import Button from './ui/Button.svelte'
  import IconButton from './ui/IconButton.svelte'
  import Switch from './ui/Switch.svelte'
  import Skeleton from './ui/Skeleton.svelte'
  import EmptyState from './ui/EmptyState.svelte'
  import SideList, { type SideItem } from './settings/SideList.svelte'
  import { api, errMsg } from '../lib/api'
  import { t, queueLabel } from '../lib/i18n.svelte'
  import { confirmBox } from '../lib/dialogs.svelte'
  import type { QueueState, Schedule, SchedulerConfig } from '../lib/types'

  let { onclose }: { onclose: () => void } = $props()
  let queues = $state<QueueState[]>([])
  let cfg = $state<SchedulerConfig>({ schedules: [], quota: { enabled: false, mb: 1000, hours: 1 }, syncEveryMinutes: 0 })
  let used = $state(0)
  let current = $state('main')
  let newName = $state('')
  let error = $state('')
  let loaded = $state(false)
  let saving = $state(false)

  onMount(load)

  async function load() {
    try {
      queues = await api.queues()
      const st = await api.scheduler()
      cfg = st.config
      if (!cfg.schedules) cfg.schedules = []
      if (!cfg.quota) cfg.quota = { enabled: false, mb: 1000, hours: 1 }
      used = st.quotaUsedMb
    } catch (e) { error = errMsg(e) } finally { loaded = true }
  }

  let sched = $derived(cfg.schedules.find((s) => s.queue === current))
  let queue = $derived(queues.find((q) => q.name === current))
  let sideItems = $derived<SideItem[]>(queues.map((q) => ({
    id: q.name, label: queueLabel(q.name), icon: 'queue',
    mark: cfg.schedules.some((s) => s.queue === q.name && s.enabled) ? { icon: 'alarm', label: t('schScheduled') } : undefined,
  })))

  function addSchedule() {
    if (cfg.schedules.some((x) => x.queue === current)) return
    const s: Schedule = { queue: current, enabled: true, startTime: '02:00', stopTime: '', days: [], once: false, onDone: 'none' }
    cfg.schedules.push(s)
  }

  function removeSchedule() {
    cfg.schedules = cfg.schedules.filter((s) => s.queue !== current)
  }

  function toggleDay(s: Schedule, d: number) {
    s.days = s.days.includes(d) ? s.days.filter((x) => x !== d) : [...s.days, d].sort()
  }

  async function addQueue(e?: Event) {
    e?.preventDefault()
    const n = newName.trim()
    if (!n) return
    try {
      await api.saveQueue({ name: n, running: false, maxActive: 0 })
      newName = ''
      queues = await api.queues()
      current = n
    } catch (err) { error = errMsg(err) }
  }

  async function delQueue() {
    if (current === 'main') return
    const r = await confirmBox(t('schDeleteQueueConfirm'), undefined, { title: t('deleteQueue'), okLabel: t('delete'), danger: true })
    if (!r.ok) return
    try {
      await api.deleteQueue(current)
      cfg.schedules = cfg.schedules.filter((s) => s.queue !== current)
      current = 'main'
      queues = await api.queues()
    } catch (e) { error = errMsg(e) }
  }

  async function queueAction(fn: () => Promise<void>) {
    try { await fn(); queues = await api.queues() } catch (e) { error = errMsg(e) }
  }

  async function save() {
    saving = true
    error = ''
    try {
      if (queue) await api.saveQueue($state.snapshot(queue) as QueueState)
      await api.saveScheduler($state.snapshot(cfg) as SchedulerConfig)
      onclose()
    } catch (e) { error = errMsg(e) } finally { saving = false }
  }

  const dayOrder = [1, 2, 3, 4, 5, 6, 0]
</script>

<Dialog title={t('schedTitle')} {onclose} width={780}>
  {#if !loaded}
    <div class="wrap" aria-busy="true">
      <div class="skside"><Skeleton height="28px" /><Skeleton height="28px" /><Skeleton height="28px" /></div>
      <div class="grow col"><Skeleton height="32px" /><Skeleton height="140px" radius="var(--r-md)" /><Skeleton height="32px" /></div>
    </div>
  {:else}
    <div class="wrap">
      <SideList items={sideItems} value={current} onselect={(id) => (current = id)} label={t('queues')} heading={t('queues')}>
        {#snippet footer()}
          <form class="add" onsubmit={addQueue}>
            <input placeholder={t('queueName')} aria-label={t('newQueue')} bind:value={newName} />
            <IconButton icon="plus" label={t('newQueue')} type="submit" variant="default" disabled={!newName.trim()} />
          </form>
        {/snippet}
      </SideList>
      <section class="grow">
        {#if queue}
          <div class="qhead">
            <h3>{queueLabel(queue.name)}{#if queue.running}<span class="tag ok">{t('active')}</span>{/if}</h3>
            <div class="row">
              <Button size="sm" icon="play" onclick={() => queueAction(() => api.startQueue(current))}>{t('startQueue')}</Button>
              <Button size="sm" icon="stop" onclick={() => queueAction(() => api.stopQueue(current))}>{t('stopQueue')}</Button>
              {#if current !== 'main'}<Button size="sm" variant="subtle" icon="trash" class="danger" onclick={delQueue}>{t('deleteQueue')}</Button>{/if}
            </div>
          </div>
          <div class="field narrow"><label for="sch-max">{t('maxActiveQueue')}</label>
            <input id="sch-max" type="number" min="0" max="20" bind:value={queue.maxActive} /></div>
        {/if}

        {#if sched}
          <div class="card sched">
            <div class="row between">
              <Switch bind:checked={sched.enabled} label={t('schEnabled')} />
              <Button size="sm" variant="subtle" icon="trash" onclick={removeSchedule}>{t('schRemove')}</Button>
            </div>
            <div class="row">
              <div class="field grow"><label for="sch-start">{t('startAt')}</label><input id="sch-start" type="time" bind:value={sched.startTime} /></div>
              <div class="field grow"><label for="sch-stop">{t('stopAt')}</label><input id="sch-stop" type="time" bind:value={sched.stopTime} /></div>
            </div>
            <div class="field">
              <span id="sch-days">{t('days')} {sched.days.length === 0 ? '(' + t('everyDay') + ')' : ''}</span>
              <div class="days" role="group" aria-labelledby="sch-days">
                {#each dayOrder as d}
                  <button type="button" aria-pressed={sched.days.includes(d)} class:on={sched.days.includes(d)} onclick={() => toggleDay(sched!, d)}>{t('dayNames')[d]}</button>
                {/each}
              </div>
            </div>
            <Switch bind:checked={sched.once} label={t('once')} />
            <div class="field"><label for="sch-done">{t('whenDone')}</label>
              <select id="sch-done" bind:value={sched.onDone}>
                <option value="none">{t('actNone')}</option><option value="quit">{t('actQuit')}</option>
                <option value="sleep">{t('actSleep')}</option><option value="hibernate">{t('actHibernate')}</option>
                <option value="shutdown">{t('actShutdown')}</option>
              </select></div>
          </div>
        {:else}
          <div class="card">
            <EmptyState compact art="queue" title={t('schNoSchedule')} text={t('schNoScheduleHint')}>
              <Button icon="alarm" onclick={addSchedule}>{t('addSchedule')}</Button>
            </EmptyState>
          </div>
        {/if}

        <h4>{t('quota')}</h4>
        <Switch bind:checked={cfg.quota.enabled} label={t('schQuotaEnable')} />
        <div class="row">
          <div class="field grow"><label for="sch-mb">{t('quotaMb')}</label><input id="sch-mb" type="number" min="1" bind:value={cfg.quota.mb} disabled={!cfg.quota.enabled} /></div>
          <div class="field grow"><label for="sch-h">{t('quotaHours')}</label><input id="sch-h" type="number" min="1" bind:value={cfg.quota.hours} disabled={!cfg.quota.enabled} /></div>
        </div>
        <p class="muted small used">{t('quotaUsed')} : <span class="num">{used.toFixed(1)}</span> {t('schMb')}</p>
        <div class="field narrow"><label for="sch-sync">{t('syncEvery')}</label><input id="sch-sync" type="number" min="0" bind:value={cfg.syncEveryMinutes} /></div>
      </section>
    </div>
  {/if}
  {#snippet footer()}
    {#if error}<span class="err grow" role="alert">{error}</span>{/if}
    <Button onclick={onclose}>{t('cancel')}</Button>
    <Button variant="primary" loading={saving} disabled={!loaded} onclick={save}>{t('save')}</Button>
  {/snippet}
</Dialog>

<style>
  .wrap { display: flex; gap: 20px; min-height: 400px; }
  .skside { width: 190px; display: flex; flex-direction: column; gap: 6px; }
  .col { display: flex; flex-direction: column; gap: 12px; }
  .add { display: flex; gap: 4px; }
  .add input { min-width: 0; }
  section { min-width: 0; }
  .qhead { display: flex; align-items: center; gap: 12px; justify-content: space-between; flex-wrap: wrap; margin-bottom: 12px; }
  h3 { margin: 0; font-size: var(--fs-subtitle); font-weight: 600; display: flex; align-items: center; gap: 8px; }
  h4 { margin: 20px 0 6px; font-size: var(--fs-body-lg); }
  .narrow { max-width: 320px; }
  .sched { display: flex; flex-direction: column; gap: 4px; }
  .between { justify-content: space-between; }
  .days { display: flex; gap: 4px; flex-wrap: wrap; }
  .days button { min-width: 44px; padding: 4px 8px; }
  .days button.on, :global(:root[data-theme='dark']) .days button.on { background: var(--accent); color: var(--accent-fg); border-color: transparent; }
  .days button.on:hover { background: var(--accent-hover) !important; }
  .used { margin: 0 0 10px; }
</style>
