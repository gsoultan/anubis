/* Drives the structure and membership flows the way an operator does: by
   clicking what the screen offers. A request built by hand can pass against a
   form no person could send, so nothing here calls the API.

   Not part of CI — it writes to the database it is pointed at. Run against a
   scratch stack seeded with an `org` structure:
     CHROME_BIN=… ANUBIS_E2E_BASE_URL=http://127.0.0.1:7468 SHOTS=/tmp/shots \
       bun run scripts/drive-structure.ts */
import puppeteer, { type Page } from 'puppeteer-core'
import { mkdirSync } from 'node:fs'

const BASE = process.env.ANUBIS_E2E_BASE_URL ?? 'http://localhost:7448'
const USER = process.env.ANUBIS_SMOKE_USER ?? 'devadmin'
const PASS = process.env.ANUBIS_SMOKE_PASSWORD ?? 'anubis-dev-password'
const CHROME = process.env.CHROME_BIN ?? '/usr/bin/google-chrome'
const SHOTS = process.env.SHOTS ?? '/tmp/anubis-shots'
const SCHEME = (process.env.SCHEME ?? 'light') as 'light' | 'dark'
mkdirSync(SHOTS, { recursive: true })

const failures: string[] = []
const errors: string[] = []
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

async function settle(page: Page) {
  let last = -1
  let stable = 0
  for (let i = 0; i < 40 && stable < 3; i++) {
    const n = await page.evaluate(() =>
      document.querySelector('.animate-pulse') ? -1 : document.querySelectorAll('body *').length)
    stable = n !== -1 && n === last ? stable + 1 : 0
    last = n
    await sleep(150)
  }
}

/** Click the first visible element matching `sel` whose text contains `text`. */
async function click(page: Page, sel: string, text: string, scope = 'body', exact = false) {
  const ok = await page.evaluate((sel, text, scope, exact) => {
    const roots = [...document.querySelectorAll(scope)]
    const root = roots[roots.length - 1]
    if (!root) return false
    const el = [...root.querySelectorAll(sel)].find((e) => {
      const t = (e.textContent ?? '').replace(/\s+/g, ' ').trim()
      return (exact ? t === text : t.includes(text)) && (e as HTMLElement).offsetParent !== null
    })
    if (!(el instanceof HTMLElement)) return false
    el.scrollIntoView({ block: 'center' })
    el.click()
    return true
  }, sel, text, scope, exact)
  if (!ok) throw new Error(`nothing to click: ${sel} "${text}" in ${scope}`)
  await settle(page)
}

async function type(page: Page, sel: string, value: string) {
  const el = await page.waitForSelector(sel, { visible: true, timeout: 5000 })
  if (!el) throw new Error(`no input ${sel}`)
  await el.click()
  // Clear through the element's own setter: React ignores a plain assignment,
  // and a triple click does not select everything in every input.
  await el.evaluate((node) => {
    const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    set?.call(node, '')
    node.dispatchEvent(new Event('input', { bubbles: true }))
  })
  await el.type(value)
  await settle(page)
}

async function has(page: Page, text: string, scope = 'body') {
  return page.evaluate((text, scope) =>
    [...document.querySelectorAll(scope)].some((e) => (e.textContent ?? '').replace(/\s+/g, ' ').includes(text)), text, scope)
}

async function expectText(page: Page, text: string, scope = 'body') {
  for (let i = 0; i < 30; i++) {
    if (await has(page, text, scope)) return
    await sleep(150)
  }
  throw new Error(`expected to read "${text}" in ${scope}`)
}

let shotN = 0
async function shot(page: Page, name: string) {
  await settle(page)
  await page.screenshot({ path: `${SHOTS}/${SCHEME}-${String(++shotN).padStart(2, '0')}-${name}.png` as `${string}.png` })
}

let current: Page | null = null
async function step(name: string, fn: () => Promise<void>) {
  errors.length = 0
  try {
    await fn()
    const bad = errors.filter((e) => !/favicon/.test(e))
    if (bad.length) throw new Error(bad.join(' | '))
    console.log(`ok    ${name}`)
  } catch (e) {
    failures.push(`${name}: ${(e as Error).message}`)
    await current?.screenshot({ path: `${SHOTS}/${SCHEME}-FAIL-${name.replace(/[^a-z]+/gi, '-').slice(0, 40)}.png` as `${string}.png` }).catch(() => {})
    console.log(`FAIL  ${name}\n        ${(e as Error).message}`)
  }
}

const browser = await puppeteer.launch({ executablePath: CHROME, headless: true, args: ['--no-sandbox'] })
try {
  const page = await browser.newPage()
  current = page
  page.on('pageerror', (e) => errors.push(`page error: ${(e as Error).message}`))
  page.on('console', (m) => { if (m.type() === 'error') errors.push(`console: ${m.text()}`) })
  await page.setViewport({ width: 1280, height: 900 })
  const go = async (path: string) => { await page.goto(BASE + path, { waitUntil: 'domcontentloaded' }); await settle(page) }
  const sfx = String(Date.now() % 100000)

  // The console keeps its own scheme; ?scheme= stores it for every later load.
  await go(`/signin?scheme=${SCHEME}`)
  await page.type('input[autocomplete=username]', USER)
  await page.type('input[autocomplete=current-password]', PASS)
  await Promise.all([page.waitForFunction(() => location.pathname !== '/signin', { timeout: 15_000 }),
    page.keyboard.press('Enter')])

  await step('structure: the tree names levels, not codes', async () => {
    await go('/scope?axis=org')
    await expectText(page, 'Acme Holdings', '.scope-row')
    if (await has(page, 'org_company', '.scope-row')) throw new Error('a row prints the level code')
    await shot(page, 'structure-tree')
  })

  await step('structure: search says where each hit sits', async () => {
    await type(page, 'input[placeholder="Search by name…"]', 'Marketing')
    await expectText(page, 'Acme Indonesia › Commercial')
    await expectText(page, 'Acme Indonesia › Acme Retail › Commercial')
    await shot(page, 'structure-search-paths')
  })

  await step('levels: read, edit, and add one that nests', async () => {
    await go('/scope?axis=org')
    await click(page, 'label, .mantine-SegmentedControl-label', 'Levels')
    await expectText(page, 'Sits under Acme Holdings, or another of its own')
    await shot(page, 'levels')
    await click(page, 'button', 'Add level')
    await type(page, 'input[placeholder="Division"]', `Team ${sfx}`)
    await page.click('input[placeholder="Choose one or more levels"]')
    await click(page, '[role=option]', 'Department')
    await page.keyboard.press('Escape')
    await click(page, 'label', 'can sit inside another')
    await shot(page, 'levels-add')
    await click(page, 'button', 'Add level')
    await expectText(page, 'Level added')
    await expectText(page, 'Sits under Department, or another of its own')
  })

  await step('levels: a rule in use cannot be removed, and says why', async () => {
    await click(page, 'button[aria-label="Edit Division"]', 'Edit')
    // Division sits under Company, and divisions exist: take Company away.
    await page.keyboard.press('Tab')
    await page.evaluate(() => {
      const pill = [...document.querySelectorAll('.mantine-Pill-root')].find((p) => (p.textContent ?? '').includes('Company'))
      pill?.querySelector('button')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await page.click('input[placeholder="Choose one or more levels"]').catch(() => {})
    await click(page, '[role=option]', 'Department')
    await page.keyboard.press('Escape')
    errors.length = 0
    await click(page, 'main button', 'Save', 'body', true)
    await expectText(page, 'Rejected')
    await expectText(page, 'Some "Division" items already sit under a "Company"; move them before removing that rule.')
    await shot(page, 'levels-rule-in-use')
    errors.length = 0 // the refused request logs a console error by design
    await click(page, 'main button', 'Cancel', 'body', true)
  })

  await step('item: rename, archive, find it again, restore', async () => {
    await go('/scope?axis=org')
    await type(page, 'input[placeholder="Search by name…"]', 'Acme Retail')
    await click(page, 'button', 'Acme Retail', 'main')
    await expectText(page, 'In Acme Indonesia')
    await shot(page, 'item-inspector')
    await click(page, 'main button', 'Rename', 'body', true)
    await type(page, '.mantine-Modal-content input', `Acme Retail ${sfx}`)
    await click(page, '.mantine-Modal-content button', 'Rename', 'body', true)
    await expectText(page, `Acme Retail ${sfx}`, '.t-h1')
    await click(page, 'main button', 'Archive', 'body', true)
    await shot(page, 'item-archive-confirm')
    await click(page, '.mantine-Modal-content button', 'Archive', 'body', true)
    await expectText(page, 'hidden from pickers')
    // Gone from the tree until archived items are shown.
    await type(page, 'input[placeholder="Search by name…"]', `Acme Retail ${sfx}`)
    await expectText(page, 'Nothing matches')
    await click(page, 'label', 'Show archived')
    await expectText(page, 'archived', 'main button')
    await shot(page, 'item-archived')
    await click(page, 'main button', 'Restore', 'body', true)
    await expectText(page, 'Restored')
    // Put the name back for the next run.
    await click(page, 'main button', 'Rename', 'body', true)
    await type(page, '.mantine-Modal-content input', 'Acme Retail')
    await click(page, '.mantine-Modal-content button', 'Rename', 'body', true)
    await expectText(page, 'Renamed')
  })

  await step('item: a move that breaks the level rules is explained before it is sent', async () => {
    await go('/scope?axis=org')
    await type(page, 'input[placeholder="Search by name…"]', 'Brand')
    await click(page, 'button', 'Brand', 'main')
    await click(page, 'main button', 'Move', 'body', true)
    await type(page, '.mantine-Modal-content input[placeholder="Search for the new place…"]', 'Acme Singapore')
    await click(page, '.mantine-Modal-content button', 'Acme Singapore')
    await expectText(page, 'A department cannot sit under a company', '.mantine-Modal-content')
    await shot(page, 'item-move-refused')
  })

  const council = `Marketing Council ${sfx}`
  await step('membership: create one that applies where each member is assigned', async () => {
    await go('/?new=membership')
    await type(page, '.mantine-Drawer-content input[placeholder="Marketing Council"]', council)
    await click(page, '.mantine-Drawer-content [role=radio], .mantine-Drawer-content label', 'Where each member is assigned')
    await page.click('.mantine-Drawer-content input[placeholder="Choose a structure"]')
    await click(page, '[role=option]', 'Organisation')
    await page.click('.mantine-Drawer-content input[placeholder="Pick a role"]')
    await click(page, '[role=option]', 'e2e-authz.reader')
    await expectText(page, 'Applies at the place each member holds this membership')
    await click(page, '.mantine-Drawer-content button', 'Add role')
    await shot(page, 'membership-create')
    await click(page, '.mantine-Drawer-content button', 'Create membership')
    await expectText(page, 'Membership created')
  })

  const seat = async (place: string, exact: boolean) => {
    await go('/memberships')
    await click(page, 'section', council) // focus nothing; proves the card is there
    await page.evaluate((council) => {
      const card = [...document.querySelectorAll('main section.panel')].find((s) => (s.textContent ?? '').includes(council))
      const btn = [...(card?.querySelectorAll('button') ?? [])].find((b) => (b.textContent ?? '').includes('Add member'))
      btn?.click()
    }, council)
    await settle(page)
    await type(page, '.mantine-Drawer-content input[placeholder="Search by username or email…"]', 'admin')
    await click(page, '[role=option]', 'admin')
    await expectText(page, 'Where do they hold it?', '.mantine-Drawer-content')
    await type(page, '.mantine-Drawer-content input[placeholder="Search Organisation…"]', place)
    await click(page, '.mantine-Drawer-content .panel-inset button', place)
    if (exact) await click(page, '.mantine-Drawer-content label', 'Only ')
    await click(page, '.mantine-Drawer-content label', '90 days')
  }

  await step('membership: add a member at one company, then at one office', async () => {
    await seat('Acme Singapore', false)
    await shot(page, 'membership-add-member')
    await click(page, '.mantine-Drawer-content button', 'Add to membership')
    await expectText(page, 'Added to membership')
    await seat('Head office', true)
    await expectText(page, 'Already held at Acme Singapore', '.mantine-Drawer-content')
    await click(page, '.mantine-Drawer-content button', 'Add to membership')
    await expectText(page, 'Added to membership')
  })

  await step('membership: the same place twice is refused in the form', async () => {
    await seat('Acme Singapore', false)
    await expectText(page, 'They already hold it at Acme Singapore')
    const off = await page.evaluate(() =>
      [...document.querySelectorAll('.mantine-Drawer-content button')].find((b) => (b.textContent ?? '').includes('Add to membership'))?.hasAttribute('disabled'))
    if (!off) throw new Error('the button is enabled for an assignment that already exists')
  })

  await step('membership: roster, edit what it gives, remove one place', async () => {
    await go('/memberships')
    await page.evaluate((council) => {
      const card = [...document.querySelectorAll('main section.panel')].find((s) => (s.textContent ?? '').includes(council))
      const btn = [...(card?.querySelectorAll('button') ?? [])].find((b) => (b.textContent ?? '').includes('Members'))
      btn?.click()
    }, council)
    await expectText(page, 'Acme Singapore and inside')
    await expectText(page, 'Only Head office')
    await shot(page, 'membership-roster')
    await page.evaluate((council) => {
      const card = [...document.querySelectorAll('main section.panel')].find((s) => (s.textContent ?? '').includes(council))
      const btn = [...(card?.querySelectorAll('button') ?? [])].find((b) => (b.textContent ?? '').trim() === 'Edit')
      btn?.click()
    }, council)
    await settle(page)
    await page.click('.mantine-Modal-content input[placeholder="Pick a role"]')
    await click(page, '[role=option]', 'cyclegraph.node0')
    await click(page, '.mantine-Modal-content button', 'Add role')
    await shot(page, 'membership-edit')
    await click(page, '.mantine-Modal-content button', 'Save', 'body', true)
    await expectText(page, '2 grants changed')
  })

  await step('person: one group per place held, removed on its own', async () => {
    await go('/identities')
    await type(page, 'main input[type=search], main input[placeholder*="earch"]', 'admin')
    await click(page, 'tbody tr[data-clickable]', 'admin')
    await expectText(page, `Through ${council}`)
    await expectText(page, 'Acme Singapore and everything inside')
    await expectText(page, 'Only Head office itself')
    await shot(page, 'person-access')
    await page.evaluate(() => {
      const group = [...document.querySelectorAll('.access-group')].find((g) => (g.textContent ?? '').includes('Only Head office itself'))
      const btn = [...(group?.querySelectorAll('button') ?? [])].find((b) => (b.textContent ?? '').includes('Remove'))
      btn?.click()
    })
    await settle(page)
    await expectText(page, 'Only this place', '.mantine-Modal-content')
    await shot(page, 'person-remove-one-place')
    await click(page, '.mantine-Modal-content button', 'Remove', 'body', true)
    await expectText(page, 'Removed from membership')
    await expectText(page, 'Acme Singapore and everything inside')
    if (await has(page, 'Only Head office itself', '.access-group')) throw new Error('the removed place is still listed')
  })

  await step('structure: a new one comes with its top level', async () => {
    await go('/?new=axis')
    await type(page, '.mantine-Drawer-content input[placeholder="cost_center"]', `region_${sfx}`)
    await type(page, '.mantine-Drawer-content input[placeholder="Cost Centre"]', `Regions ${sfx}`)
    await shot(page, 'structure-create')
    await click(page, '.mantine-Drawer-content button', 'Add structure')
    await expectText(page, 'added')
    await go(`/scope?axis=region_${sfx}`)
    await click(page, 'label, .mantine-SegmentedControl-label', 'Levels')
    await expectText(page, `All Regions ${sfx}`)
    await expectText(page, 'top level')
    await shot(page, 'structure-new-levels')
  })
} finally {
  await browser.close()
}

if (failures.length) {
  console.error(`\n${failures.length} flow${failures.length === 1 ? '' : 's'} failed.`)
  process.exit(1)
}
console.log(`\nevery flow worked; screenshots in ${SHOTS}`)
