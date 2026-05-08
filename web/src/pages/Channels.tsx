import '@mantine/notifications/styles.css'

import {
  Stack, Group, Select, ActionIcon, Tooltip, Text, Checkbox, Badge,
  Button, TextInput, Divider, Box, ScrollArea, Table, Menu,
  Paper, Skeleton, Alert, Tabs, Modal,
} from '@mantine/core'
import {
  IconSquarePlus, IconSquareMinus, IconEdit, IconFilter, IconDots,
  IconPlayerPlay, IconRefresh, IconBinaryTree, IconAlertCircle,
  IconGripVertical, IconLayoutDashboard, IconList, IconDeviceTv, IconEyeOff,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  DndContext, closestCenter, type DragEndEvent,
  PointerSensor, useSensor, useSensors,
} from '@dnd-kit/core'
import {
  SortableContext, verticalListSortingStrategy,
  useSortable, arrayMove,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useState, useMemo, useCallback } from 'react'

import {
  channelsApi, streamsApi, profilesApi, groupsApi, autoMatchApi,
  type Channel, type Stream, type ChannelProfile,
} from '../api/channels'
import { ChannelEditModal } from '../components/channels/ChannelEditModal'
import { BulkEditModal } from '../components/channels/BulkEditModal'
import { AutoMatchModal } from '../components/channels/AutoMatchModal'
import { LinksFooter } from '../components/channels/LinksFooter'
import { PreviewDrawer } from '../components/channels/PreviewDrawer'
import { api } from '../api/client'

// ──────────────────────────────────────────────────────────────────
// Drag-and-drop row component
// ──────────────────────────────────────────────────────────────────
function SortableRow({
  channel, selected, onSelect, onEdit, onDelete, onPreview, onRowClick, isActive,
}: {
  channel: Channel
  selected: boolean
  onSelect: (id: number, checked: boolean) => void
  onEdit: (ch: Channel) => void
  onDelete: (id: number) => void
  onPreview: (ch: Channel) => void
  onRowClick: (ch: Channel) => void
  isActive: boolean
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
    useSortable({ id: channel.id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    backgroundColor: isActive
      ? 'var(--mantine-color-dark-5)'
      : selected
        ? 'var(--mantine-color-dark-6)'
        : undefined,
  }

  return (
    <Table.Tr ref={setNodeRef} style={style} onClick={() => onRowClick(channel)}>
      <Table.Td onClick={e => e.stopPropagation()} style={{ width: 36 }}>
        <Checkbox
          size="xs"
          checked={selected}
          onChange={e => onSelect(channel.id, e.currentTarget.checked)}
        />
      </Table.Td>
      <Table.Td style={{ width: 28, cursor: 'grab' }} {...attributes} {...listeners}
        onClick={e => e.stopPropagation()}>
        <IconGripVertical size={14} color="var(--mantine-color-dark-3)" />
      </Table.Td>
      <Table.Td style={{ width: 60 }}>
        <Text size="xs" c="dimmed">{channel.channel_number || '—'}</Text>
      </Table.Td>
      <Table.Td>
        <Text size="sm" lineClamp={1}>{channel.name}</Text>
      </Table.Td>
      <Table.Td>
        <Text size="xs" c="dimmed" lineClamp={1}>{channel.epg_name || '—'}</Text>
      </Table.Td>
      <Table.Td>
        <Text size="xs" c="dimmed" lineClamp={1}>{channel.group_name || '—'}</Text>
      </Table.Td>
      <Table.Td style={{ width: 80 }} onClick={e => e.stopPropagation()}>
        <Group gap={4} wrap="nowrap">
          <Tooltip label="Edit">
            <ActionIcon size="xs" variant="subtle" color="yellow" onClick={() => onEdit(channel)}>
              <IconEdit size={14} />
            </ActionIcon>
          </Tooltip>
          <Tooltip label="Preview">
            <ActionIcon size="xs" variant="subtle" color="teal" onClick={() => onPreview(channel)}>
              <IconPlayerPlay size={14} />
            </ActionIcon>
          </Tooltip>
          <Tooltip label="Delete">
            <ActionIcon size="xs" variant="subtle" color="red" onClick={() => onDelete(channel.id)}>
              <IconSquareMinus size={14} />
            </ActionIcon>
          </Tooltip>
        </Group>
      </Table.Td>
    </Table.Tr>
  )
}

// ──────────────────────────────────────────────────────────────────
// Streams pane row
// ──────────────────────────────────────────────────────────────────
function StreamRow({
  stream, selectedChannelIds, onAddToChannel, onPreview, onDelete,
}: {
  stream: Stream
  selectedChannelIds: number[]
  onAddToChannel: (streamId: number, channelId: number) => void
  onPreview: (st: Stream) => void
  onDelete: (id: number) => void
}) {
  return (
    <Table.Tr style={{ opacity: stream.stale ? 0.5 : 1 }}>
      <Table.Td>
        <Text size="sm" lineClamp={1}>{stream.name || stream.url}</Text>
        {stream.stale && <Badge size="xs" color="red" variant="outline">stale</Badge>}
      </Table.Td>
      <Table.Td><Text size="xs" c="dimmed">{stream.m3u_name || '—'}</Text></Table.Td>
      <Table.Td style={{ width: 100 }}>
        <Group gap={4} wrap="nowrap">
          <Tooltip label="Preview">
            <ActionIcon size="xs" variant="subtle" color="teal" onClick={() => onPreview(stream)}>
              <IconPlayerPlay size={14} />
            </ActionIcon>
          </Tooltip>
          {selectedChannelIds.length > 0 && (
            <Tooltip label={`Add to ${selectedChannelIds.length} channel${selectedChannelIds.length > 1 ? 's' : ''}`}>
              <ActionIcon size="xs" variant="subtle" color="blue"
                onClick={() => selectedChannelIds.forEach(cid => onAddToChannel(stream.id, cid))}>
                <IconSquarePlus size={14} />
              </ActionIcon>
            </Tooltip>
          )}
          <Tooltip label="Delete">
            <ActionIcon size="xs" variant="subtle" color="red" onClick={() => onDelete(stream.id)}>
              <IconSquareMinus size={14} />
            </ActionIcon>
          </Tooltip>
        </Group>
      </Table.Td>
    </Table.Tr>
  )
}

// ──────────────────────────────────────────────────────────────────
// Profile dropdown with management actions
// ──────────────────────────────────────────────────────────────────
function ProfileSelector({
  profiles, selected, onChange, onCreate, onRename, onDelete, onDuplicate,
}: {
  profiles: ChannelProfile[]
  selected: number | null
  onChange: (id: number | null) => void
  onCreate: (name: string) => void
  onRename: (id: number, name: string) => void
  onDelete: (id: number) => void
  onDuplicate: (id: number, name: string) => void
}) {
  const options = [
    { value: 'all', label: 'All Channels' },
    ...profiles.map(p => ({ value: String(p.id), label: p.name })),
  ]

  return (
    <Group gap="xs" align="center">
      <Text size="sm" fw={500}>Channels</Text>
      <Select
        size="xs"
        data={options}
        value={selected ? String(selected) : 'all'}
        onChange={v => onChange(v && v !== 'all' ? Number(v) : null)}
        style={{ width: 180 }}
      />
      <Tooltip label="New profile">
        <ActionIcon size="sm" variant="subtle" color="teal"
          onClick={() => {
            const name = prompt('Profile name:')
            if (name) onCreate(name)
          }}>
          <IconSquarePlus size={16} />
        </ActionIcon>
      </Tooltip>
      {selected && (
        <Menu shadow="md" width={180}>
          <Menu.Target>
            <ActionIcon size="sm" variant="subtle"><IconDots size={16} /></ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Item onClick={() => {
              const name = prompt('Duplicate as:')
              if (name) onDuplicate(selected, name)
            }}>Duplicate</Menu.Item>
            <Menu.Item onClick={() => {
              const p = profiles.find(x => x.id === selected)
              const name = prompt('Rename to:', p?.name)
              if (name) onRename(selected, name)
            }}>Rename</Menu.Item>
            <Menu.Divider />
            <Menu.Item color="red" onClick={() => {
              if (confirm('Delete this profile?')) onDelete(selected)
            }}>Delete</Menu.Item>
          </Menu.Dropdown>
        </Menu>
      )}
    </Group>
  )
}

// ──────────────────────────────────────────────────────────────────
// Virtual Channels tab
// ──────────────────────────────────────────────────────────────────
function VirtualTab() {
  const [brandingTarget, setBrandingTarget] = useState<Record<string,unknown> | null>(null)
  const [logoUrl, setLogoUrl] = useState('')
  const [bugText, setBugText] = useState('')
  const [themeColor, setThemeColor] = useState('')

  const report = useQuery({
    queryKey: ['virtual-report'],
    queryFn: () => api.get<Record<string, unknown>>('/api/virtual-channels/report.json'),
    staleTime: 30_000,
  })
  const schedule = useQuery({
    queryKey: ['virtual-schedule'],
    queryFn: () => api.get<Record<string, unknown>>('/api/virtual-channels/schedule.json?horizon=3h'),
    staleTime: 30_000,
  })
  const recovery = useQuery({
    queryKey: ['virtual-recovery'],
    queryFn: () => api.get<Record<string, unknown>>('/api/virtual-channels/recovery-report.json?limit=8'),
    staleTime: 30_000,
  })

  const updateMeta = useMutation({
    mutationFn: ({ id, ...fields }: Record<string, unknown>) =>
      api.post('/api/virtual-channels/channel-detail.json', { action: 'update_metadata', channel_id: id, ...fields }),
    onSuccess: () => {
      report.refetch()
      notifications.show({ message: 'Channel metadata updated', color: 'teal' })
      setBrandingTarget(null)
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const stations = Array.isArray(report.data) ? report.data as Record<string,unknown>[] :
    Array.isArray((report.data as Record<string,unknown>|null)?.stations) ? (report.data as Record<string,unknown>)!.stations as Record<string,unknown>[] : []
  const slots = Array.isArray(schedule.data) ? schedule.data as Record<string,unknown>[] :
    Array.isArray((schedule.data as Record<string,unknown>|null)?.slots) ? (schedule.data as Record<string,unknown>)!.slots as Record<string,unknown>[] : []
  const events = Array.isArray(recovery.data) ? recovery.data as Record<string,unknown>[] :
    Array.isArray((recovery.data as Record<string,unknown>|null)?.events) ? (recovery.data as Record<string,unknown>)!.events as Record<string,unknown>[] : []

  return (
    <ScrollArea mah="calc(100vh - 200px)">
      <Stack gap="md">
        {/* Station list */}
        <Paper withBorder p="md">
          <Text fw={600} mb="sm">Virtual Stations</Text>
          {report.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : report.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Virtual channels unavailable.</Alert>
            : stations.length === 0 ? <Text size="sm" c="dimmed">No virtual channels configured.</Text>
            : (
              <Table striped withRowBorders={false} fz="sm">
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>Name</Table.Th>
                    <Table.Th>Stream mode</Table.Th>
                    <Table.Th>Recovery mode</Table.Th>
                    <Table.Th>Recovery events</Table.Th>
                    <Table.Th style={{ width: 60 }} />
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {stations.map((s, i) => (
                    <Table.Tr key={i}>
                      <Table.Td>
                        <Group gap="xs">
                          {!!s.logo_url && <img src={String(s.logo_url)} alt="" style={{ height: 20, width: 'auto' }} />}
                          <Text size="sm">{String(s.name ?? '—')}</Text>
                        </Group>
                      </Table.Td>
                      <Table.Td><Badge size="xs" variant="outline">{String(s.stream_mode ?? '—')}</Badge></Table.Td>
                      <Table.Td><Badge size="xs" color="blue" variant="outline">{String(s.recovery_mode ?? '—')}</Badge></Table.Td>
                      <Table.Td c="dimmed">{String(Array.isArray(s.recovery_events) ? s.recovery_events.length : s.recovery_event_count ?? '0')}</Table.Td>
                      <Table.Td>
                        <Tooltip label="Edit branding">
                          <ActionIcon size="xs" variant="subtle" color="yellow"
                            onClick={() => {
                              setBrandingTarget(s)
                              setLogoUrl(String(s.logo_url ?? ''))
                              setBugText(String(s.bug_text ?? ''))
                              setThemeColor(String(s.theme_color ?? ''))
                            }}>
                            <IconEdit size={14} />
                          </ActionIcon>
                        </Tooltip>
                      </Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            )}
        </Paper>

        {/* Schedule */}
        <Paper withBorder p="md">
          <Text fw={600} mb="sm">Upcoming Schedule (3h)</Text>
          {schedule.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : schedule.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Schedule unavailable.</Alert>
            : slots.length === 0 ? <Text size="sm" c="dimmed">No upcoming slots.</Text>
            : (
              <Table striped withRowBorders={false} fz="sm">
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>Channel</Table.Th>
                    <Table.Th>Title</Table.Th>
                    <Table.Th>Start</Table.Th>
                    <Table.Th>End</Table.Th>
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {slots.map((sl, i) => (
                    <Table.Tr key={i}>
                      <Table.Td>{String(sl.channel_name ?? sl.channel ?? '—')}</Table.Td>
                      <Table.Td>{String(sl.title ?? '—')}</Table.Td>
                      <Table.Td c="dimmed">{sl.start ? new Date(String(sl.start)).toLocaleTimeString() : '—'}</Table.Td>
                      <Table.Td c="dimmed">{sl.end ? new Date(String(sl.end)).toLocaleTimeString() : '—'}</Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            )}
        </Paper>

        {/* Recovery */}
        <Paper withBorder p="md">
          <Text fw={600} mb="sm">Recent Recovery Events</Text>
          {recovery.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : recovery.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Recovery report unavailable.</Alert>
            : events.length === 0 ? <Text size="sm" c="dimmed">No recent recovery events.</Text>
            : (
              <Table striped withRowBorders={false} fz="sm">
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>Channel</Table.Th>
                    <Table.Th>Reason</Table.Th>
                    <Table.Th>When</Table.Th>
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {events.map((ev, i) => (
                    <Table.Tr key={i}>
                      <Table.Td>{String(ev.channel_name ?? ev.channel ?? '—')}</Table.Td>
                      <Table.Td>{String(ev.reason ?? ev.type ?? '—')}</Table.Td>
                      <Table.Td c="dimmed">{ev.at ? new Date(String(ev.at)).toLocaleTimeString() : '—'}</Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            )}
        </Paper>
      </Stack>

      {/* Branding modal */}
      <Modal opened={!!brandingTarget} onClose={() => setBrandingTarget(null)}
        title={`Branding — ${String(brandingTarget?.name ?? '')}`} size="sm">
        <Stack gap="sm">
          <TextInput label="Logo URL" value={logoUrl} onChange={e => setLogoUrl(e.currentTarget.value)} />
          <TextInput label="Bug text (overlay)" value={bugText} onChange={e => setBugText(e.currentTarget.value)} />
          <TextInput label="Theme color (hex)" value={themeColor} onChange={e => setThemeColor(e.currentTarget.value)} placeholder="#1a1a2e" />
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setBrandingTarget(null)}>Cancel</Button>
            <Button color="teal" loading={updateMeta.isPending}
              onClick={() => updateMeta.mutate({
                id: brandingTarget?.id ?? brandingTarget?.channel_id,
                logo_url: logoUrl,
                bug_text: bugText,
                theme_color: themeColor,
              })}>
              Save
            </Button>
          </Group>
        </Stack>
      </Modal>
    </ScrollArea>
  )
}

// ──────────────────────────────────────────────────────────────────
// Lineup tab
// ──────────────────────────────────────────────────────────────────
function LineupTab() {
  const qc = useQueryClient()

  const categories = useQuery({
    queryKey: ['programming-categories'],
    queryFn: () => api.get<Record<string, unknown>>('/api/programming/categories.json'),
    staleTime: 60_000,
  })
  const recipe = useQuery({
    queryKey: ['programming-recipe'],
    queryFn: () => api.get<Record<string, unknown>>('/api/programming/recipe.json'),
    staleTime: 60_000,
  })
  const preview = useQuery({
    queryKey: ['programming-preview'],
    queryFn: () => api.get<Record<string, unknown>>('/api/programming/preview.json?limit=50'),
    staleTime: 60_000,
  })
  const backups = useQuery({
    queryKey: ['programming-backups'],
    queryFn: () => api.get<Record<string, unknown>>('/api/programming/backups.json'),
    staleTime: 60_000,
  })

  const categoryAction = useMutation({
    mutationFn: ({ action, category_id }: { action: string; category_id: unknown }) =>
      api.post('/api/programming/recipe.json', { action, category_id }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['programming-categories'] })
      qc.invalidateQueries({ queryKey: ['programming-recipe'] })
      notifications.show({ message: 'Recipe updated', color: 'teal' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const harvest = useMutation({
    mutationFn: () => api.post('/api/programming/harvest-request.json'),
    onSuccess: () => notifications.show({ message: 'Plex harvest requested', color: 'teal' }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const catList = Array.isArray(categories.data) ? categories.data as Record<string,unknown>[] :
    Array.isArray((categories.data as Record<string,unknown>|null)?.categories) ? (categories.data as Record<string,unknown>)!.categories as Record<string,unknown>[] : []
  const rec = recipe.data as Record<string, unknown> | undefined
  const previewList = Array.isArray(preview.data) ? preview.data as Record<string,unknown>[] :
    Array.isArray((preview.data as Record<string,unknown>|null)?.channels) ? (preview.data as Record<string,unknown>)!.channels as Record<string,unknown>[] : []
  const backupList = Array.isArray(backups.data) ? backups.data as Record<string,unknown>[] :
    Array.isArray((backups.data as Record<string,unknown>|null)?.groups) ? (backups.data as Record<string,unknown>)!.groups as Record<string,unknown>[] : []

  const includedCats = Array.isArray(rec?.selected_categories) ? rec!.selected_categories as unknown[] : []
  const excludedCats = Array.isArray(rec?.excluded_categories) ? rec!.excluded_categories as unknown[] : []

  return (
    <ScrollArea mah="calc(100vh - 200px)">
      <Stack gap="md">
        <Group justify="flex-end">
          <Button size="xs" color="violet"
            onClick={() => { if (confirm('Request a Plex channel harvest?')) harvest.mutate() }}
            loading={harvest.isPending}>
            Request Plex Harvest
          </Button>
        </Group>

        {/* Categories */}
        <Paper withBorder p="md">
          <Text fw={600} mb="sm">Categories</Text>
          {categories.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : categories.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Categories unavailable.</Alert>
            : catList.length === 0 ? <Text size="sm" c="dimmed">No categories found.</Text>
            : (
              <Table striped withRowBorders={false} fz="sm">
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>Category</Table.Th>
                    <Table.Th>Channels</Table.Th>
                    <Table.Th>Status</Table.Th>
                    <Table.Th style={{ width: 120 }} />
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {catList.map((cat, i) => {
                    const id = cat.id ?? cat.category_id
                    const isIncluded = includedCats.includes(id)
                    const isExcluded = excludedCats.includes(id)
                    return (
                      <Table.Tr key={i}>
                        <Table.Td>{String(cat.name ?? cat.category ?? '—')}</Table.Td>
                        <Table.Td c="dimmed">{String(cat.channel_count ?? cat.count ?? '—')}</Table.Td>
                        <Table.Td>
                          {isIncluded && <Badge size="xs" color="teal">Included</Badge>}
                          {isExcluded && <Badge size="xs" color="red">Excluded</Badge>}
                          {!isIncluded && !isExcluded && <Badge size="xs" color="gray">Default</Badge>}
                        </Table.Td>
                        <Table.Td>
                          <Group gap={4}>
                            <Button size="xs" variant="subtle" color="teal"
                              onClick={() => categoryAction.mutate({ action: 'include', category_id: id })}>
                              +
                            </Button>
                            <Button size="xs" variant="subtle" color="red"
                              onClick={() => categoryAction.mutate({ action: 'exclude', category_id: id })}>
                              −
                            </Button>
                            <Button size="xs" variant="subtle" color="gray"
                              onClick={() => categoryAction.mutate({ action: 'remove', category_id: id })}>
                              Reset
                            </Button>
                          </Group>
                        </Table.Td>
                      </Table.Tr>
                    )
                  })}
                </Table.Tbody>
              </Table>
            )}
        </Paper>

        {/* Backup groups */}
        {backupList.length > 0 && (
          <Paper withBorder p="md">
            <Text fw={600} mb="sm">Backup Groups</Text>
            <Table striped withRowBorders={false} fz="sm">
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>Group</Table.Th>
                  <Table.Th>Sources</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {backupList.map((g, i) => (
                  <Table.Tr key={i}>
                    <Table.Td>{String(g.name ?? g.group ?? '—')}</Table.Td>
                    <Table.Td c="dimmed">{String(Array.isArray(g.sources) ? g.sources.length : g.source_count ?? '—')}</Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </Paper>
        )}

        {/* Preview */}
        <Paper withBorder p="md">
          <Text fw={600} mb="sm">Curated Preview (what Plex sees)</Text>
          {preview.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : preview.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Preview unavailable.</Alert>
            : previewList.length === 0 ? <Text size="sm" c="dimmed">No channels in curated lineup.</Text>
            : (
              <ScrollArea mah={400}>
                <Table striped withRowBorders={false} fz="sm">
                  <Table.Thead style={{ position: 'sticky', top: 0, background: 'var(--mantine-color-dark-7)', zIndex: 1 }}>
                    <Table.Tr>
                      <Table.Th>#</Table.Th>
                      <Table.Th>Name</Table.Th>
                      <Table.Th>Source</Table.Th>
                    </Table.Tr>
                  </Table.Thead>
                  <Table.Tbody>
                    {previewList.map((ch, i) => (
                      <Table.Tr key={i}>
                        <Table.Td c="dimmed">{String(ch.guide_number ?? ch.channel_number ?? i + 1)}</Table.Td>
                        <Table.Td>{String(ch.guide_name ?? ch.name ?? '—')}</Table.Td>
                        <Table.Td c="dimmed">{String(ch.source_tag ?? ch.source ?? '—')}</Table.Td>
                      </Table.Tr>
                    ))}
                  </Table.Tbody>
                </Table>
              </ScrollArea>
            )}
        </Paper>
      </Stack>
    </ScrollArea>
  )
}

// ──────────────────────────────────────────────────────────────────
// Main channel manager (two-pane)
// ──────────────────────────────────────────────────────────────────
function ChannelManagerTab() {
  const qc = useQueryClient()
  const [activeProfileId, setActiveProfileId] = useState<number | null>(null)
  const [activeChannelId, setActiveChannelId] = useState<number | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [channelSearch, setChannelSearch] = useState('')
  const [streamSearch, setStreamSearch] = useState('')
  const [streamOnlyUnassigned, setStreamOnlyUnassigned] = useState(false)
  const [streamHideStale, setStreamHideStale] = useState(false)
  const [editTarget, setEditTarget] = useState<Partial<Channel> | null>(null)
  const [bulkOpen, setBulkOpen] = useState(false)
  const [autoMatchOpen, setAutoMatchOpen] = useState(false)
  const [previewTarget, setPreviewTarget] = useState<Channel | Stream | null>(null)
  const [channelOrder, setChannelOrder] = useState<number[]>([])

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))

  // ── Queries ──────────────────────────────────────────────────────
  const { data: profilesData } = useQuery({
    queryKey: ['profiles'],
    queryFn: () => profilesApi.list(),
  })
  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: () => groupsApi.list(),
  })
  const { data: channelsData, isLoading: channelsLoading, error: channelsError } = useQuery({
    queryKey: ['channels', activeProfileId, channelSearch],
    queryFn: () => channelsApi.list({
      profile_id: activeProfileId ?? undefined,
      search: channelSearch || undefined,
    }),
    placeholderData: prev => prev,
  })
  const { data: streamsData, isLoading: streamsLoading } = useQuery({
    queryKey: ['streams', streamSearch, streamOnlyUnassigned, streamHideStale],
    queryFn: () => streamsApi.list({
      search: streamSearch || undefined,
      unassigned: streamOnlyUnassigned,
      hide_stale: streamHideStale,
    }),
    placeholderData: prev => prev,
  })

  const channels = useMemo(() => {
    const list = channelsData?.channels ?? []
    if (channelOrder.length === 0) return list
    const byId = Object.fromEntries(list.map(c => [c.id, c]))
    return channelOrder.map(id => byId[id]).filter(Boolean) as Channel[]
  }, [channelsData, channelOrder])

  const streams = streamsData?.streams ?? []
  const profiles = profilesData ?? []
  const groups = groupsData ?? []

  // ── Mutations ────────────────────────────────────────────────────
  const invalidate = useCallback(() => {
    qc.invalidateQueries({ queryKey: ['channels'] })
    qc.invalidateQueries({ queryKey: ['streams'] })
  }, [qc])

  const createChannel = useMutation({
    mutationFn: (ch: Partial<Channel>) => channelsApi.create(ch),
    onSuccess: () => { invalidate(); setEditTarget(null) },
    onError: e => notifications.show({ color: 'red', message: String(e) }),
  })
  const updateChannel = useMutation({
    mutationFn: ({ id, ch }: { id: number; ch: Partial<Channel> }) => channelsApi.update(id, ch),
    onSuccess: () => { invalidate(); setEditTarget(null) },
    onError: e => notifications.show({ color: 'red', message: String(e) }),
  })
  const deleteChannel = useMutation({
    mutationFn: (id: number) => channelsApi.delete(id),
    onSuccess: invalidate,
    onError: e => notifications.show({ color: 'red', message: String(e) }),
  })
  const reorderChannels = useMutation({
    mutationFn: (ids: number[]) => channelsApi.reorder(ids),
    onError: e => notifications.show({ color: 'red', message: String(e) }),
  })
  const bulkUpdate = useMutation({
    mutationFn: ({ ids, update }: { ids: number[]; update: Partial<Channel> & { clear_epg?: boolean } }) =>
      channelsApi.bulk(ids, update),
    onSuccess: () => { invalidate(); setBulkOpen(false); setSelectedIds(new Set()) },
    onError: e => notifications.show({ color: 'red', message: String(e) }),
  })
  const deleteStream = useMutation({
    mutationFn: (id: number) => streamsApi.delete(id),
    onSuccess: invalidate,
    onError: e => notifications.show({ color: 'red', message: String(e) }),
  })
  const assignStream = useMutation({
    mutationFn: ({ streamId, channelId }: { streamId: number; channelId: number }) =>
      streamsApi.assign(streamId, channelId),
    onSuccess: invalidate,
    onError: e => notifications.show({ color: 'red', message: String(e) }),
  })
  const createProfile = useMutation({
    mutationFn: (name: string) => profilesApi.create(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['profiles'] }),
  })
  const renameProfile = useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) => profilesApi.rename(id, name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['profiles'] }),
  })
  const deleteProfile = useMutation({
    mutationFn: (id: number) => profilesApi.delete(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['profiles'] }); setActiveProfileId(null) },
  })
  const duplicateProfile = useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) => profilesApi.duplicate(id, name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['profiles'] }),
  })

  // ── Drag and drop ────────────────────────────────────────────────
  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldOrder = channelOrder.length > 0 ? channelOrder : channels.map(c => c.id)
    const from = oldOrder.indexOf(Number(active.id))
    const to   = oldOrder.indexOf(Number(over.id))
    const next = arrayMove(oldOrder, from, to)
    setChannelOrder(next)
    reorderChannels.mutate(next)
  }

  // ── Selection ────────────────────────────────────────────────────
  function toggleSelect(id: number, checked: boolean) {
    setSelectedIds(prev => {
      const next = new Set(prev)
      checked ? next.add(id) : next.delete(id)
      return next
    })
  }
  function toggleSelectAll(checked: boolean) {
    setSelectedIds(checked ? new Set(channels.map(c => c.id)) : new Set())
  }
  const allSelected = channels.length > 0 && selectedIds.size === channels.length
  const someSelected = selectedIds.size > 0 && !allSelected

  // ── Save handler for edit modal ──────────────────────────────────
  async function saveChannel(ch: Partial<Channel>) {
    if (ch.id) {
      await updateChannel.mutateAsync({ id: ch.id, ch })
    } else {
      await createChannel.mutateAsync(ch)
    }
  }

  // ── Channel IDs for dnd context ──────────────────────────────────
  const channelIds = channels.map(c => c.id)

  return (
    <Stack gap="sm" h="100%" style={{ overflow: 'hidden' }}>
      {/* ── Toolbar ─────────────────────────────────────────────── */}
      <Group justify="space-between" align="center" wrap="nowrap">
        <ProfileSelector
          profiles={profiles}
          selected={activeProfileId}
          onChange={setActiveProfileId}
          onCreate={name => createProfile.mutate(name)}
          onRename={(id, name) => renameProfile.mutate({ id, name })}
          onDelete={id => deleteProfile.mutate(id)}
          onDuplicate={(id, name) => duplicateProfile.mutate({ id, name })}
        />

        <Group gap="xs">
          {selectedIds.size > 0 && (
            <>
              <Badge color="teal" variant="light">{selectedIds.size} selected</Badge>
              <Button size="xs" variant="subtle" leftSection={<IconEdit size={14} />}
                onClick={() => setBulkOpen(true)}>
                Edit
              </Button>
              <Button size="xs" variant="subtle" color="red" leftSection={<IconSquareMinus size={14} />}
                onClick={() => {
                  if (confirm(`Delete ${selectedIds.size} channels?`)) {
                    selectedIds.forEach(id => deleteChannel.mutate(id))
                    setSelectedIds(new Set())
                  }
                }}>
                Delete
              </Button>
            </>
          )}
          <Tooltip label="Auto-match EPG">
            <ActionIcon size="sm" variant="subtle" onClick={() => setAutoMatchOpen(true)}>
              <IconBinaryTree size={16} />
            </ActionIcon>
          </Tooltip>
          <Tooltip label="New channel">
            <ActionIcon size="sm" variant="subtle" color="teal"
              onClick={() => setEditTarget({})}>
              <IconSquarePlus size={16} />
            </ActionIcon>
          </Tooltip>
          <Tooltip label="Refresh">
            <ActionIcon size="sm" variant="subtle" onClick={invalidate}>
              <IconRefresh size={16} />
            </ActionIcon>
          </Tooltip>
        </Group>
      </Group>

      {/* ── Two-pane table area ─────────────────────────────────── */}
      <Group align="flex-start" gap="sm" style={{ flex: 1, overflow: 'hidden', minHeight: 0 }}>

        {/* Left — Channels */}
        <Paper withBorder style={{ flex: '0 0 58%', display: 'flex', flexDirection: 'column', height: '100%' }}>
          <Box p="xs" pb={4}>
            <TextInput
              size="xs"
              placeholder="Search channels…"
              leftSection={<IconFilter size={12} />}
              value={channelSearch}
              onChange={e => setChannelSearch(e.currentTarget.value)}
            />
          </Box>
          <Divider />
          <ScrollArea style={{ flex: 1 }}>
            {channelsError && (
              <Alert icon={<IconAlertCircle size={16} />} color="red" m="sm">
                {String(channelsError)}
              </Alert>
            )}
            <Table striped highlightOnHover withRowBorders={false} fz="sm">
              <Table.Thead style={{ position: 'sticky', top: 0, zIndex: 1, background: 'var(--mantine-color-dark-7)' }}>
                <Table.Tr>
                  <Table.Th style={{ width: 36 }}>
                    <Checkbox size="xs" checked={allSelected} indeterminate={someSelected}
                      onChange={e => toggleSelectAll(e.currentTarget.checked)} />
                  </Table.Th>
                  <Table.Th style={{ width: 28 }} />
                  <Table.Th style={{ width: 60 }}>#</Table.Th>
                  <Table.Th>Name</Table.Th>
                  <Table.Th>EPG</Table.Th>
                  <Table.Th>Group</Table.Th>
                  <Table.Th style={{ width: 80 }} />
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {channelsLoading ? (
                  Array.from({ length: 8 }).map((_, i) => (
                    <Table.Tr key={i}>
                      {Array.from({ length: 7 }).map((_, j) => (
                        <Table.Td key={j}><Skeleton h={16} /></Table.Td>
                      ))}
                    </Table.Tr>
                  ))
                ) : (
                  <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
                    <SortableContext items={channelIds} strategy={verticalListSortingStrategy}>
                      {channels.map(ch => (
                        <SortableRow
                          key={ch.id}
                          channel={ch}
                          selected={selectedIds.has(ch.id)}
                          onSelect={toggleSelect}
                          onEdit={setEditTarget}
                          onDelete={id => {
                            if (confirm(`Delete "${ch.name}"?`)) deleteChannel.mutate(id)
                          }}
                          onPreview={setPreviewTarget}
                          onRowClick={c => setActiveChannelId(c.id === activeChannelId ? null : c.id)}
                          isActive={ch.id === activeChannelId}
                        />
                      ))}
                    </SortableContext>
                  </DndContext>
                )}
                {!channelsLoading && channels.length === 0 && (
                  <Table.Tr>
                    <Table.Td colSpan={7}>
                      <Text c="dimmed" size="sm" ta="center" py="md">
                        No channels. Add an M3U account and import streams, or create channels manually.
                      </Text>
                    </Table.Td>
                  </Table.Tr>
                )}
              </Table.Tbody>
            </Table>
          </ScrollArea>
          <Divider />
          <Box p="xs">
            <Text size="xs" c="dimmed">
              {channelsData?.total ?? 0} channel{(channelsData?.total ?? 0) !== 1 ? 's' : ''}
            </Text>
          </Box>
        </Paper>

        {/* Right — Streams */}
        <Paper withBorder style={{ flex: 1, display: 'flex', flexDirection: 'column', height: '100%' }}>
          <Box p="xs" pb={4}>
            <Group gap="xs">
              <TextInput
                size="xs"
                placeholder="Search streams…"
                leftSection={<IconFilter size={12} />}
                value={streamSearch}
                onChange={e => setStreamSearch(e.currentTarget.value)}
                style={{ flex: 1 }}
              />
              <Tooltip label="Only unassigned">
                <ActionIcon size="sm" variant={streamOnlyUnassigned ? 'filled' : 'subtle'} color="teal"
                  onClick={() => setStreamOnlyUnassigned(p => !p)}>
                  <IconFilter size={14} />
                </ActionIcon>
              </Tooltip>
              <Tooltip label="Hide stale">
                <ActionIcon size="sm" variant={streamHideStale ? 'filled' : 'subtle'} color="orange"
                  onClick={() => setStreamHideStale(p => !p)}>
                  <IconEyeOff size={14} />
                </ActionIcon>
              </Tooltip>
            </Group>
          </Box>
          <Divider />
          <ScrollArea style={{ flex: 1 }}>
            <Table striped highlightOnHover withRowBorders={false} fz="sm">
              <Table.Thead style={{ position: 'sticky', top: 0, zIndex: 1, background: 'var(--mantine-color-dark-7)' }}>
                <Table.Tr>
                  <Table.Th>Name</Table.Th>
                  <Table.Th>M3U</Table.Th>
                  <Table.Th style={{ width: 100 }} />
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {streamsLoading ? (
                  Array.from({ length: 6 }).map((_, i) => (
                    <Table.Tr key={i}>
                      {[1,2,3].map(j => <Table.Td key={j}><Skeleton h={16} /></Table.Td>)}
                    </Table.Tr>
                  ))
                ) : (
                  streams.map(st => (
                    <StreamRow
                      key={st.id}
                      stream={st}
                      selectedChannelIds={Array.from(selectedIds)}
                      onAddToChannel={(streamId, channelId) =>
                        assignStream.mutate({ streamId, channelId })}
                      onPreview={setPreviewTarget}
                      onDelete={id => {
                        if (confirm(`Delete stream "${st.name || st.url}"?`)) deleteStream.mutate(id)
                      }}
                    />
                  ))
                )}
                {!streamsLoading && streams.length === 0 && (
                  <Table.Tr>
                    <Table.Td colSpan={3}>
                      <Text c="dimmed" size="sm" ta="center" py="md">No streams.</Text>
                    </Table.Td>
                  </Table.Tr>
                )}
              </Table.Tbody>
            </Table>
          </ScrollArea>
          <Divider />
          <Box p="xs">
            <Text size="xs" c="dimmed">
              {streamsData?.total ?? 0} stream{(streamsData?.total ?? 0) !== 1 ? 's' : ''}
            </Text>
          </Box>
        </Paper>
      </Group>

      {/* ── Links footer ───────────────────────────────────────── */}
      <LinksFooter profileId={activeProfileId ?? undefined} />

      {/* ── Modals & drawers ───────────────────────────────────── */}
      <ChannelEditModal
        channel={editTarget}
        groups={groups}
        opened={editTarget !== null}
        onClose={() => setEditTarget(null)}
        onSave={saveChannel}
      />

      <BulkEditModal
        count={selectedIds.size}
        groups={groups}
        opened={bulkOpen}
        onClose={() => setBulkOpen(false)}
        onApply={async update =>
          bulkUpdate.mutateAsync({ ids: Array.from(selectedIds), update })
        }
      />

      <AutoMatchModal
        channelCount={selectedIds.size}
        opened={autoMatchOpen}
        onClose={() => setAutoMatchOpen(false)}
        onRun={async opts => {
          const result = await autoMatchApi.run({
            channel_ids: selectedIds.size > 0 ? Array.from(selectedIds) : undefined,
            ignore_prefixes: opts.ignore_prefixes,
            ignore_suffixes: opts.ignore_suffixes,
            ignore_strings: opts.ignore_strings,
          })
          invalidate()
          notifications.show({
            message: `Matched ${result.matched} of ${result.total} channels`,
            color: result.matched > 0 ? 'teal' : 'orange',
          })
          setAutoMatchOpen(false)
        }}
      />

      <PreviewDrawer
        target={previewTarget}
        opened={previewTarget !== null}
        onClose={() => setPreviewTarget(null)}
      />
    </Stack>
  )
}

// ──────────────────────────────────────────────────────────────────
// Channels page (tabbed)
// ──────────────────────────────────────────────────────────────────
export function Channels() {
  return (
    <Tabs defaultValue="channels" keepMounted={false} style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Tabs.List>
        <Tabs.Tab value="channels" leftSection={<IconDeviceTv size={14} />}>Channels</Tabs.Tab>
        <Tabs.Tab value="virtual" leftSection={<IconLayoutDashboard size={14} />}>Virtual</Tabs.Tab>
        <Tabs.Tab value="lineup" leftSection={<IconList size={14} />}>Lineup</Tabs.Tab>
      </Tabs.List>
      <Divider />
      <Box style={{ flex: 1, overflow: 'hidden', paddingTop: 12 }}>
        <Tabs.Panel value="channels" style={{ height: '100%' }}>
          <ChannelManagerTab />
        </Tabs.Panel>
        <Tabs.Panel value="virtual">
          <VirtualTab />
        </Tabs.Panel>
        <Tabs.Panel value="lineup">
          <LineupTab />
        </Tabs.Panel>
      </Box>
    </Tabs>
  )
}
