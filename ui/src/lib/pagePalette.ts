/* The TypeScript half of internal/tenancy/domain/pagecfg/palette.
 *
 * Two copies of a formula is normally two chances to be wrong, and it is a
 * trade made on purpose here: the alternative is a round trip to the server on
 * every keystroke of a colour picker, and a preview that lags the input it is
 * previewing is worse than one that is occasionally a shade off. The numbers
 * are fixed constants of sRGB and WCAG, not decisions that will drift.
 *
 * If the Go changes, this changes in the same commit. Its job is that an
 * operator who picks a dark background sees the dark card they will actually
 * get, rather than the white one the builder used to promise.
 */

const DARK_THRESHOLD = 0.179

function parse(hex: string): [number, number, number] | null {
  let h = hex
  if (h.length === 4) h = `#${h[1]}${h[1]}${h[2]}${h[2]}${h[3]}${h[3]}`
  if (h.length !== 7 || h[0] !== '#') return null
  const out: number[] = []
  for (let i = 0; i < 3; i++) {
    const v = Number.parseInt(h.slice(1 + i * 2, 3 + i * 2), 16)
    if (Number.isNaN(v)) return null
    out.push(v / 255)
  }
  return [out[0]!, out[1]!, out[2]!]
}

/** WCAG relative luminance. An unparseable colour reports white, so a
    half-typed hex never flips the preview to dark mid-keystroke. */
export function luminance(hex: string): number {
  const rgb = parse(hex)
  if (!rgb) return 1
  const lin = (c: number) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)
  return 0.2126 * lin(rgb[0]) + 0.7152 * lin(rgb[1]) + 0.0722 * lin(rgb[2])
}

export function isDark(hex: string): boolean {
  return luminance(hex) < DARK_THRESHOLD
}

/** WCAG contrast ratio, 1 to 21. Body text wants 4.5, large text 3. */
export function contrast(a: string, b: string): number {
  const la = luminance(a)
  const lb = luminance(b)
  const [hi, lo] = la > lb ? [la, lb] : [lb, la]
  return (hi + 0.05) / (lo + 0.05)
}

export function mix(a: string, b: string, t: number): string {
  const ca = parse(a)
  const cb = parse(b)
  if (!ca || !cb) return a
  const ch = (x: number, y: number) =>
    Math.min(255, Math.max(0, Math.round((x + (y - x) * t) * 255)))
      .toString(16)
      .padStart(2, '0')
  return `#${ch(ca[0], cb[0])}${ch(ca[1], cb[1])}${ch(ca[2], cb[2])}`
}

/** Every colour the hosted page uses that nobody configures, derived exactly
    as pagecfg.Brand derives them. */
export function pageColors(brand: {
  primary_color: string
  background_color: string
  text_color: string
}) {
  const dark = isDark(brand.background_color)
  const surface = dark ? mix(brand.background_color, '#ffffff', 0.09) : '#ffffff'
  return {
    dark,
    surface,
    field: dark ? mix(brand.background_color, '#ffffff', 0.14) : '#ffffff',
    onPrimary: isDark(brand.primary_color) ? '#ffffff' : '#111111',
    muted: mix(brand.text_color, surface, 0.38),
    border: mix(brand.text_color, surface, 0.78),
  }
}
