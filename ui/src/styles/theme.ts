import { createTheme, rem, type MantineColorsTuple } from '@mantine/core'

/* Anubis gold. Reserved for identity and navigation — never for status, so it
   can never be confused with a verdict. */
const gold: MantineColorsTuple = [
  '#fdf6e6', '#f6e9cc', '#ecd39c', '#e2bc68', '#daa93e',
  '#d69d24', '#d59718', '#bc830d', '#a67306', '#8d6100',
]

/* Cool neutral tuned to sit on #0a0b0e without turning muddy. */
const slate: MantineColorsTuple = [
  '#f4f6f8', '#e7eaee', '#cbd1da', '#adb6c3', '#939eae',
  '#8290a2', '#78879b', '#667488', '#5a6779', '#4b586a',
]

const allow: MantineColorsTuple = [
  '#e6fbf2', '#ccf5e3', '#99ebc7', '#66e0ab', '#3ddc97',
  '#22d489', '#12c97d', '#0aa967', '#068a54', '#046b41',
]

const deny: MantineColorsTuple = [
  '#ffecec', '#ffd6d6', '#ffadad', '#ff8585', '#ff6b6b',
  '#ff5252', '#f83b3b', '#dc2626', '#b91c1c', '#991b1b',
]

export const theme = createTheme({
  primaryColor: 'gold',
  primaryShade: { dark: 5, light: 6 },
  /* Filled gold buttons get dark text automatically — white-on-gold fails
     contrast in both schemes. */
  autoContrast: true,
  luminanceThreshold: 0.4,
  colors: { gold, slate, allow, deny },
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
    fontWeight: '600',
    sizes: {
      h1: { fontSize: rem(26), lineHeight: '1.15' },
      h2: { fontSize: rem(18), lineHeight: '1.3' },
      h3: { fontSize: rem(14), lineHeight: '1.4' },
      h4: { fontSize: rem(13), lineHeight: '1.4' },
    },
  },

  /* 4px rhythm. */
  spacing: { xs: rem(6), sm: rem(10), md: rem(16), lg: rem(24), xl: rem(36) },
  radius:  { xs: rem(4), sm: rem(6), md: rem(8), lg: rem(12), xl: rem(16) },


  components: {
    Button: {
      defaultProps: { radius: 'sm' },
      styles: { root: { fontWeight: 550, letterSpacing: '-0.005em' } },
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
          padding: '7px 10px',
        },
      },
    },
    Modal: {
      defaultProps: {
        centered: true, radius: 'md',
        overlayProps: { blur: 3, backgroundOpacity: 0.55, color: 'var(--overlay-tint)' },
        transitionProps: { transition: 'pop', duration: 180 },
      },
    },
    Popover: {
      defaultProps: { radius: 'md', shadow: 'xl', withinPortal: true,
        transitionProps: { transition: 'pop', duration: 140 } },
      styles: { dropdown: { background: 'var(--s-raised)', border: '1px solid var(--line)' } },
    },
    Input: { styles: { input: { background: 'var(--s-sunken)', borderColor: 'var(--line)' } } },
    TextInput: { defaultProps: { size: 'sm' } },
    Select: { defaultProps: { size: 'sm', comboboxProps: { shadow: 'xl' } } },
    ScrollArea: { defaultProps: { scrollbarSize: 8, type: 'hover' } },
    Divider: { styles: { root: { borderColor: 'var(--line-soft)' } } },
  },
})
