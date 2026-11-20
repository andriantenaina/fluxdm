// node --test extension/common.test.js
const test = require('node:test')
const assert = require('node:assert')
const F = require('./common.js')

test('cleanTitle', () => {
  assert.strictEqual(F.cleanTitle('(3) Ma super vidéo - YouTube'), 'Ma super vidéo')
  assert.strictEqual(F.cleanTitle('Watch film_2024.mp4 - Voe'), 'film 2024')
  assert.strictEqual(F.cleanTitle('master'), '')
  assert.strictEqual(F.cleanTitle('Episode 3 | Streaming'), 'Episode 3')
})

test('names and formats', () => {
  assert.strictEqual(F.finalName('Mon film - YouTube', '1080p', 'mp4'), 'Mon film (1080p).mp4')
  assert.strictEqual(F.finalName('', '', 'mp3'), 'video.mp3')
  assert.strictEqual(F.extFromMime('video/webm; codecs="vp9"'), 'webm')
  assert.strictEqual(F.dispositionName(`attachment; filename*=UTF-8''%C3%A9t%C3%A9.mp4`), 'été.mp4')
  assert.strictEqual(F.dispositionName('inline; filename="clip.mov"'), 'clip.mov')
  assert.strictEqual(F.fmtSize(1536 * 1024 * 1024, true), '~1.5 Go')
})

test('groupMedia merges HLS variants', () => {
  const now = Date.now()
  const g = F.groupMedia([
    { kind: 'hls', url: 'https://cdn.x/v/master.m3u8', key: 'a', frameId: 3, time: now },
    { kind: 'hls', url: 'https://cdn.x/v/720/index.m3u8', key: 'b', frameId: 3, time: now + 500 },
    { kind: 'hls', url: 'https://cdn.x/v/audio.m3u8', key: 'c', frameId: 3, time: now + 700 },
    { kind: 'direct', url: 'https://y/a.mp4', key: 'd', frameId: 0, time: now + 900 },
    { kind: 'direct', url: 'https://y/a.mp4', key: 'd', frameId: 0, time: now + 950 },
  ])
  assert.strictEqual(g.length, 2)
  assert.strictEqual(g.find((x) => x.kind === 'hls').url, 'https://cdn.x/v/master.m3u8')
})

test('describe uses inspection', () => {
  const d = F.describe({ kind: 'hls', url: 'https://cdn/master.m3u8', title: 'X - YouTube' }, {
    title: 'Vrai titre', defaultQuality: 'q1', outputExt: 'mp4', fileName: 'Vrai titre (1080p).mp4',
    qualities: [{ id: 'q1', label: '1080p', sizeEstimate: 300 * 1024 * 1024, ext: 'mp4' }],
  })
  assert.deepStrictEqual([d.title, d.quality, d.format, d.size], ['Vrai titre', '1080p', 'MP4', '~300 Mo'])
  const raw = F.describe({ kind: 'hls', url: 'https://cdn/index-v1-a1.m3u8', title: '' })
  assert.notStrictEqual(raw.title, 'index-v1-a1.m3u8')
})

test('fmtSize follows the language', () => {
  assert.strictEqual(F.fmtSize(1536 * 1024 * 1024, true, 'fr'), '~1.5 Go')
  assert.strictEqual(F.fmtSize(1536 * 1024 * 1024, true, 'en'), '~1.5 GB')
  assert.strictEqual(F.fmtSize(512, false, 'en'), '512 B')
  assert.strictEqual(F.fmtSize(512, false, 'fr'), '512 o')
  assert.strictEqual(F.fmtSize(0, false, 'en'), '')
  assert.strictEqual(F.fmtSpeed(2 * 1024 * 1024, 'en'), '2.0 MB/s')
  assert.strictEqual(F.fmtSpeed(0, 'en'), '')
  assert.strictEqual(F.langOf('fr-CA'), 'fr')
  assert.strictEqual(F.langOf('en-GB'), 'en')
  assert.strictEqual(F.langOf('de'), 'en')
})

test('fmtEta', () => {
  assert.strictEqual(F.fmtEta(8, 1, 'fr'), '8 s')
  assert.strictEqual(F.fmtEta(134, 0.9, 'fr'), '2 min 14 s')
  assert.strictEqual(F.fmtEta(3900, 0.9, 'en'), '1 h 05 min')
  assert.strictEqual(F.fmtEta(134, 0.2, 'en'), '≈ 2 min 14 s')
  assert.strictEqual(F.fmtEta(-2, 1, 'fr'), 'Calcul…')
  assert.strictEqual(F.fmtEta(-2, 1, 'en'), 'Calculating…')
  assert.strictEqual(F.fmtEta(-1, 1, 'en'), '—')
  assert.strictEqual(F.fmtEta(undefined, undefined, 'en'), '—')
  assert.strictEqual(F.fmtEta(0, undefined, 'en'), '0 s')
})

test('fmtFinish', () => {
  const now = new Date(2026, 0, 1, 14, 0, 0).getTime()
  assert.match(F.fmtFinish(32 * 60, 'fr', now), /^vers 14:32$/)
  assert.match(F.fmtFinish(32 * 60, 'en', now), /^at 2:32\sPM$/)
  assert.strictEqual(F.fmtFinish(-1, 'fr', now), '')
})

test('hosts and lists', () => {
  assert.strictEqual(F.normHost('https://www.Example.com:8080/path?q'), 'example.com')
  assert.strictEqual(F.normHost('*.cdn.example.org'), 'cdn.example.org')
  assert.strictEqual(F.normHost('localhost'), 'localhost')
  assert.strictEqual(F.normHost('not a host'), '')
  assert.strictEqual(F.normHost('foo'), '')
  assert.strictEqual(F.siteOf('https://www.youtube.com/watch?v=1'), 'youtube.com')
  assert.strictEqual(F.siteOf('chrome://extensions'), '')
  assert.ok(F.hostMatches('m.youtube.com', ['youtube.com']))
  assert.ok(!F.hostMatches('notyoutube.com', ['youtube.com']))
  assert.deepStrictEqual(F.parseExts('mp4, .ZIP exe;mp4 b@d'), { values: ['mp4', 'zip', 'exe'], invalid: ['b@d'] })
  assert.deepStrictEqual(F.parseHosts('youtube.com\nhttps://www.vimeo.com/x\n???'), { values: ['youtube.com', 'vimeo.com'], invalid: ['???'] })
  assert.ok(F.isRestrictedUrl('chrome://settings'))
  assert.ok(F.isRestrictedUrl('edge://extensions'))
  assert.ok(F.isRestrictedUrl('https://chromewebstore.google.com/detail/x'))
  assert.ok(F.isRestrictedUrl(''))
  assert.ok(!F.isRestrictedUrl('https://example.com'))
})

test('percent and icons', () => {
  assert.strictEqual(F.percent(50, 200), 25)
  assert.strictEqual(F.percent(10, 0), 0)
  assert.strictEqual(F.percent(300, 200), 100)
  for (const n of F.ICON_NAMES) assert.match(F.icon(n), /^<svg [^>]*aria-hidden="true"/)
  assert.strictEqual(F.msg('missingKey', null, 'fallback'), 'fallback')
})

test('describe is localized', () => {
  assert.strictEqual(F.describe({ kind: 'hls', url: 'https://cdn/master.m3u8', title: '' }, null, 'en').title, 'Video')
  assert.strictEqual(F.describe({ kind: 'hls', url: 'https://cdn/master.m3u8', title: '' }, null, 'fr').title, 'Vidéo')
  assert.strictEqual(F.describe({ kind: 'direct', url: 'https://y/a.webm', size: 2048 }, null, 'en').size, '2.0 KB')
})
