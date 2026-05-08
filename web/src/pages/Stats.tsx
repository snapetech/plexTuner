import React from 'react'
import {
  Stack, Group, Text, Paper, Table, Badge, ActionIcon, Tooltip,
  Button, Modal, TextInput, Select, MultiSelect, Switch, Divider,
  Tabs, Box, Alert, ScrollArea, Progress, Code, Collapse,
} from '@mantine/core'
import {
  IconPlayerStop, IconAlertCircle, IconPlus, IconTrash, IconEdit,
  IconRefresh, IconActivity, IconBolt, IconWifi, IconRoute, IconRobot,
  IconGhost,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import {
  statsApi, connectionsApi,
  type EventHook, type EventHookInput,
} from '../api/stats'
import { api } from '../api/client'

// ── Active Streams ────────────────────────────────────────────────────────
function ActiveStreamsTab() {
  const qc = useQueryClient()

  const { data, isLoading, isError, dataUpdatedAt } = useQuery({
    queryKey: ['active-streams'],
    queryFn: () => statsApi.activeStreams(),
    refetchInterval: 5000,
  })

  const stop = useMutation({
    mutationFn: (reqId: string) => statsApi.stopStream(reqId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['active-streams'] })
      notifications.show({ message: 'Stop requested', color: 'orange' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  function formatDuration(ms: number) {
    const s = Math.floor(ms / 1000)
    const m = Math.floor(s / 60)
    const h = Math.floor(m / 60)
    if (h > 0) return `${h}h ${m % 60}m`
    if (m > 0) return `${m}m ${s % 60}s`
    return `${s}s`
  }

  const streams = data?.active ?? []
  const inUse = data?.in_use ?? 0
  const limit = data?.tuner_limit ?? 0

  return (
    <>
      <Group justify="space-between" mb="md">
        <Group gap="md">
          <Text fw={500}>Active Streams</Text>
          {limit > 0 && (
            <Text size="sm" c="dimmed">{inUse} / {limit} tuner slots</Text>
          )}
        </Group>
        <Group gap="xs">
          {dataUpdatedAt > 0 && (
            <Text size="xs" c="dimmed">
              Updated {new Date(dataUpdatedAt).toLocaleTimeString()}
            </Text>
          )}
          <Tooltip label="Refresh now">
            <ActionIcon variant="subtle" size="sm"
              onClick={() => qc.invalidateQueries({ queryKey: ['active-streams'] })}>
              <IconRefresh size={14} />
            </ActionIcon>
          </Tooltip>
        </Group>
      </Group>

      {limit > 0 && (
        <Progress
          value={(inUse / limit) * 100}
          color={inUse >= limit ? 'red' : inUse >= limit * 0.8 ? 'orange' : 'teal'}
          mb="md"
          size="sm"
        />
      )}

      {isLoading ? (
        <Text size="sm" c="dimmed">Loading…</Text>
      ) : isError ? (
        <Alert icon={<IconAlertCircle size={16} />} color="red">
          Could not fetch active streams. Check that the tuner is running.
        </Alert>
      ) : streams.length === 0 ? (
        <Alert icon={<IconWifi size={16} />} color="gray">
          No active streams right now.
        </Alert>
      ) : (
        <ScrollArea>
          <Table striped highlightOnHover withRowBorders={false} fz="sm">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Channel</Table.Th>
                <Table.Th>Client</Table.Th>
                <Table.Th>Duration</Table.Th>
                <Table.Th>Started</Table.Th>
                <Table.Th style={{ width: 60 }} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {streams.map(st => (
                <Table.Tr key={st.request_id}>
                  <Table.Td>
                    <Stack gap={0}>
                      <Text size="sm">{st.guide_name || st.channel_id}</Text>
                      {st.guide_number && (
                        <Text size="xs" c="dimmed">Ch {st.guide_number}</Text>
                      )}
                    </Stack>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed" lineClamp={1} maw={200}>
                      {st.client_ua || '—'}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Badge color="teal" variant="outline" size="sm">
                      {formatDuration(st.duration_ms)}
                    </Badge>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {new Date(st.started_at).toLocaleTimeString()}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    {st.cancelable && !st.cancel_requested && (
                      <Tooltip label="Force stop">
                        <ActionIcon size="xs" variant="subtle" color="red"
                          onClick={() => {
                            if (confirm('Force stop this stream?')) stop.mutate(st.request_id)
                          }}>
                          <IconPlayerStop size={14} />
                        </ActionIcon>
                      </Tooltip>
                    )}
                    {st.cancel_requested && (
                      <Badge size="xs" color="orange">stopping</Badge>
                    )}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </ScrollArea>
      )}
    </>
  )
}

// ── System Events ─────────────────────────────────────────────────────────
const LEVEL_COLOR: Record<string, string> = {
  info: 'blue', warn: 'orange', error: 'red',
}

function SystemEventsTab() {
  const [levelFilter, setLevelFilter] = useState<string | null>(null)
  const [sourceFilter, setSourceFilter] = useState('')
  const [expandedId, setExpandedId] = useState<number | null>(null)

  const { data: events = [], isLoading } = useQuery({
    queryKey: ['system-events', levelFilter, sourceFilter],
    queryFn: () => statsApi.systemEvents({
      level: levelFilter ?? undefined,
      source: sourceFilter || undefined,
      limit: 200,
    }),
    refetchInterval: 10_000,
  })

  return (
    <>
      <Group mb="md" gap="sm">
        <Text fw={500}>System Events</Text>
        <Select
          size="xs"
          placeholder="All levels"
          data={[
            { value: 'info', label: 'Info' },
            { value: 'warn', label: 'Warning' },
            { value: 'error', label: 'Error' },
          ]}
          value={levelFilter}
          onChange={setLevelFilter}
          clearable
          style={{ width: 120 }}
        />
        <TextInput
          size="xs"
          placeholder="Filter by source"
          value={sourceFilter}
          onChange={e => setSourceFilter(e.currentTarget.value)}
          style={{ width: 160 }}
        />
      </Group>

      {isLoading ? (
        <Text size="sm" c="dimmed">Loading…</Text>
      ) : events.length === 0 ? (
        <Alert icon={<IconAlertCircle size={16} />} color="gray">
          No system events recorded yet.
        </Alert>
      ) : (
        <ScrollArea mah={600}>
          <Table withRowBorders={false} fz="xs">
            <Table.Thead style={{ position: 'sticky', top: 0, background: 'var(--mantine-color-dark-7)', zIndex: 1 }}>
              <Table.Tr>
                <Table.Th style={{ width: 60 }}>Level</Table.Th>
                <Table.Th style={{ width: 160 }}>Time</Table.Th>
                <Table.Th style={{ width: 120 }}>Source</Table.Th>
                <Table.Th>Message</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {events.map(ev => (
                <React.Fragment key={ev.id}>
                  <Table.Tr
                    style={{ cursor: ev.detail ? 'pointer' : undefined }}
                    onClick={() => ev.detail && setExpandedId(expandedId === ev.id ? null : ev.id)}
                  >
                    <Table.Td>
                      <Badge size="xs" color={LEVEL_COLOR[ev.level] ?? 'gray'}>{ev.level}</Badge>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">{new Date(ev.at).toLocaleString()}</Text>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">{ev.source || '—'}</Text>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs">{ev.message}</Text>
                    </Table.Td>
                  </Table.Tr>
                  {ev.detail && expandedId === ev.id && (
                    <Table.Tr>
                      <Table.Td colSpan={4}>
                        <Code block fz="xs" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                          {ev.detail}
                        </Code>
                      </Table.Td>
                    </Table.Tr>
                  )}
                </React.Fragment>
              ))}
            </Table.Tbody>
          </Table>
        </ScrollArea>
      )}
    </>
  )
}

// ── Connections (Event Hooks) ──────────────────────────────────────────────
const KNOWN_EVENTS = [
  'stream.start', 'stream.stop', 'stream.failover',
  'm3u.refresh', 'epg.refresh',
  'recording.start', 'recording.end',
  'channel.failover', 'system.error',
]

const SCRIPT_TEMPLATE = `#!/bin/sh
# Called with event JSON on stdin.
# Environment: TUNERR_EVENT_TYPE, TUNERR_EVENT_ID, TUNERR_EVENT_AT
read -r payload
echo "Event: $TUNERR_EVENT_TYPE" >&2
echo "$payload" | jq . >&2
`

function ConnectionModal({
  opened, onClose, initial,
}: {
  opened: boolean
  onClose: () => void
  initial: EventHook | null
}) {
  const qc = useQueryClient()
  const isEdit = !!initial

  const [name, setName] = useState(initial?.name ?? '')
  const [kind, setKind] = useState<'webhook' | 'script'>(initial?.kind ?? 'webhook')
  const [target, setTarget] = useState(initial?.target ?? '')
  const [eventTypes, setEventTypes] = useState<string[]>(initial?.event_types ?? [])
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)
  const [showTemplate, setShowTemplate] = useState(false)

  function reset() {
    setName(''); setKind('webhook'); setTarget('')
    setEventTypes([]); setEnabled(true); setShowTemplate(false)
  }

  const save = useMutation({
    mutationFn: () => {
      const data: EventHookInput = { name, kind, target, event_types: eventTypes, enabled }
      return isEdit
        ? connectionsApi.update(initial!.id, data)
        : connectionsApi.create(data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['connections'] })
      notifications.show({ message: isEdit ? 'Connection updated' : 'Connection created', color: 'teal' })
      reset(); onClose()
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Modal
      opened={opened}
      onClose={() => { reset(); onClose() }}
      title={isEdit ? `Edit — ${initial?.name}` : 'New Connection'}
      size="md"
    >
      <Stack gap="sm">
        <TextInput label="Name" value={name} onChange={e => setName(e.currentTarget.value)} required />
        <Select
          label="Kind"
          data={[
            { value: 'webhook', label: 'Webhook (HTTP POST)' },
            { value: 'script', label: 'Script (shell)' },
          ]}
          value={kind}
          onChange={v => setKind((v ?? 'webhook') as 'webhook' | 'script')}
        />
        <TextInput
          label={kind === 'script' ? 'Script path' : 'URL'}
          value={target}
          onChange={e => setTarget(e.currentTarget.value)}
          placeholder={kind === 'script' ? '/state/scripts/notify.sh' : 'https://hooks.example.com/…'}
          required
        />
        <MultiSelect
          label="Event types (empty = all)"
          data={KNOWN_EVENTS}
          value={eventTypes}
          onChange={setEventTypes}
          placeholder="All events"
          clearable
        />
        <Switch label="Enabled" checked={enabled} onChange={e => setEnabled(e.currentTarget.checked)} />

        {kind === 'script' && (
          <>
            <Button size="xs" variant="subtle" onClick={() => setShowTemplate(p => !p)}>
              {showTemplate ? 'Hide template' : 'Show starter script'}
            </Button>
            <Collapse in={showTemplate}>
              <Code block fz="xs" style={{ whiteSpace: 'pre' }}>{SCRIPT_TEMPLATE}</Code>
            </Collapse>
          </>
        )}

        <Group justify="flex-end" mt="sm">
          <Button variant="default" onClick={() => { reset(); onClose() }}>Cancel</Button>
          <Button color="teal" loading={save.isPending} onClick={() => save.mutate()}>
            {isEdit ? 'Save' : 'Create'}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

function ConnectionsTab() {
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<EventHook | null>(null)

  const { data: hooks = [], isLoading } = useQuery({
    queryKey: ['connections'],
    queryFn: () => connectionsApi.list(),
  })

  const del = useMutation({
    mutationFn: (id: number) => connectionsApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connections'] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <>
      <Group justify="space-between" mb="md">
        <Text fw={500}>Event Connections</Text>
        <Button size="xs" leftSection={<IconPlus size={14} />} color="teal"
          onClick={() => { setEditTarget(null); setModalOpen(true) }}>
          New Connection
        </Button>
      </Group>

      {isLoading ? (
        <Text size="sm" c="dimmed">Loading…</Text>
      ) : hooks.length === 0 ? (
        <Alert icon={<IconBolt size={16} />} color="gray">
          No connections yet. Wire up webhooks or scripts to react to stream and guide events.
        </Alert>
      ) : (
        <ScrollArea>
          <Table striped highlightOnHover withRowBorders={false} fz="sm">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Name</Table.Th>
                <Table.Th>Kind</Table.Th>
                <Table.Th>Target</Table.Th>
                <Table.Th>Events</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th style={{ width: 80 }} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {hooks.map(h => (
                <Table.Tr key={h.id}>
                  <Table.Td><Text size="sm">{h.name}</Text></Table.Td>
                  <Table.Td>
                    <Badge size="xs" color={h.kind === 'script' ? 'grape' : 'blue'} variant="outline">
                      {h.kind}
                    </Badge>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed" lineClamp={1} maw={220}>{h.target}</Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {h.event_types.length === 0 ? 'All' : h.event_types.join(', ')}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Badge size="xs" color={h.enabled ? 'teal' : 'gray'}>
                      {h.enabled ? 'Active' : 'Disabled'}
                    </Badge>
                  </Table.Td>
                  <Table.Td>
                    <Group gap={4} wrap="nowrap">
                      <Tooltip label="Edit">
                        <ActionIcon size="xs" variant="subtle" color="yellow"
                          onClick={() => { setEditTarget(h); setModalOpen(true) }}>
                          <IconEdit size={14} />
                        </ActionIcon>
                      </Tooltip>
                      <Tooltip label="Delete">
                        <ActionIcon size="xs" variant="subtle" color="red"
                          onClick={() => { if (confirm(`Delete "${h.name}"?`)) del.mutate(h.id) }}>
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

      <ConnectionModal
        opened={modalOpen}
        onClose={() => { setModalOpen(false); setEditTarget(null) }}
        initial={editTarget}
      />
    </>
  )
}

// ── Routing tab ───────────────────────────────────────────────────────────
function RoutingTab() {
  const qc = useQueryClient()

  const profile = useQuery({
    queryKey: ['provider-profile'],
    queryFn: () => api.get<Record<string, unknown>>('/api/provider/profile.json'),
    staleTime: 30_000,
  })
  const relays = useQuery({
    queryKey: ['shared-relays'],
    queryFn: () => api.get<Record<string, unknown>>('/api/debug/shared-relays.json'),
    staleTime: 30_000,
  })
  const attempts = useQuery({
    queryKey: ['stream-attempts'],
    queryFn: () => api.get<Record<string, unknown>>('/api/debug/stream-attempts.json?limit=20'),
    staleTime: 30_000,
  })
  const clearAttempts = useMutation({
    mutationFn: () => api.post('/api/ops/actions/stream-attempts-clear'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['stream-attempts'] })
      notifications.show({ message: 'Attempt history cleared', color: 'teal' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })
  const resetProfile = useMutation({
    mutationFn: () => api.post('/api/ops/actions/provider-profile-reset'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['provider-profile'] })
      notifications.show({ message: 'Provider penalties reset', color: 'teal' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })
  const p = profile.data as Record<string, unknown> | undefined
  const r = relays.data as Record<string, unknown> | undefined
  const atList = (attempts.data as Record<string, unknown> | undefined)

  return (
    <ScrollArea>
      <Stack gap="md">
        {/* Provider Profile */}
        <Paper withBorder p="md">
          <Group justify="space-between" mb="xs">
            <Text fw={600}>Provider Profile</Text>
            <Button size="xs" color="orange" variant="outline"
              onClick={() => { if (confirm('Reset provider penalties?')) resetProfile.mutate() }}
              loading={resetProfile.isPending}>
              Reset Penalties
            </Button>
          </Group>
          {profile.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : profile.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Provider profile unavailable.</Alert>
            : (
              <Table withRowBorders={false} fz="sm">
                <Table.Tbody>
                  <Table.Tr><Table.Td c="dimmed" w={220}>Effective tuner limit</Table.Td><Table.Td><Badge size="sm" color="teal">{String(p?.effective_tuner_limit ?? '—')}</Badge></Table.Td></Table.Tr>
                  <Table.Tr><Table.Td c="dimmed">Learned tuner limit</Table.Td><Table.Td>{String(p?.learned_tuner_limit ?? '—')}</Table.Td></Table.Tr>
                  <Table.Tr><Table.Td c="dimmed">Penalized hosts</Table.Td><Table.Td>{Array.isArray(p?.penalized_hosts) ? p!.penalized_hosts.length : '0'}</Table.Td></Table.Tr>
                  <Table.Tr><Table.Td c="dimmed">CF block hits</Table.Td><Table.Td>{String(p?.cf_block_hits ?? '0')}</Table.Td></Table.Tr>
                  <Table.Tr><Table.Td c="dimmed">Concurrency signals</Table.Td><Table.Td>{String(p?.concurrency_signals_seen ?? '0')}</Table.Td></Table.Tr>
                </Table.Tbody>
              </Table>
            )}
          {p && Array.isArray(p.remediation_hints) && p.remediation_hints.length > 0 && (
            <Box mt="xs">
              <Text size="xs" c="dimmed" mb={4}>Remediation hints:</Text>
              {(p.remediation_hints as string[]).map((h, i) => (
                <Alert key={i} color="yellow" p="xs" mb={4}><Text size="xs">{h}</Text></Alert>
              ))}
            </Box>
          )}
        </Paper>

        {/* Shared Relays */}
        <Paper withBorder p="md">
          <Text fw={600} mb="xs">Shared Relays</Text>
          {relays.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : relays.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Relay info unavailable.</Alert>
            : (
              <Group gap="xl">
                <Box>
                  <Text size="xs" c="dimmed">Active relays</Text>
                  <Text fw={500}>{String(r?.relay_count ?? r?.count ?? '—')}</Text>
                </Box>
                <Box>
                  <Text size="xs" c="dimmed">Total subscribers</Text>
                  <Text fw={500}>{String(r?.subscriber_total ?? r?.subscribers ?? '—')}</Text>
                </Box>
              </Group>
            )}
        </Paper>

        {/* Stream Attempts */}
        <Paper withBorder p="md">
          <Group justify="space-between" mb="xs">
            <Text fw={600}>Recent Stream Attempts</Text>
            <Button size="xs" color="red" variant="outline"
              onClick={() => { if (confirm('Clear attempt history?')) clearAttempts.mutate() }}
              loading={clearAttempts.isPending}>
              Clear History
            </Button>
          </Group>
          {attempts.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : attempts.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Attempt log unavailable.</Alert>
            : (() => {
                const list = Array.isArray(atList) ? atList as Record<string,unknown>[] : Array.isArray((atList as Record<string,unknown>|null)?.attempts) ? (atList as Record<string,unknown>)!.attempts as Record<string,unknown>[] : []
                if (list.length === 0) return <Text size="sm" c="dimmed">No recent attempts.</Text>
                return (
                  <Table withRowBorders={false} fz="xs" striped>
                    <Table.Thead>
                      <Table.Tr>
                        <Table.Th>Channel</Table.Th>
                        <Table.Th>Outcome</Table.Th>
                        <Table.Th>When</Table.Th>
                      </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                      {list.slice(0, 20).map((a, i) => (
                        <Table.Tr key={i}>
                          <Table.Td>{String(a.channel_name ?? a.channel_id ?? '—')}</Table.Td>
                          <Table.Td>
                            <Badge size="xs" color={String(a.outcome ?? a.result ?? 'ok') === 'ok' ? 'teal' : 'red'}>
                              {String(a.outcome ?? a.result ?? '—')}
                            </Badge>
                          </Table.Td>
                          <Table.Td c="dimmed">{a.at ? new Date(String(a.at)).toLocaleTimeString() : '—'}</Table.Td>
                        </Table.Tr>
                      ))}
                    </Table.Tbody>
                  </Table>
                )
              })()}
        </Paper>

      </Stack>
    </ScrollArea>
  )
}

// ── Autopilot tab ─────────────────────────────────────────────────────────
function AutopilotTab() {
  const qc = useQueryClient()

  const report = useQuery({
    queryKey: ['autopilot-report'],
    queryFn: () => api.get<Record<string, unknown>>('/api/autopilot/report.json?limit=8'),
    staleTime: 30_000,
  })

  const reset = useMutation({
    mutationFn: () => api.post('/api/ops/actions/autopilot-reset'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['autopilot-report'] })
      notifications.show({ message: 'Autopilot memory reset', color: 'teal' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const d = report.data
  const hot = Array.isArray(d?.hot_channels) ? d!.hot_channels as Record<string,unknown>[] : []

  return (
    <Stack gap="md">
      <Paper withBorder p="md">
        <Group justify="space-between" mb="xs">
          <Text fw={600}>Autopilot Report</Text>
          <Button size="xs" color="orange" variant="outline"
            onClick={() => { if (confirm('Reset autopilot memory? This will clear learned channel routing.')) reset.mutate() }}
            loading={reset.isPending}>
            Reset Autopilot Memory
          </Button>
        </Group>
        {report.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
          : report.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Autopilot report unavailable.</Alert>
          : (
            <Stack gap="sm">
              <Group gap="xl">
                <Box>
                  <Text size="xs" c="dimmed">Decisions made</Text>
                  <Text fw={500}>{String(d?.decision_count ?? '—')}</Text>
                </Box>
                {!!d?.consensus_host && (
                  <Box>
                    <Text size="xs" c="dimmed">Consensus host</Text>
                    <Code>{String(d.consensus_host)}</Code>
                  </Box>
                )}
                {d?.consensus_dna_count !== undefined && (
                  <Box>
                    <Text size="xs" c="dimmed">DNA samples</Text>
                    <Text fw={500}>{String(d.consensus_dna_count)}</Text>
                  </Box>
                )}
              </Group>
              {hot.length > 0 && (
                <>
                  <Divider label="Hot Channels" />
                  <Table withRowBorders={false} fz="sm" striped>
                    <Table.Thead>
                      <Table.Tr>
                        <Table.Th>Channel</Table.Th>
                        <Table.Th>Score</Table.Th>
                      </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                      {hot.map((ch, i) => (
                        <Table.Tr key={i}>
                          <Table.Td>{String(ch.name ?? ch.channel_name ?? '—')}</Table.Td>
                          <Table.Td>
                            <Badge size="sm" color="blue" variant="outline">{String(ch.score ?? '—')}</Badge>
                          </Table.Td>
                        </Table.Tr>
                      ))}
                    </Table.Tbody>
                  </Table>
                </>
              )}
              {hot.length === 0 && <Text size="sm" c="dimmed">No hot channel data yet.</Text>}
            </Stack>
          )}
      </Paper>
    </Stack>
  )
}

// ── Plex tab ──────────────────────────────────────────────────────────────
function PlexTab() {
  const ghostReport = useQuery({
    queryKey: ['plex-ghost-report'],
    queryFn: () => api.get<Record<string, unknown>>('/api/plex/ghost-report.json?observe=0s'),
    staleTime: 30_000,
  })

  const stopVisible = useMutation({
    mutationFn: () => api.post('/api/ops/actions/ghost-visible-stop'),
    onSuccess: () => notifications.show({ message: 'Stop visible ghosts requested', color: 'teal' }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })
  const dryRun = useMutation({
    mutationFn: () => api.post('/api/ops/actions/ghost-hidden-recover?mode=dry-run'),
    onSuccess: () => notifications.show({ message: 'Dry-run recovery triggered', color: 'teal' }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })
  const restart = useMutation({
    mutationFn: () => api.post('/api/ops/actions/ghost-hidden-recover?mode=restart'),
    onSuccess: () => notifications.show({ message: 'Hidden grab restart requested', color: 'teal' }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const d = ghostReport.data
  const visible = Array.isArray(d?.visible_ghosts) ? d!.visible_ghosts as Record<string,unknown>[] : []

  return (
    <Stack gap="md">
      <Paper withBorder p="md">
        <Group justify="space-between" mb="xs">
          <Text fw={600}>Plex Ghost Hunter</Text>
          <Group gap="xs">
            <Button size="xs" color="red" variant="outline"
              onClick={() => { if (confirm('Stop all visible ghost sessions?')) stopVisible.mutate() }}
              loading={stopVisible.isPending}>
              Stop Visible Ghosts
            </Button>
            <Button size="xs" variant="outline"
              onClick={() => { if (confirm('Run dry-run hidden recovery?')) dryRun.mutate() }}
              loading={dryRun.isPending}>
              Dry-Run Hidden Recovery
            </Button>
            <Button size="xs" color="orange" variant="outline"
              onClick={() => { if (confirm('Restart all hidden grabs? This will interrupt them.')) restart.mutate() }}
              loading={restart.isPending}>
              Restart Hidden Grabs
            </Button>
          </Group>
        </Group>
        {ghostReport.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
          : ghostReport.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Ghost report unavailable. Plex integration may not be configured.</Alert>
          : (
            <Stack gap="sm">
              <Group gap="xl">
                <Box>
                  <Text size="xs" c="dimmed">Visible ghosts</Text>
                  <Text fw={500} c={visible.length > 0 ? 'red' : 'teal'}>{visible.length}</Text>
                </Box>
                <Box>
                  <Text size="xs" c="dimmed">Hidden grabs</Text>
                  <Text fw={500}>{String(d?.hidden_grabs ?? '0')}</Text>
                </Box>
              </Group>
              {visible.length > 0 && (
                <>
                  <Divider label="Visible Ghost Sessions" />
                  <Table withRowBorders={false} fz="sm" striped>
                    <Table.Thead>
                      <Table.Tr>
                        <Table.Th>Session</Table.Th>
                        <Table.Th>When</Table.Th>
                      </Table.Tr>
                    </Table.Thead>
                    <Table.Tbody>
                      {visible.map((g, i) => (
                        <Table.Tr key={i}>
                          <Table.Td>{String(g.session_name ?? g.session_id ?? g.name ?? '—')}</Table.Td>
                          <Table.Td c="dimmed">{g.at ? new Date(String(g.at)).toLocaleTimeString() : '—'}</Table.Td>
                        </Table.Tr>
                      ))}
                    </Table.Tbody>
                  </Table>
                </>
              )}
              {visible.length === 0 && <Text size="sm" c="dimmed">No visible ghost sessions.</Text>}
            </Stack>
          )}
      </Paper>
    </Stack>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────
export function Stats() {
  return (
    <Stack gap="md" h="100%" style={{ overflow: 'hidden' }}>
      <Group justify="space-between">
        <Text size="lg" fw={600}>Stats</Text>
      </Group>

      <Paper withBorder p="md" style={{ flex: 1, overflow: 'hidden' }}>
        <Tabs defaultValue="streams" keepMounted={false}>
          <Tabs.List>
            <Tabs.Tab value="streams" leftSection={<IconActivity size={14} />}>
              Active Streams
            </Tabs.Tab>
            <Tabs.Tab value="events" leftSection={<IconAlertCircle size={14} />}>
              System Events
            </Tabs.Tab>
            <Tabs.Tab value="connections" leftSection={<IconBolt size={14} />}>
              Connections
            </Tabs.Tab>
            <Tabs.Tab value="routing" leftSection={<IconRoute size={14} />}>
              Routing
            </Tabs.Tab>
            <Tabs.Tab value="autopilot" leftSection={<IconRobot size={14} />}>
              Autopilot
            </Tabs.Tab>
            <Tabs.Tab value="plex" leftSection={<IconGhost size={14} />}>
              Plex
            </Tabs.Tab>
          </Tabs.List>
          <Divider />
          <Box pt="md">
            <Tabs.Panel value="streams"><ActiveStreamsTab /></Tabs.Panel>
            <Tabs.Panel value="events"><SystemEventsTab /></Tabs.Panel>
            <Tabs.Panel value="connections"><ConnectionsTab /></Tabs.Panel>
            <Tabs.Panel value="routing"><RoutingTab /></Tabs.Panel>
            <Tabs.Panel value="autopilot"><AutopilotTab /></Tabs.Panel>
            <Tabs.Panel value="plex"><PlexTab /></Tabs.Panel>
          </Box>
        </Tabs>
      </Paper>
    </Stack>
  )
}
