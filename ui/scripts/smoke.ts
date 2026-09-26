/* Console smoke: every screen loaded by its URL, at phone and desktop width,
   in a real browser against a real server with real data.

   Nothing else in CI renders the console. v0.4.0 shipped defects that the
   type-checker and every backend suite passed straight through — a person's
   page that was blank on refresh, two create drawers that could not do their
   job, tables that cut off their own columns — and all of them were found by
   hand, driving a browser. This is that hand, written down.

   Every screen must first prove it RENDERED: a blank page has nothing to
   overflow, and a check for the absence of a problem passes on nothing at
   all. The first version of these checks, run by hand, passed a blank page.

   Run by scripts/ci/backend-suite.sh after the e2e suite, against its server.
   Locally: CHROME_BIN=… ANUBIS_E2E_BASE_URL=http://localhost:7448 bun run smoke */
import puppeteer, { type Page } from 'puppeteer-core'

const BASE = process.env.ANUBIS_E2E_BASE_URL ?? 'http://localhost:7448'
const USER = process.env.ANUBIS_SMOKE_USER ?? 'devadmin'
const PASS = process.env.ANUBIS_SMOKE_PASSWORD ?? 'anubis-dev-password'
const CHROME = process.env.CHROME_BIN ?? '/usr/bin/google-chrome'

const ROUTES = ['/', '/identities', '/grants', '/memberships', '/operators', '/audit', '/realms',
  '/roles', '/applications', '/tenants', '/catalog', '/import', '/keys', '/playground', '/scope',
  '/signin-page']
const DRAWERS = ['identity', 'permission', 'role', 'grant', 'node', 'axis', 'membership']
const WIDTHS: [number, number][] = [[375, 812], [1280, 900]]

const failures: string[] = []
function report(where: string, problems: string[]) {
  for (const p of problems) failures.push(`${where}: ${p}`)
  console.log(`${problems.length ? 'FAIL' : 'ok  '}  ${where}${problems.length ? '\n        ' + problems.join('\n        ') : ''}`)
}

/* Runs in the page. Self-contained: puppeteer serialises it. */
function measureScreen() {
  const W = window.innerWidth
  const main = document.querySelector('main')
  const problems: string[] = []
  const rendered = main ? main.querySelectorAll('*').length : 0
  if (rendered < 20) return [`did not render (${rendered} elements in <main>)`]

  const desc = (el: Element) => `${el.tagName.toLowerCase()}.${String(el.className).slice(0, 50)} "${(el.textContent ?? '').trim().slice(0, 30)}"`
  // <main> scrolls the page, so it is not a deliberate sideways scroller.
  const inScroller = (el: Element) => {
    for (let a = el.parentElement; a && a !== main; a = a.parentElement) {
      const o = getComputedStyle(a).overflowX
      if (o === 'auto' || o === 'scroll') return true
    }
    return false
  }
  const docW = document.documentElement.scrollWidth
  if (docW > W + 1) problems.push(`the document is ${docW}px wide in a ${W}px viewport`)
  if (main && main.scrollWidth > main.clientWidth + 1) {
    // Name the element, or a CI failure means opening a browser to find it.
    const mr = main.getBoundingClientRect()
    const past = [...main.querySelectorAll('*')].filter((k) => {
      const r = k.getBoundingClientRect()
      return r.width > 0 && r.right > mr.right + 1 && !inScroller(k)
    })
    const leaf = past.find((el) => !past.some((o) => o !== el && el.contains(o)))
    problems.push(`the page scrolls sideways by ${main.scrollWidth - main.clientWidth}px` +
      (leaf ? `: ${desc(leaf)} ends at ${Math.round(leaf.getBoundingClientRect().right)}px` : ''))
  }

  for (const p of document.querySelectorAll('main .panel')) {
    if (p.querySelector('.tbl-body') || p.scrollWidth <= p.clientWidth + 2) continue
    const pr = p.getBoundingClientRect()
    const past = [...p.querySelectorAll('*')].filter((k) => {
      const r = k.getBoundingClientRect()
      return r.width > 0 && r.right > pr.right + 1 && !inScroller(k)
    })
    const leaf = past.find((el) => !past.some((o) => o !== el && el.contains(o)))
    problems.push(`a panel clips its content${leaf ? ': ' + desc(leaf) : ''}`)
  }
  // Clipped with no way to reach it — an ellipsis says so on purpose.
  for (const el of document.querySelectorAll('body *')) {
    if (el.tagName === 'MAIN' || el.closest('.tbl-body') || el.classList.contains('panel')) continue
    const cs = getComputedStyle(el)
    if ((cs.overflowX === 'hidden' || cs.overflowX === 'clip') && cs.textOverflow !== 'ellipsis' &&
        el.clientWidth > 0 && el.scrollWidth > el.clientWidth + 2)
      problems.push(`${desc(el)} is cut off (${el.scrollWidth}px of content in ${el.clientWidth}px)`)
  }
  // A table wider than its panel must be able to scroll to the rest: .panel
  // clips, so otherwise the columns past its edge are simply gone.
  for (const b of document.querySelectorAll('.tbl-body')) {
    const o = getComputedStyle(b).overflowX
    if (b.scrollWidth > b.clientWidth + 1 && o !== 'auto' && o !== 'scroll')
      problems.push(`a table is ${b.scrollWidth}px wide in a ${b.clientWidth}px panel and cannot scroll to the rest`)
  }
  // At rest, nothing scrolled, a table's header sits at the top of its table.
  for (const t of document.querySelectorAll('.tbl')) {
    const th = t.querySelector('thead th')
    const off = th ? Math.round(th.getBoundingClientRect().top - t.getBoundingClientRect().top) : 0
    if (off !== 0) problems.push(`a table header is pushed ${off}px down over its rows`)
  }
  return problems
}

function measureDrawer() {
  const box = document.querySelector('.mantine-Drawer-content')
  if (!box) return ['did not open']
  if (box.querySelectorAll('*').length < 10) return ['opened empty']
  const problems: string[] = []
  const br = box.getBoundingClientRect()
  if (br.left < -1 || br.right > window.innerWidth + 1)
    problems.push(`spans ${Math.round(br.left)}..${Math.round(br.right)} in a ${window.innerWidth}px viewport`)
  const past = [...box.querySelectorAll('*')].filter((k) => {
    const r = k.getBoundingClientRect()
    if (r.width === 0 || r.right <= br.right + 1) return false
    for (let a = k.parentElement; a && a !== box; a = a.parentElement) {
      const o = getComputedStyle(a).overflowX
      if ((o === 'auto' || o === 'scroll') && !a.matches('.mantine-Drawer-body')) return false
    }
    return true
  })
  if (past.length) problems.push(`${past.length} elements reach past its edge`)
  return problems
}

/* Wait until the screen stops changing: data arrives after the shell does,
   and a table measured while its skeleton rows are still up measures nothing. */
async function settle(page: Page) {
  await page.waitForSelector('main, .mantine-Drawer-content', { timeout: 15_000 }).catch(() => {})
  let last = -1
  let stable = 0
  for (let i = 0; i < 60 && stable < 3; i++) {
    const n = await page.evaluate(() =>
      document.querySelector('.animate-pulse') ? -1 : document.querySelectorAll('body *').length)
    stable = n !== -1 && n === last ? stable + 1 : 0
    last = n
    await new Promise((r) => setTimeout(r, 250))
  }
}

async function visit(page: Page, path: string, errors: string[]) {
  errors.length = 0
  await page.goto(BASE + path, { waitUntil: 'domcontentloaded' })
  await settle(page)
}

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: true,
  // A throwaway browser pointed at the suite's own server on loopback.
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
})
try {
  const page = await browser.newPage()
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(`page error: ${(e as Error).message}`))
  page.on('console', (m) => { if (m.type() === 'error') errors.push(`console: ${m.text()}`) })

  // Sign in through the form a person uses.
  await page.setViewport({ width: 1280, height: 900 })
  await visit(page, '/signin', errors)
  await page.type('input[autocomplete=username]', USER)
  await page.type('input[autocomplete=current-password]', PASS)
  await Promise.all([page.waitForFunction(() => location.pathname !== '/signin', { timeout: 15_000 }),
    page.keyboard.press('Enter')])

  // A person's page, reached the way an operator reaches it, then loaded by
  // URL like everything else — which is how the blank-on-refresh bug hid.
  await visit(page, '/identities', errors)
  const row = await page.$('tbody tr[data-clickable]')
  if (row) {
    await Promise.all([page.waitForFunction(() => /^\/identities\/./.test(location.pathname)), row.click()])
    ROUTES.push(await page.evaluate(() => location.pathname))
  } else {
    report('/identities/<id>', ['no person to open: the People table has no clickable rows'])
  }

  for (const [w, h] of WIDTHS) {
    await page.setViewport({ width: w, height: h })
    for (const path of ROUTES) {
      await visit(page, path, errors)
      const problems = await page.evaluate(measureScreen)
      report(`${w}px ${path.replace(/^\/identities\/.+/, '/identities/<id>')}`, [...problems, ...errors])
    }
  }

  await page.setViewport({ width: 375, height: 812 })
  for (const kind of DRAWERS) {
    await visit(page, `/?new=${kind}`, errors)
    const problems = await page.evaluate(measureDrawer)
    report(`375px drawer ${kind}`, [...problems, ...errors])
  }
} finally {
  await browser.close()
}

if (failures.length) {
  console.error(`\n${failures.length} problem${failures.length === 1 ? '' : 's'} on screen.`)
  process.exit(1)
}
console.log('\nevery screen rendered, fits at 375px and 1280px, and logged no errors')
