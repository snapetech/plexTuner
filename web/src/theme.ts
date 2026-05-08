import { createTheme, MantineColorsTuple } from '@mantine/core'

const teal: MantineColorsTuple = [
  '#e6fcf5',
  '#c3fae8',
  '#96f2d7',
  '#63e6be',
  '#38d9a9',
  '#20c997',
  '#12b886',
  '#0ca678',
  '#099268',
  '#087f5b',
]

export const theme = createTheme({
  primaryColor: 'teal',
  colors: { teal },
  defaultRadius: 'sm',
  fontFamily: 'Inter, system-ui, -apple-system, sans-serif',
  components: {
    NavLink: {
      defaultProps: {
        variant: 'subtle',
      },
    },
  },
})
