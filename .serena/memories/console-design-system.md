# Console design system — three colour families, and why

`ui/src/styles/index.css` owns the tokens; `ui/src/styles/theme.ts` mirrors the
accent into Mantine. Nothing below the token blocks may name a raw colour.

## The rule that was being broken

There are **three families and they may never be crossed**:

| Family | Token | Job |
| :--- | :--- | :--- |
| brand | `--brand` | the jackal and the wordmark. Identity, nothing else. |
| interactive | `--accent*` | buttons, links, focus rings, active nav. |
| meaning | `--allow` `--deny` `--warn` `--info` | verdicts and status only. |
| category | `--kind-*` | realm kinds. Not a verdict, not an accent. |

The accent used to be `--gold: #e3ad3c` and the warning was `--warn: #f3c14a`
— the same amber. "You are here" and "something is wrong" rendered in one
colour, which is the thing the original token comment said must never happen.
The fix is structural, not disciplinary: interactive is blue and sits a whole
hue family away from every status colour, so the mistake is no longer
expressible.

Realm kinds had the same bug one level down — `partner` was painted with
`--info`, the status colour for "a fact you should notice". They now have
their own `--kind-*` family.

## Contrast is a constraint, not a preference

Every status colour clears 4.5:1 on its own background. The light scheme did
not: `allow` was `#0f9d63` at **3.5:1**, so the one word on the screen that
says access was granted was the one a person with ordinary eyesight could not
read. `--ink-3` is the floor for real text and is pinned at ~4.6:1 in both
schemes; `--ink-4` is decorative only.

Filled accent buttons use `--accent-solid` (darker than `--accent`) so white
label text clears AA at 13px. `--accent` is the *readable* one, for text and
icons on a page background.

## Surfaces must step far enough apart to be seen

Dark surfaces used to step by ~2 points (`#090a0d` → `#0b0d11` → `#13161c`),
which is invisible on an office monitor — sidebar, page and panels read as one
flat sheet. Steps are now ~9 points. Anything below about 6 does not survive a
real display.

`--s-nav` exists so the sidebar can go the opposite way per scheme: **below**
the page in dark, **above** it (white) in light. Both give two visibly
different planes.

Dark mode also had `--panel-shadow: none`. A panel that is only a 1px border
on near-black does not read as sitting above anything; there is now a real
`--shadow-sm/md/lg` scale in both schemes.

## Density is a token

`--row-h: 44px`, applied as `height` on `<td>` — which resolves as a *minimum*,
so a one-line row is exactly 44px and a genuinely two-line cell still grows.
That is what lets density be one number instead of a per-table decision.

`Cell` is **inline by default** (`name  qualifier` on one line) with a
`stacked` escape hatch for cells carrying two real facts (a permission key and
its prose description). Stacking every identity row cost 63px and fitted
eleven of 57,138 people on a laptop; inline fits ~19. See [[console-tables]].

## Table columns: give every column a width

Leave one column without one and it becomes the only column the browser can
grow, so all the slack on a wide display pools into it. The roles table ran
670px of Role column for 120px of name while the right-hand columns were
crushed. `identities.tsx` already carried this lesson in a comment; it is a
general rule, not a per-screen fix.

## Master-detail: the detail pane must be on screen when you click

The Structure page had the selection inspector as the **fourth** panel in the
right column, under Configuration, Sync and Item kinds. Clicking a node at the
top of the tree updated something ~900px away, below the fold. A detail pane
that is not visible when the thing it describes is clicked is not a detail
pane. Order is: selection first, settings after, rail sticky.

Two structural rules came out of the same screen:

- **A grid stretches its columns to equal height.** The tree panel grew to
  860px to match the settings stack beside it and rendered one row of content
  inside an empty white rectangle. `items-start` on the grid, plus a
  `minHeight` on the primary surface so it does not collapse the other way.
- **Switching between N things is navigation, not content.** As a wrapping
  grid of cards the 26-axis switcher came to five rows and ~290px before the
  tree it selects — unbounded in N. One scrollable row is bounded. Put the
  overflow fade on a wrapper, not on the scroller, or it scrolls away with the
  content.

Hard-coded control widths do not survive a narrower column: `MultiSelect
w={220}` in a 400px rail truncated the level name to "Depart…" for both
`Department` and `Departments`, which is the one distinction that panel
exists to draw. Stack label over control instead of pinning a width.

## Related

[[console-tables]] · [[console-person-page]] · [[console-create-drawers]]
