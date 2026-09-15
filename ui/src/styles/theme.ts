import { createTheme, rem, type MantineColorsTuple } from '@mantine/core'

/* The interactive colour: buttons, links, focus, active navigation.
   Deliberately a full hue family away from every status colour — the previous
   accent was the same amber as --warn, so "you are here" and "something is
   wrong" rendered identically. Shades 6 and 7 are the filled-button shades and
   both clear 4.5:1 against white text. */
const accent: MantineColorsTuple = [
  '#eef2ff', '#dce4ff', '#bcc9ff', '#97aaff', '#7590fb',
  '#5a76f5', '#4361ee', '#3b5bdc', '#2f49b4', '#273c92',
]

/* Anubis gold. The jackal and the wordmark, and nothing else. */
const brand: MantineColorsTuple = [
  '#fdf7ea', '#f7ebd0', '#eed6a1', '#e5c06e', '#ddae47',
  '#d9a32f', '#d89c22', '#bf8717', '#a8801c', '#8d6100',
]

/* Cool neutral tuned to sit on the dark surfaces without turning muddy. */
const slate: MantineColorsTuple = [
  '#f4f6f8', '#e7eaee', '#cbd1da', '#adb6c3', '#939eae',
  '#8290a2', '#78879b', '#667488', '#5a6779', '#4b586a',
]

const allow: MantineColorsTuple = [
  '#ecfdf5', '#d1fae5', '#a7f3d0', '#6ee7b7', '#34d399',
  '#10b981', '#059669', '#047857', '#065f46', '#064e3b',
]

const deny: MantineColorsTuple = [
  '#fef2f2', '#fee2e2', '#fecaca', '#fca5a5', '#f87171',
  '#ef4444', '#dc2626', '#b91c1c', '#991b1b', '#7f1d1d',
]

export const theme = createTheme({
  primaryColor: 'accent',
  /* Dark takes 6 (#4361ee), light takes 7 (#3b5bdc). Both are dark enough that
     autoContrast resolves to white text and clears AA at 13px. */
  primaryShade: { dark: 6, light: 7 },
  autoContrast: true,
  luminanceThreshold: 0.4,
  colors: { accent, brand, slate, allow, deny },
  defaultRadius: 'sm',

  fontFamily:
    'Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif',
  fontFamilyMonospace:
    "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace",

  /* 13px body, not 12 or 10. The first version chased density and produced
     text that is genuinely hard to read during an incident — density has to
     come from spacing and hierarchy, not from shrinking the type. */
  fontSizes: {
    xs: rem(11), sm: rem(12.5), md: rem(13), lg: rem(15), xl: rem(18),
  },
  lineHeights: { xs: '1.45', sm: '1.5', md: '1.55', lg: '1.5', xl: '1.4' },

  headings: {
    fontWeight: '610',
    sizes: {
      h1: { fontSize: rem(22), lineHeight: '1.2' },
      h2: { fontSize: rem(16), lineHeight: '1.35' },
      h3: { fontSize: rem(13.5), lineHeight: '1.4' },
      h4: { fontSize: rem(13), lineHeight: '1.4' },
    },
  },

  /* 4px rhythm. */
  spacing: { xs: rem(6), sm: rem(10), md: rem(16), lg: rem(24), xl: rem(36) },
  radius:  { xs: rem(4), sm: rem(6), md: rem(8), lg: rem(12), xl: rem(16) },

  components: {
    Button: {
      defaultProps: { radius: 'sm' },
      styles: { root: { fontWeight: 560, letterSpacing: '-0.005em' } },
    },
    Badge: {
      defaultProps: { variant: 'light', radius: 'sm' },
      styles: {
        root: {
          fontWeight: 600, letterSpacing: '0.01em',
          textTransform: 'none' as const, paddingInline: rem(7),
        },
      },
    },
    Tooltip: {
      defaultProps: {
        withArrow: true, openDelay: 350, maw: 300, multiline: true,
        transitionProps: { duration: 140 },
      },
      styles: {
        tooltip: {
          background: 'var(--tooltip-bg)', border: '1px solid var(--line)',
          color: 'var(--tooltip-ink)', fontSize: rem(11.5), lineHeight: 1.5,
          padding: '7px 10px', borderRadius: rem(6),
        },
      },
    },
    Modal: {
      defaultProps: {
        centered: true, radius: 'lg',
        overlayProps: { blur: 3, backgroundOpacity: 0.55, color: 'var(--overlay-tint)' },
        transitionProps: { transition: 'pop', duration: 180 },
      },
      styles: {
        content: { boxShadow: 'var(--shadow-lg)' },
        header: { borderBottom: '1px solid var(--line-soft)' },
        title: { fontWeight: 610, letterSpacing: '-0.014em' },
      },
    },
    Drawer: {
      defaultProps: {
        overlayProps: { blur: 3, backgroundOpacity: 0.55, color: 'var(--overlay-tint)' },
      },
      styles: {
        content: { background: 'var(--s-base)' },
        header: { background: 'var(--s-base)', borderBottom: '1px solid var(--line)' },
        title: { fontWeight: 610, letterSpacing: '-0.014em' },
      },
    },
    Popover: {
      defaultProps: { radius: 'md', shadow: 'xl', withinPortal: true,
        transitionProps: { transition: 'pop', duration: 140 } },
      styles: { dropdown: { background: 'var(--s-raised)', border: '1px solid var(--line)',
        boxShadow: 'var(--shadow-lg)' } },
    },
    Menu: {
      defaultProps: { radius: 'md', shadow: 'xl', withinPortal: true },
      styles: {
        dropdown: { background: 'var(--s-raised)', border: '1px solid var(--line)',
          boxShadow: 'var(--shadow-lg)', padding: rem(5) },
        item: { borderRadius: rem(6), fontSize: rem(13) },
        label: { fontSize: rem(10.5), fontWeight: 650, letterSpacing: '0.06em',
          textTransform: 'uppercase' as const, color: 'var(--ink-3)' },
      },
    },
    Input: { styles: { input: { background: 'var(--s-sunken)', borderColor: 'var(--line)' } } },
    TextInput: { defaultProps: { size: 'sm' } },
    Select: { defaultProps: { size: 'sm', comboboxProps: { shadow: 'xl' } } },
    ScrollArea: { defaultProps: { scrollbarSize: 8, type: 'hover' } },
    Divider: { styles: { root: { borderColor: 'var(--line-soft)' } } },
  },
})
