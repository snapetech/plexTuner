import {
  NavLink,
  Stack,
  Text,
  Group,
  Box,
  Divider,
  ActionIcon,
  Tooltip,
  Kbd,
  UnstyledButton,
} from '@mantine/core'
import {
  IconPlaylistAdd,
  IconMovie,
  IconAntenna,
  IconCalendarTime,
  IconRecordMail,
  IconChartBar,
  IconPuzzle,
  IconUsers,
  IconPhoto,
  IconSettings,
  IconLogout,
  IconNetwork,
  IconSearch,
} from '@tabler/icons-react'
import { useNavigate, useLocation } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { boot, api } from '../api/client'

const NAV_ITEMS = [
  { path: '/channels',     label: 'Channels',       Icon: IconPlaylistAdd  },
  { path: '/vods',         label: 'VODs',            Icon: IconMovie        },
  { path: '/m3u-epg',     label: 'M3U & EPG',       Icon: IconAntenna      },
  { path: '/tv-guide',    label: 'TV Guide',         Icon: IconCalendarTime },
  { path: '/dvr',         label: 'DVR',              Icon: IconRecordMail   },
  { path: '/stats',       label: 'Stats',            Icon: IconChartBar     },
  { path: '/plugins',     label: 'Plugins',          Icon: IconPuzzle       },
  { path: '/users',       label: 'Users',            Icon: IconUsers        },
  { path: '/logos',       label: 'Logo Manager',     Icon: IconPhoto        },
  { path: '/settings',    label: 'Settings',         Icon: IconSettings     },
]

export function AppNav({ onOpenPalette }: { onOpenPalette?: () => void }) {
  const navigate = useNavigate()
  const location = useLocation()
  const { user, port } = boot()

  const { data: discover } = useQuery({
    queryKey: ['discover'],
    queryFn: () => api.get<{ LocalIP?: string; FriendlyName?: string }>('/api/discover.json'),
    staleTime: 5 * 60_000,
    retry: false,
  })

  const tunerAddr = discover?.LocalIP
    ? `${discover.LocalIP}:${port}`
    : `localhost:${port}`

  return (
    <Stack h="100%" justify="space-between" gap={0}>
      <Box>
        <Box px="md" py="sm">
          <Text fw={700} size="lg" c="teal">IPTV Tunerr</Text>
        </Box>
        <Divider />
        <Box px="xs" pt="xs">
          <Tooltip label={<Group gap={4}><Kbd size="xs">Ctrl</Kbd><Kbd size="xs">K</Kbd></Group>} position="right">
            <UnstyledButton
              onClick={onOpenPalette}
              style={{
                width: '100%',
                padding: '6px 8px',
                borderRadius: 'var(--mantine-radius-sm)',
                border: '1px solid var(--mantine-color-dark-4)',
                color: 'var(--mantine-color-dimmed)',
                display: 'flex',
                alignItems: 'center',
                gap: 6,
              }}
            >
              <IconSearch size={13} />
              <Text size="xs" c="dimmed" style={{ flex: 1 }}>Navigate…</Text>
              <Kbd size="xs">⌘K</Kbd>
            </UnstyledButton>
          </Tooltip>
        </Box>
        <Divider mt="xs" />
        <Stack gap={2} p="xs">
          {NAV_ITEMS.map(({ path, label, Icon }) => (
            <NavLink
              key={path}
              label={label}
              leftSection={<Icon size={16} stroke={1.5} />}
              active={location.pathname.startsWith(path)}
              onClick={() => navigate(path)}
            />
          ))}
        </Stack>
      </Box>

      <Box>
        <Divider />
        <Box px="md" py="sm">
          <Group justify="space-between" align="center">
            <Stack gap={2}>
              <Tooltip label={`Tuner at ${tunerAddr}`} position="right">
                <Group gap={6} style={{ cursor: 'default' }}>
                  <IconNetwork size={12} stroke={1.5} />
                  <Text size="xs" c="dimmed">{tunerAddr}</Text>
                </Group>
              </Tooltip>
              <Text size="xs" c="dimmed" truncate maw={140}>{user || 'admin'}</Text>
            </Stack>
            <Tooltip label="Sign out">
              <ActionIcon
                variant="subtle"
                color="red"
                size="sm"
                component="a"
                href="/logout"
              >
                <IconLogout size={14} stroke={1.5} />
              </ActionIcon>
            </Tooltip>
          </Group>
        </Box>
      </Box>
    </Stack>
  )
}
