import {
  Modal, TextInput, Stack, UnstyledButton, Group, Text, Badge, Box,
} from '@mantine/core'
import {
  IconPlaylistAdd, IconMovie, IconAntenna, IconCalendarTime, IconRecordMail,
  IconChartBar, IconPuzzle, IconUsers, IconPhoto, IconSettings, IconSearch,
  IconChevronRight,
} from '@tabler/icons-react'
import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'

const COMMANDS = [
  { path: '/channels',  label: 'Channels',      Icon: IconPlaylistAdd,   keys: 'c' },
  { path: '/vods',      label: 'VODs',           Icon: IconMovie,         keys: 'v' },
  { path: '/m3u-epg',  label: 'M3U & EPG',      Icon: IconAntenna,       keys: 'm' },
  { path: '/tv-guide', label: 'TV Guide',        Icon: IconCalendarTime,  keys: 'g' },
  { path: '/dvr',      label: 'DVR',             Icon: IconRecordMail,    keys: 'd' },
  { path: '/stats',    label: 'Stats',           Icon: IconChartBar,      keys: 's' },
  { path: '/plugins',  label: 'Plugins',         Icon: IconPuzzle,        keys: null },
  { path: '/users',    label: 'Users',           Icon: IconUsers,         keys: null },
  { path: '/logos',    label: 'Logo Manager',    Icon: IconPhoto,         keys: null },
  { path: '/settings', label: 'Settings',        Icon: IconSettings,      keys: null },
]

interface Props {
  opened: boolean
  onClose: () => void
}

export function CommandPalette({ opened, onClose }: Props) {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [cursor, setCursor] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  const filtered = COMMANDS.filter(c =>
    c.label.toLowerCase().includes(query.toLowerCase())
  )

  useEffect(() => {
    if (opened) {
      setQuery('')
      setCursor(0)
      // focus input after modal animation
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [opened])

  useEffect(() => { setCursor(0) }, [query])

  const go = useCallback((path: string) => {
    navigate(path)
    onClose()
  }, [navigate, onClose])

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setCursor(c => Math.min(c + 1, filtered.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setCursor(c => Math.max(c - 1, 0))
    } else if (e.key === 'Enter' && filtered[cursor]) {
      go(filtered[cursor].path)
    } else if (e.key === 'Escape') {
      onClose()
    }
  }

  return (
    <Modal
      opened={opened}
      onClose={onClose}
      withCloseButton={false}
      size="sm"
      padding={0}
      styles={{
        content: { overflow: 'hidden' },
        body: { padding: 0 },
      }}
    >
      <Box p="sm" style={{ borderBottom: '1px solid var(--mantine-color-dark-4)' }}>
        <TextInput
          ref={inputRef}
          leftSection={<IconSearch size={16} />}
          placeholder="Navigate to…"
          value={query}
          onChange={e => setQuery(e.currentTarget.value)}
          onKeyDown={onKeyDown}
          variant="unstyled"
          size="md"
        />
      </Box>
      <Stack gap={0} p="xs">
        {filtered.length === 0 ? (
          <Text size="sm" c="dimmed" ta="center" py="md">No results</Text>
        ) : (
          filtered.map((cmd, i) => {
            const { Icon } = cmd
            const active = i === cursor
            return (
              <UnstyledButton
                key={cmd.path}
                onClick={() => go(cmd.path)}
                onMouseEnter={() => setCursor(i)}
                style={{
                  borderRadius: 'var(--mantine-radius-sm)',
                  padding: '6px 8px',
                  background: active ? 'var(--mantine-color-dark-5)' : 'transparent',
                }}
              >
                <Group justify="space-between" wrap="nowrap">
                  <Group gap="xs" wrap="nowrap">
                    <Icon size={16} style={{ opacity: 0.7, flexShrink: 0 }} />
                    <Text size="sm">{cmd.label}</Text>
                  </Group>
                  <Group gap="xs" wrap="nowrap">
                    {cmd.keys && (
                      <Badge size="xs" variant="outline" color="gray">{cmd.keys}</Badge>
                    )}
                    {active && <IconChevronRight size={14} style={{ opacity: 0.4 }} />}
                  </Group>
                </Group>
              </UnstyledButton>
            )
          })
        )}
      </Stack>
      <Box
        px="sm"
        py="xs"
        style={{ borderTop: '1px solid var(--mantine-color-dark-4)', display: 'flex', gap: 12 }}
      >
        {[['↑↓', 'Navigate'], ['↵', 'Go'], ['Esc', 'Close']].map(([key, label]) => (
          <Group key={key} gap={4}>
            <Badge size="xs" variant="outline" color="gray">{key}</Badge>
            <Text size="xs" c="dimmed">{label}</Text>
          </Group>
        ))}
      </Box>
    </Modal>
  )
}

// Hook: registers Ctrl+K / Cmd+K globally + single-letter nav shortcuts.
export function useCommandPalette() {
  const [opened, setOpened] = useState(false)
  const navigate = useNavigate()

  useEffect(() => {
    function handler(e: KeyboardEvent) {
      const tag = (e.target as HTMLElement).tagName
      const inInput = tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement).isContentEditable

      // Ctrl+K / Cmd+K → open palette
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault()
        setOpened(o => !o)
        return
      }

      // Single-letter shortcuts only when not in an input
      if (inInput || e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return

      const shortcut = COMMANDS.find(c => c.keys === e.key)
      if (shortcut) {
        e.preventDefault()
        navigate(shortcut.path)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [navigate])

  return { opened, open: () => setOpened(true), close: () => setOpened(false) }
}
