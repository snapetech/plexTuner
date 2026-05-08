import { AppShell } from '@mantine/core'
import { Outlet } from 'react-router-dom'
import { AppNav } from './components/AppNav'
import { CommandPalette, useCommandPalette } from './components/CommandPalette'

export function App() {
  const palette = useCommandPalette()

  return (
    <>
      <AppShell
        navbar={{ width: 220, breakpoint: 'sm' }}
        padding="md"
        styles={{
          navbar: { borderRight: '1px solid var(--mantine-color-dark-4)' },
          main: { backgroundColor: 'var(--mantine-color-dark-8)' },
        }}
      >
        <AppShell.Navbar>
          <AppNav onOpenPalette={palette.open} />
        </AppShell.Navbar>

        <AppShell.Main>
          <Outlet />
        </AppShell.Main>
      </AppShell>

      <CommandPalette opened={palette.opened} onClose={palette.close} />
    </>
  )
}
