import {
  Stack, Group, Text, Paper, Table, Badge, ActionIcon, Tooltip,
  Button, Modal, TextInput, Select, Checkbox, Divider,
  Tabs, Box, Alert, ScrollArea,
} from '@mantine/core'
import {
  IconPlus, IconTrash, IconEdit, IconAlertCircle, IconPlayerRecord,
  IconClock, IconDatabase,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, useEffect } from 'react'
import { useLocation } from 'react-router-dom'

import {
  recordingsApi,
  type RecordingInput,
  type RecordingRule, type RecordingRuleInput,
} from '../api/recordings'
import { channelsApi } from '../api/channels'
import { api } from '../api/client'

const DAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

const STATUS_COLOR: Record<string, string> = {
  scheduled: 'blue',
  recording: 'red',
  done: 'teal',
  failed: 'red',
}

// ── New Recording modal ───────────────────────────────────────────────────
function NewRecordingModal({
  opened, onClose, prefill,
}: {
  opened: boolean
  onClose: () => void
  prefill?: { title?: string; start_at?: string; end_at?: string }
}) {
  const qc = useQueryClient()
  const { data: channels = [] } = useQuery({
    queryKey: ['channels'],
    queryFn: () => channelsApi.list({}),
    select: r => r.channels,
  })

  const [channelId, setChannelId] = useState<string | null>(null)
  const [title, setTitle] = useState(prefill?.title ?? '')
  const [startAt, setStartAt] = useState(prefill?.start_at ?? '')
  const [endAt, setEndAt] = useState(prefill?.end_at ?? '')
  const [recurring, setRecurring] = useState(false)

  useEffect(() => {
    if (opened && prefill) {
      setTitle(prefill.title ?? '')
      setStartAt(prefill.start_at ?? '')
      setEndAt(prefill.end_at ?? '')
    }
  }, [opened, prefill])

  function reset() {
    setChannelId(null); setTitle(''); setStartAt(''); setEndAt(''); setRecurring(false)
  }

  const save = useMutation({
    mutationFn: () => {
      const data: RecordingInput = {
        title,
        start_at: startAt,
        end_at: endAt,
        recurring,
        ...(channelId ? { channel_id: Number(channelId) } : {}),
      }
      return recordingsApi.create(data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['recordings'] })
      notifications.show({ message: 'Recording scheduled', color: 'teal' })
      reset(); onClose()
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Modal opened={opened} onClose={() => { reset(); onClose() }} title="New Recording" size="md">
      <Stack gap="sm">
        <Select
          label="Channel (optional)"
          placeholder="Any channel"
          data={channels.map(c => ({ value: String(c.id), label: c.name }))}
          value={channelId}
          onChange={setChannelId}
          clearable
          searchable
        />
        <TextInput label="Title" value={title} onChange={e => setTitle(e.currentTarget.value)} required />
        <TextInput
          label="Start (ISO datetime)"
          value={startAt}
          onChange={e => setStartAt(e.currentTarget.value)}
          placeholder="2026-05-07T20:00:00Z"
          required
        />
        <TextInput
          label="End (ISO datetime)"
          value={endAt}
          onChange={e => setEndAt(e.currentTarget.value)}
          placeholder="2026-05-07T21:00:00Z"
          required
        />
        <Checkbox
          label="Recurring"
          checked={recurring}
          onChange={e => setRecurring(e.currentTarget.checked)}
        />
        <Group justify="flex-end" mt="sm">
          <Button variant="default" onClick={() => { reset(); onClose() }}>Cancel</Button>
          <Button color="red" leftSection={<IconPlayerRecord size={14} />}
            loading={save.isPending} onClick={() => save.mutate()}>
            Schedule
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

// ── New Rule modal ────────────────────────────────────────────────────────
function RuleModal({
  opened, onClose, initial,
}: {
  opened: boolean
  onClose: () => void
  initial: RecordingRule | null
}) {
  const qc = useQueryClient()
  const { data: channels = [] } = useQuery({
    queryKey: ['channels'],
    queryFn: () => channelsApi.list({}),
    select: r => r.channels,
  })

  const [channelId, setChannelId] = useState<string | null>(initial?.channel_id ? String(initial.channel_id) : null)
  const [title, setTitle] = useState(initial?.title ?? '')
  const [days, setDays] = useState<number[]>(initial?.days ?? [])
  const [startTime, setStartTime] = useState(initial?.start_time ?? '')
  const [endTime, setEndTime] = useState(initial?.end_time ?? '')
  const [isActive, setIsActive] = useState(initial?.is_active ?? true)

  function toggleDay(d: number) {
    setDays(prev => prev.includes(d) ? prev.filter(x => x !== d) : [...prev, d])
  }

  function reset() {
    setChannelId(null); setTitle(''); setDays([]); setStartTime(''); setEndTime(''); setIsActive(true)
  }

  const save = useMutation({
    mutationFn: () => {
      const data: RecordingRuleInput = {
        title, days, start_time: startTime, end_time: endTime, is_active: isActive,
        ...(channelId ? { channel_id: Number(channelId) } : {}),
      }
      return initial
        ? recordingsApi.updateRule(initial.id, data)
        : recordingsApi.createRule(data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['recording-rules'] })
      notifications.show({ message: initial ? 'Rule updated' : 'Rule created', color: 'teal' })
      reset(); onClose()
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Modal
      opened={opened}
      onClose={() => { reset(); onClose() }}
      title={initial ? 'Edit Rule' : 'New Recording Rule'}
      size="md"
    >
      <Stack gap="sm">
        <Select
          label="Channel (optional)"
          placeholder="Any channel"
          data={channels.map(c => ({ value: String(c.id), label: c.name }))}
          value={channelId}
          onChange={setChannelId}
          clearable
          searchable
        />
        <TextInput label="Title / show name" value={title}
          onChange={e => setTitle(e.currentTarget.value)} required />
        <Box>
          <Text size="sm" fw={500} mb={6}>Days</Text>
          <Group gap="xs">
            {DAY_LABELS.map((label, i) => (
              <Button
                key={i}
                size="xs"
                variant={days.includes(i) ? 'filled' : 'default'}
                color="teal"
                onClick={() => toggleDay(i)}
              >
                {label}
              </Button>
            ))}
          </Group>
        </Box>
        <Group grow>
          <TextInput label="Start time" value={startTime}
            onChange={e => setStartTime(e.currentTarget.value)} placeholder="20:00" />
          <TextInput label="End time" value={endTime}
            onChange={e => setEndTime(e.currentTarget.value)} placeholder="21:00" />
        </Group>
        <Checkbox label="Active" checked={isActive}
          onChange={e => setIsActive(e.currentTarget.checked)} />
        <Group justify="flex-end" mt="sm">
          <Button variant="default" onClick={() => { reset(); onClose() }}>Cancel</Button>
          <Button color="teal" loading={save.isPending} onClick={() => save.mutate()}>
            {initial ? 'Save' : 'Create Rule'}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

// ── Recordings tab ────────────────────────────────────────────────────────
function RecordingsTab({
  externalOpen, externalPrefill, onExternalClose,
}: {
  externalOpen?: boolean
  externalPrefill?: { title?: string; start_at?: string; end_at?: string }
  onExternalClose?: () => void
}) {
  const qc = useQueryClient()
  const [newOpen, setNewOpen] = useState(false)
  const [statusFilter, setStatusFilter] = useState<string | null>(null)

  const isOpen = externalOpen || newOpen
  const activePrefill = externalOpen ? externalPrefill : undefined

  const { data: recordings = [], isLoading } = useQuery({
    queryKey: ['recordings', statusFilter],
    queryFn: () => recordingsApi.list(statusFilter ?? undefined),
  })

  const del = useMutation({
    mutationFn: (id: number) => recordingsApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['recordings'] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <>
      <Group justify="space-between" mb="sm">
        <Group gap="xs">
          <Text fw={500}>Recordings</Text>
          <Select
            size="xs"
            placeholder="All statuses"
            data={[
              { value: 'scheduled', label: 'Scheduled' },
              { value: 'recording', label: 'Recording' },
              { value: 'done', label: 'Done' },
              { value: 'failed', label: 'Failed' },
            ]}
            value={statusFilter}
            onChange={setStatusFilter}
            clearable
            style={{ width: 140 }}
          />
        </Group>
        <Button size="xs" leftSection={<IconPlus size={14} />} color="red"
          onClick={() => { setNewOpen(true) }}>
          New Recording
        </Button>
      </Group>

      {isLoading ? (
        <Text size="sm" c="dimmed">Loading…</Text>
      ) : recordings.length === 0 ? (
        <Alert icon={<IconAlertCircle size={16} />} color="gray">
          No recordings yet.
        </Alert>
      ) : (
        <ScrollArea>
          <Table striped highlightOnHover withRowBorders={false} fz="sm">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Title</Table.Th>
                <Table.Th>Channel</Table.Th>
                <Table.Th>Start</Table.Th>
                <Table.Th>End</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th style={{ width: 60 }} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {recordings.map(rec => (
                <Table.Tr key={rec.id}>
                  <Table.Td><Text size="sm">{rec.title}</Text></Table.Td>
                  <Table.Td><Text size="xs" c="dimmed">{rec.channel_name || '—'}</Text></Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {new Date(rec.start_at).toLocaleString()}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {new Date(rec.end_at).toLocaleString()}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Badge size="xs" color={STATUS_COLOR[rec.status] ?? 'gray'}>
                      {rec.status}
                    </Badge>
                  </Table.Td>
                  <Table.Td>
                    <Tooltip label="Delete">
                      <ActionIcon size="xs" variant="subtle" color="red"
                        onClick={() => {
                          if (confirm(`Delete recording "${rec.title}"?`)) del.mutate(rec.id)
                        }}>
                        <IconTrash size={14} />
                      </ActionIcon>
                    </Tooltip>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </ScrollArea>
      )}

      <NewRecordingModal
        opened={isOpen}
        prefill={activePrefill}
        onClose={() => { setNewOpen(false); onExternalClose?.() }}
      />
    </>
  )
}

// ── Rules tab ─────────────────────────────────────────────────────────────
function RulesTab() {
  const qc = useQueryClient()
  const [ruleModal, setRuleModal] = useState(false)
  const [editTarget, setEditTarget] = useState<RecordingRule | null>(null)

  const { data: rules = [], isLoading } = useQuery({
    queryKey: ['recording-rules'],
    queryFn: () => recordingsApi.listRules(),
  })

  const del = useMutation({
    mutationFn: (id: number) => recordingsApi.deleteRule(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['recording-rules'] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <>
      <Group justify="space-between" mb="sm">
        <Text fw={500}>Recording Rules</Text>
        <Button size="xs" leftSection={<IconPlus size={14} />} color="teal"
          onClick={() => { setEditTarget(null); setRuleModal(true) }}>
          New Rule
        </Button>
      </Group>

      {isLoading ? (
        <Text size="sm" c="dimmed">Loading…</Text>
      ) : rules.length === 0 ? (
        <Alert icon={<IconAlertCircle size={16} />} color="gray">
          No recurring rules yet. Create rules for shows that air on a schedule.
        </Alert>
      ) : (
        <ScrollArea>
          <Table striped highlightOnHover withRowBorders={false} fz="sm">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Title</Table.Th>
                <Table.Th>Channel</Table.Th>
                <Table.Th>Days</Table.Th>
                <Table.Th>Time</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th style={{ width: 80 }} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {rules.map(rr => (
                <Table.Tr key={rr.id}>
                  <Table.Td><Text size="sm">{rr.title}</Text></Table.Td>
                  <Table.Td><Text size="xs" c="dimmed">{rr.channel_name || '—'}</Text></Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {rr.days.length === 7 ? 'Every day' : rr.days.map(d => DAY_LABELS[d]).join(', ') || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">{rr.start_time} – {rr.end_time}</Text>
                  </Table.Td>
                  <Table.Td>
                    <Badge size="xs" color={rr.is_active ? 'teal' : 'gray'}>
                      {rr.is_active ? 'Active' : 'Paused'}
                    </Badge>
                  </Table.Td>
                  <Table.Td>
                    <Group gap={4} wrap="nowrap">
                      <Tooltip label="Edit">
                        <ActionIcon size="xs" variant="subtle" color="yellow"
                          onClick={() => { setEditTarget(rr); setRuleModal(true) }}>
                          <IconEdit size={14} />
                        </ActionIcon>
                      </Tooltip>
                      <Tooltip label="Delete">
                        <ActionIcon size="xs" variant="subtle" color="red"
                          onClick={() => {
                            if (confirm(`Delete rule "${rr.title}"?`)) del.mutate(rr.id)
                          }}>
                          <IconTrash size={14} />
                        </ActionIcon>
                      </Tooltip>
                    </Group>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </ScrollArea>
      )}

      <RuleModal
        opened={ruleModal}
        onClose={() => { setRuleModal(false); setEditTarget(null) }}
        initial={editTarget}
      />
    </>
  )
}

// ── Recorder tab ─────────────────────────────────────────────────────────
function RecorderTab() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['recorder-report'],
    queryFn: () => api.get<Record<string, unknown>>('/api/recordings/recorder.json?limit=20'),
    staleTime: 30_000,
    refetchInterval: 30_000,
  })

  const d = data as Record<string, unknown> | undefined
  const status = (d?.status ?? d?.recorder ?? d) as Record<string, unknown> | undefined
  const recentList = Array.isArray(d?.recordings) ? d!.recordings as Record<string,unknown>[] :
    Array.isArray(d?.recent) ? d!.recent as Record<string,unknown>[] : []

  return (
    <Stack gap="md">
      <Paper withBorder p="md">
        <Text fw={600} mb="sm">Recorder Status</Text>
        {isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
          : isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Recorder unavailable.</Alert>
          : (
            <Table withRowBorders={false} fz="sm">
              <Table.Tbody>
                <Table.Tr>
                  <Table.Td c="dimmed" w={200}>In flight</Table.Td>
                  <Table.Td>
                    <Badge size="sm" color={Number(status?.in_flight ?? 0) > 0 ? 'red' : 'gray'}>
                      {String(status?.in_flight ?? '0')}
                    </Badge>
                  </Table.Td>
                </Table.Tr>
                {!!status?.last_error && (
                  <Table.Tr>
                    <Table.Td c="dimmed">Last error</Table.Td>
                    <Table.Td c="red">{String(status.last_error)}</Table.Td>
                  </Table.Tr>
                )}
                {status?.last_duration !== undefined && (
                  <Table.Tr>
                    <Table.Td c="dimmed">Last duration</Table.Td>
                    <Table.Td>{String(status.last_duration)}</Table.Td>
                  </Table.Tr>
                )}
                {status?.throughput !== undefined && (
                  <Table.Tr>
                    <Table.Td c="dimmed">Throughput</Table.Td>
                    <Table.Td>{String(status.throughput)}</Table.Td>
                  </Table.Tr>
                )}
              </Table.Tbody>
            </Table>
          )}
      </Paper>

      <Paper withBorder p="md">
        <Text fw={600} mb="sm">Recent Recordings</Text>
        {isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
          : isError ? null
          : recentList.length === 0 ? <Text size="sm" c="dimmed">No recent recordings.</Text>
          : (
            <ScrollArea>
              <Table striped withRowBorders={false} fz="sm">
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>Title</Table.Th>
                    <Table.Th>Channel</Table.Th>
                    <Table.Th>Start</Table.Th>
                    <Table.Th>Status</Table.Th>
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {recentList.map((rec, i) => (
                    <Table.Tr key={i}>
                      <Table.Td>{String(rec.title ?? '—')}</Table.Td>
                      <Table.Td c="dimmed">{String(rec.channel_name ?? rec.channel ?? '—')}</Table.Td>
                      <Table.Td c="dimmed">{rec.start_at ? new Date(String(rec.start_at)).toLocaleString() : '—'}</Table.Td>
                      <Table.Td>
                        <Badge size="xs" color={String(rec.status ?? 'done') === 'recording' ? 'red' : 'teal'}>
                          {String(rec.status ?? '—')}
                        </Badge>
                      </Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            </ScrollArea>
          )}
      </Paper>
    </Stack>
  )
}

interface RecordPrefill {
  title?: string
  start_at?: string
  end_at?: string
}

// ── Page ──────────────────────────────────────────────────────────────────
export function Dvr() {
  const location = useLocation()
  const [newOpen, setNewOpen] = useState(false)
  const [prefill, setPrefill] = useState<RecordPrefill | undefined>(undefined)

  useEffect(() => {
    const state = location.state as { prefill?: RecordPrefill } | null
    if (state?.prefill) {
      setPrefill(state.prefill)
      setNewOpen(true)
      window.history.replaceState({}, '')
    }
  }, [location.state])

  return (
    <Stack gap="md" h="100%" style={{ overflow: 'hidden' }}>
      <Group justify="space-between">
        <Text size="lg" fw={600}>DVR</Text>
      </Group>

      <Paper withBorder p="md" style={{ flex: 1, overflow: 'hidden' }}>
        <Tabs defaultValue="recordings" keepMounted={false}>
          <Tabs.List>
            <Tabs.Tab value="recordings" leftSection={<IconPlayerRecord size={14} />}>
              Recordings
            </Tabs.Tab>
            <Tabs.Tab value="rules" leftSection={<IconClock size={14} />}>
              Rules
            </Tabs.Tab>
            <Tabs.Tab value="recorder" leftSection={<IconDatabase size={14} />}>
              Recorder
            </Tabs.Tab>
          </Tabs.List>
          <Divider />
          <Box pt="md">
            <Tabs.Panel value="recordings"><RecordingsTab externalOpen={newOpen} externalPrefill={prefill} onExternalClose={() => { setNewOpen(false); setPrefill(undefined) }} /></Tabs.Panel>
            <Tabs.Panel value="rules"><RulesTab /></Tabs.Panel>
            <Tabs.Panel value="recorder"><RecorderTab /></Tabs.Panel>
          </Box>
        </Tabs>
      </Paper>
    </Stack>
  )
}
