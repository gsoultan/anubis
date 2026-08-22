// Single PostCSS pipeline for both design systems.
//
// Order is deliberate: postcss-preset-mantine resolves Mantine's mixins
// (light-dark(), rem(), breakpoint queries) BEFORE Tailwind scans for utility
// classes. Running Tailwind first would leave unresolved Mantine at-rules in
// the output.
module.exports = {
  plugins: {
    'postcss-preset-mantine': {},
    'postcss-simple-vars': {
      variables: {
        'mantine-breakpoint-xs': '36em',
        'mantine-breakpoint-sm': '48em',
        'mantine-breakpoint-md': '62em',
        'mantine-breakpoint-lg': '75em',
        'mantine-breakpoint-xl': '88em',
      },
    },
    '@tailwindcss/postcss': {},
  },
}
