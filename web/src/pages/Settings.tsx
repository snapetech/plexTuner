import {
  Stack, Group, Text, Paper, Tabs, TextInput, NumberInput, Select,
  Button, Switch, Textarea, Table, ActionIcon, Tooltip, Badge,
  Modal, Divider, Alert, ScrollArea, Code, Anchor, MultiSelect, Box,
} from '@mantine/core'
import {
  IconPlus, IconTrash, IconEdit, IconAlertCircle, IconDeviceFloppy,
  IconPlugConnected, IconServer, IconShield, IconExternalLink,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, useEffect } from 'react'

import { settingsApi, streamProfilesApi, type StreamProfile, type StreamProfileInput } from '../api/settings'
import { api, boot } from '../api/client'

const PROFILE_TYPES = [
  { value: 'ffmpeg', label: 'FFmpeg (transcode)' },
  { value: 'proxy', label: 'Proxy (passthrough)' },
  { value: 'redirect', label: 'Redirect (HTTP 302)' },
  { value: 'streamlink', label: 'Streamlink' },
  { value: 'vlc', label: 'VLC' },
  { value: 'yt-dlp', label: 'yt-dlp' },
  { value: 'custom', label: 'Custom' },
]

const DEFAULT_FFMPEG_CONFIG = JSON.stringify({
  video_codec: 'copy',
  audio_codec: 'copy',
  extra_args: [],
}, null, 2)

const UA_PRESETS = ['', 'Lavf/58.76.100', 'VLC/3.0.18 LibVLC/3.0.18', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)']

// ── Stream Profile Modal ───────────────────────────────────────────────────
function ProfileModal({
  opened, onClose, initial,
}: {
  opened: boolean
  onClose: () => void
  initial: StreamProfile | null
}) {
  const qc = useQueryClient()
  const isEdit = !!initial

  const [name, setName] = useState(initial?.name ?? '')
  const [type, setType] = useState(initial?.type ?? 'proxy')
  const [configJSON, setConfigJSON] = useState(initial?.config_json ?? '')
  const [isDefault, setIsDefault] = useState(initial?.is_default ?? false)

  useEffect(() => {
    if (opened) {
      setName(initial?.name ?? '')
      setType(initial?.type ?? 'proxy')
      setConfigJSON(initial?.config_json ?? '')
      setIsDefault(initial?.is_default ?? false)
    }
  }, [opened, initial])

  const save = useMutation({
    mutationFn: () => {
      const data: StreamProfileInput = { name, type, config_json: configJSON || undefined, is_default: isDefault }
      return isEdit ? streamProfilesApi.update(initial!.id, data) : streamProfilesApi.create(data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['stream-profiles'] })
      notifications.show({ message: isEdit ? 'Profile updated' : 'Profile created', color: 'teal' })
      onClose()
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Modal
      opened={opened}
      onClose={onClose}
      title={isEdit ? `Edit — ${initial?.name}` : 'New Stream Profile'}
      size="md"
    >
      <Stack gap="sm">
        <TextInput label="Name" value={name} onChange={e => setName(e.currentTarget.value)} required />
        <Select label="Type" data={PROFILE_TYPES} value={type} onChange={v => {
          const t = v ?? 'proxy'
          setType(t)
          if (t === 'ffmpeg' && !configJSON) setConfigJSON(DEFAULT_FFMPEG_CONFIG)
        }} />
        <Textarea
          label="Config JSON"
          value={configJSON}
          onChange={e => setConfigJSON(e.currentTarget.value)}
          placeholder='{"key": "value"}'
          autosize
          minRows={4}
          maxRows={12}
          styles={{ input: { fontFamily: 'monospace', fontSize: 12 } }}
        />
        <Switch
          label="Set as default profile"
          checked={isDefault}
          onChange={e => setIsDefault(e.currentTarget.checked)}
        />
      </Stack>
      <Divider my="sm" />
      <Group justify="flex-end">
        <Button variant="default" onClick={onClose}>Cancel</Button>
        <Button color="teal" loading={save.isPending} onClick={() => save.mutate()}>
          {isEdit ? 'Save' : 'Create'}
        </Button>
      </Group>
    </Modal>
  )
}

// ── General settings tab ───────────────────────────────────────────────────
function GeneralTab() {
  const qc = useQueryClient()
  const { data: settings } = useQuery({
    queryKey: ['settings'],
    queryFn: () => settingsApi.get(),
  })

  const [tunerName, setTunerName] = useState('iptvTunerr')
  const [tunerCount, setTunerCount] = useState<number>(1)
  const [dvrPath, setDvrPath] = useState('{state_dir}/recordings/{title}/{title} - {date}.ts')
  const [dvrPadBefore, setDvrPadBefore] = useState<number>(0)
  const [dvrPadAfter, setDvrPadAfter] = useState<number>(30)

  useEffect(() => {
    if (!settings) return
    setTunerName(settings['tuner.device_name'] ?? 'iptvTunerr')
    setTunerCount(Number(settings['tuner.device_count'] ?? 1))
    setDvrPath(settings['dvr.path_template'] ?? '{state_dir}/recordings/{title}/{title} - {date}.ts')
    setDvrPadBefore(Number(settings['dvr.pad_before_sec'] ?? 0))
    setDvrPadAfter(Number(settings['dvr.pad_after_sec'] ?? 30))
  }, [settings])

  const save = useMutation({
    mutationFn: () => settingsApi.patch({
      'tuner.device_name': tunerName,
      'tuner.device_count': String(tunerCount),
      'dvr.path_template': dvrPath,
      'dvr.pad_before_sec': String(dvrPadBefore),
      'dvr.pad_after_sec': String(dvrPadAfter),
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['settings'] })
      notifications.show({ message: 'Settings saved', color: 'teal' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const { version, port } = boot()

  return (
    <Stack gap="md">
      <Paper withBorder p="md">
        <Text fw={600} mb="sm">Tuner Device</Text>
        <Stack gap="sm">
          <TextInput
            label="Device name (shown to Plex/Emby)"
            value={tunerName}
            onChange={e => setTunerName(e.currentTarget.value)}
          />
          <NumberInput
            label="Tuner count (max concurrent streams)"
            value={tunerCount}
            onChange={v => setTunerCount(Number(v))}
            min={1}
            max={100}
          />
        </Stack>
      </Paper>

      <Paper withBorder p="md">
        <Text fw={600} mb="sm">DVR</Text>
        <Stack gap="sm">
          <TextInput
            label="Recording path template"
            value={dvrPath}
            onChange={e => setDvrPath(e.currentTarget.value)}
            description="Tokens: {state_dir} {title} {channel} {date} {time} {year} {month} {day}"
          />
          <Group grow>
            <NumberInput
              label="Pad before (seconds)"
              value={dvrPadBefore}
              onChange={v => setDvrPadBefore(Number(v))}
              min={0}
            />
            <NumberInput
              label="Pad after (seconds)"
              value={dvrPadAfter}
              onChange={v => setDvrPadAfter(Number(v))}
              min={0}
            />
          </Group>
        </Stack>
      </Paper>

      <Paper withBorder p="md">
        <Text fw={600} mb="sm">System</Text>
        <Stack gap="xs">
          <Group gap="xs">
            <Text size="sm" c="dimmed" w={120}>Version</Text>
            <Code>{version}</Code>
          </Group>
          <Group gap="xs">
            <Text size="sm" c="dimmed" w={120}>Port</Text>
            <Code>{port}</Code>
          </Group>
          <Group gap="xs">
            <Text size="sm" c="dimmed" w={120}>API</Text>
            <Anchor size="sm" href="/api/" target="_blank">/api/</Anchor>
          </Group>
        </Stack>
      </Paper>

      <Group justify="flex-end">
        <Button
          color="teal"
          leftSection={<IconDeviceFloppy size={14} />}
          loading={save.isPending}
          onClick={() => save.mutate()}
        >
          Save Settings
        </Button>
      </Group>
    </Stack>
  )
}

// ── Stream Profiles tab ────────────────────────────────────────────────────
function StreamProfilesTab() {
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<StreamProfile | null>(null)

  const { data: profiles = [], isLoading } = useQuery({
    queryKey: ['stream-profiles'],
    queryFn: () => streamProfilesApi.list(),
  })

  const del = useMutation({
    mutationFn: (id: number) => streamProfilesApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['stream-profiles'] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Text size="sm" c="dimmed">
          Stream profiles control how channels are delivered (transcode, proxy, redirect, or via external tools).
        </Text>
        <Button size="xs" leftSection={<IconPlus size={14} />} color="teal"
          onClick={() => { setEditTarget(null); setModalOpen(true) }}>
          New Profile
        </Button>
      </Group>

      {isLoading ? (
        <Text size="sm" c="dimmed">Loading…</Text>
      ) : profiles.length === 0 ? (
        <Alert icon={<IconAlertCircle size={16} />} color="gray">
          No stream profiles. The built-in proxy mode is used by default.
        </Alert>
      ) : (
        <Paper withBorder style={{ overflow: 'hidden' }}>
          <ScrollArea>
            <Table striped highlightOnHover withRowBorders={false} fz="sm">
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>Name</Table.Th>
                  <Table.Th>Type</Table.Th>
                  <Table.Th>Default</Table.Th>
                  <Table.Th>Created</Table.Th>
                  <Table.Th style={{ width: 80 }} />
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {profiles.map(p => (
                  <Table.Tr key={p.id}>
                    <Table.Td>
                      <Group gap="xs">
                        <IconPlugConnected size={14} style={{ opacity: 0.6 }} />
                        <Text size="sm">{p.name}</Text>
                      </Group>
                    </Table.Td>
                    <Table.Td>
                      <Badge size="xs" color="blue" variant="light">{p.type}</Badge>
                    </Table.Td>
                    <Table.Td>
                      {p.is_default && <Badge size="xs" color="teal">default</Badge>}
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">{new Date(p.created_at).toLocaleDateString()}</Text>
                    </Table.Td>
                    <Table.Td>
                      <Group gap={4} wrap="nowrap">
                        <Tooltip label="Edit">
                          <ActionIcon size="xs" variant="subtle" color="yellow"
                            onClick={() => { setEditTarget(p); setModalOpen(true) }}>
                            <IconEdit size={14} />
                          </ActionIcon>
                        </Tooltip>
                        <Tooltip label="Delete">
                          <ActionIcon size="xs" variant="subtle" color="red"
                            onClick={() => {
                              if (confirm(`Delete profile "${p.name}"?`)) del.mutate(p.id)
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
        </Paper>
      )}

      <ProfileModal
        opened={modalOpen}
        onClose={() => { setModalOpen(false); setEditTarget(null) }}
        initial={editTarget}
      />
    </Stack>
  )
}

// ── Provider tab ───────────────────────────────────────────────────────────
function ProviderTab() {
  const qc = useQueryClient()
  const { data: settings } = useQuery({
    queryKey: ['settings'],
    queryFn: () => settingsApi.get(),
  })

  const [userAgent, setUserAgent] = useState('')
  const [xtreamUser, setXtreamUser] = useState('')
  const [xtreamPass, setXtreamPass] = useState('')
  const [cookieText, setCookieText] = useState('')
  const [cookieStatus, setCookieStatus] = useState<'idle' | 'saving' | 'ok' | 'error'>('idle')

  useEffect(() => {
    if (!settings) return
    setUserAgent(settings['provider.user_agent'] ?? '')
    setXtreamUser(settings['xtream.user'] ?? '')
    setXtreamPass(settings['xtream.pass'] ?? '')
  }, [settings])

  const saveUA = useMutation({
    mutationFn: () => settingsApi.patch({
      'provider.user_agent': userAgent,
      'xtream.user': xtreamUser,
      'xtream.pass': xtreamPass,
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['settings'] })
      notifications.show({ message: 'Provider settings saved', color: 'teal' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  async function importCookies() {
    if (!cookieText.trim()) return
    setCookieStatus('saving')
    try {
      const csrf = boot().csrf
      const headers: Record<string, string> = { 'Content-Type': 'text/plain' }
      if (csrf) headers['X-IPTVTunerr-CSRF'] = csrf
      const res = await fetch('/api/v2/settings/cookie-jar', { method: 'POST', headers, body: cookieText })
      if (!res.ok) throw new Error(`${res.status}`)
      setCookieStatus('ok')
      setCookieText('')
      notifications.show({ message: 'Cookie jar imported', color: 'teal' })
    } catch (e) {
      setCookieStatus('error')
      notifications.show({ message: `Import failed: ${e}`, color: 'red' })
    }
  }

  const uaPresetValue = UA_PRESETS.includes(userAgent) ? userAgent : 'custom'

  return (
    <Stack gap="md">
      <Paper withBorder p="md">
        <Text fw={600} mb="sm">User-Agent</Text>
        <Stack gap="sm">
          <Select
            label="Preset"
            data={[
              { value: '', label: 'Default (iptvTunerr)' },
              { value: 'Lavf/58.76.100', label: 'Lavf (FFmpeg)' },
              { value: 'VLC/3.0.18 LibVLC/3.0.18', label: 'VLC' },
              { value: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)', label: 'Browser (generic)' },
              { value: 'custom', label: 'Custom…' },
            ]}
            value={uaPresetValue}
            onChange={v => { if (v !== null && v !== 'custom') setUserAgent(v) }}
            clearable={false}
          />
          <TextInput
            label="User-Agent string"
            value={userAgent}
            onChange={e => setUserAgent(e.currentTarget.value)}
            placeholder="Leave blank to use iptvTunerr default"
          />
        </Stack>
        <Group justify="flex-end" mt="sm">
          <Button size="xs" color="teal" leftSection={<IconDeviceFloppy size={14} />}
            loading={saveUA.isPending} onClick={() => saveUA.mutate()}>
            Save
          </Button>
        </Group>
      </Paper>

      <Paper withBorder p="md">
        <Text fw={600} mb="sm">Xtream Credentials</Text>
        <Text size="sm" c="dimmed" mb="sm">
          Used by the VODs page to query movie and series data from the tuner's Xtream API.
        </Text>
        <Stack gap="sm">
          <TextInput label="Xtream username" value={xtreamUser} onChange={e => setXtreamUser(e.currentTarget.value)} />
          <TextInput label="Xtream password" value={xtreamPass} onChange={e => setXtreamPass(e.currentTarget.value)} />
        </Stack>
      </Paper>

      <Paper withBorder p="md">
        <Text fw={600} mb="sm">Cookie Jar</Text>
        <Text size="sm" c="dimmed" mb="sm">
          Import a Netscape-format cookie file to bypass Cloudflare and similar systems.
          Export using a browser extension such as "Get cookies.txt LOCALLY".
        </Text>
        <Textarea
          placeholder={"# Netscape HTTP Cookie File\n.example.com\tTRUE\t/\tFALSE\t0\tcf_clearance\tabc123..."}
          value={cookieText}
          onChange={e => setCookieText(e.currentTarget.value)}
          autosize
          minRows={5}
          maxRows={12}
          styles={{ input: { fontFamily: 'monospace', fontSize: 11 } }}
        />
        <Group justify="flex-end" mt="sm">
          <Button
            size="xs"
            color="teal"
            disabled={!cookieText.trim()}
            loading={cookieStatus === 'saving'}
            onClick={importCookies}
          >
            Import Cookie Jar
          </Button>
        </Group>
      </Paper>
    </Stack>
  )
}

// ── System tab ────────────────────────────────────────────────────────────
const QUICK_LINKS = [
  { label: 'Runtime snapshot', href: '/api/debug/runtime.json' },
  { label: 'Health check', href: '/api/healthz' },
  { label: 'Discover', href: '/api/discover.json' },
  { label: 'Provider profile', href: '/api/provider/profile.json' },
  { label: 'Guide health', href: '/api/guide/health.json' },
  { label: 'Recorder', href: '/api/recordings/recorder.json' },
  { label: 'Autopilot report', href: '/api/autopilot/report.json' },
]

function SystemTab() {
  const qc = useQueryClient()
  const [replayBytes, setReplayBytes] = useState<number>(0)
  const [replayOpen, setReplayOpen] = useState(false)
  const [stallSec, setStallSec] = useState<number>(0)
  const [stallOpen, setStallOpen] = useState(false)

  const runtime = useQuery({
    queryKey: ['system-runtime'],
    queryFn: () => api.get<Record<string, unknown>>('/api/debug/runtime.json'),
    staleTime: 60_000,
  })

  const d = runtime.data as Record<string, unknown> | undefined
  const tuner = d?.tuner as Record<string, unknown> | undefined

  const applyReplay = useMutation({
    mutationFn: (n: number) => api.post('/api/ops/actions/shared-relay-replay', { shared_relay_replay_bytes: n }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['system-runtime'] })
      notifications.show({ message: 'Relay buffer updated', color: 'teal' })
      setReplayOpen(false)
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const applyStall = useMutation({
    mutationFn: (n: number) => api.post('/api/ops/actions/virtual-channel-live-stall', { virtual_channel_recovery_live_stall_sec: n }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['system-runtime'] })
      notifications.show({ message: 'Stall threshold updated', color: 'teal' })
      setStallOpen(false)
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const evidenceBundle = useMutation({
    mutationFn: () => api.post('/api/ops/actions/evidence-intake-start'),
    onSuccess: () => notifications.show({ message: 'Evidence bundle creation started', color: 'teal' }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Stack gap="md">
      {/* Runtime snapshot */}
      <Paper withBorder p="md">
        <Text fw={600} mb="sm">Runtime Snapshot</Text>
        {runtime.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
          : runtime.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Runtime info unavailable.</Alert>
          : (
            <Table withRowBorders={false} fz="sm">
              <Table.Tbody>
                <Table.Tr><Table.Td c="dimmed" w={240}>Version</Table.Td><Table.Td><Code>{String(d?.version ?? tuner?.version ?? '—')}</Code></Table.Td></Table.Tr>
                <Table.Tr><Table.Td c="dimmed">Tuner limit</Table.Td><Table.Td>{String(d?.tuner_limit ?? tuner?.tuner_limit ?? '—')}</Table.Td></Table.Tr>
                <Table.Tr>
                  <Table.Td c="dimmed">Shared relay replay bytes</Table.Td>
                  <Table.Td>
                    <Group gap="xs">
                      <Text size="sm">{String(tuner?.shared_relay_replay_bytes ?? d?.shared_relay_replay_bytes ?? '—')}</Text>
                      <Button size="xs" variant="subtle"
                        onClick={() => {
                          setReplayBytes(Number(tuner?.shared_relay_replay_bytes ?? d?.shared_relay_replay_bytes ?? 0))
                          setReplayOpen(true)
                        }}>Edit</Button>
                    </Group>
                  </Table.Td>
                </Table.Tr>
                <Table.Tr>
                  <Table.Td c="dimmed">Virtual channel stall sec</Table.Td>
                  <Table.Td>
                    <Group gap="xs">
                      <Text size="sm">{String(tuner?.virtual_channel_recovery_live_stall_sec ?? d?.virtual_channel_recovery_live_stall_sec ?? '—')}</Text>
                      <Button size="xs" variant="subtle"
                        onClick={() => {
                          setStallSec(Number(tuner?.virtual_channel_recovery_live_stall_sec ?? d?.virtual_channel_recovery_live_stall_sec ?? 0))
                          setStallOpen(true)
                        }}>Edit</Button>
                    </Group>
                  </Table.Td>
                </Table.Tr>
                {!!(d?.provider_base_url ?? tuner?.provider_base_url) && (
                  <Table.Tr><Table.Td c="dimmed">Provider base URL</Table.Td><Table.Td><Code fz="xs">{String(d?.provider_base_url ?? tuner?.provider_base_url)}</Code></Table.Td></Table.Tr>
                )}
              </Table.Tbody>
            </Table>
          )}
      </Paper>

      {/* Quick links */}
      <Paper withBorder p="md">
        <Text fw={600} mb="sm">Endpoint Browser</Text>
        <Stack gap="xs">
          {QUICK_LINKS.map(({ label, href }) => (
            <Group key={href} gap="xs">
              <IconExternalLink size={14} style={{ opacity: 0.5 }} />
              <Anchor size="sm" href={href} target="_blank">{label}</Anchor>
              <Code fz="xs" c="dimmed">{href}</Code>
            </Group>
          ))}
        </Stack>
      </Paper>

      {/* Diagnostics actions */}
      <Paper withBorder p="md">
        <Text fw={600} mb="sm">Diagnostics</Text>
        <Text size="xs" c="dimmed" mb="sm">
          Guide refresh and stream attempt history are managed in M3U &amp; EPG → Guide Health and Stats → Routing respectively.
        </Text>
        <Group gap="xs">
          <Button size="xs" color="blue" variant="outline"
            loading={evidenceBundle.isPending}
            onClick={() => { if (confirm('Create a diagnostic evidence bundle?')) evidenceBundle.mutate() }}>
            Create Evidence Bundle
          </Button>
        </Group>
      </Paper>

      {/* Replay buffer modal */}
      <Modal opened={replayOpen} onClose={() => setReplayOpen(false)} title="Set Relay Replay Buffer" size="sm">
        <Stack gap="sm">
          <Text size="sm" c="dimmed">Bytes buffered per shared relay for late subscribers.</Text>
          <NumberInput label="Bytes" value={replayBytes} onChange={v => setReplayBytes(Number(v))} min={0} />
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setReplayOpen(false)}>Cancel</Button>
            <Button color="teal" loading={applyReplay.isPending}
              onClick={() => { if (confirm(`Set replay buffer to ${replayBytes} bytes?`)) applyReplay.mutate(replayBytes) }}>
              Apply
            </Button>
          </Group>
        </Stack>
      </Modal>

      {/* Stall sec modal */}
      <Modal opened={stallOpen} onClose={() => setStallOpen(false)} title="Set Virtual Channel Stall Threshold" size="sm">
        <Stack gap="sm">
          <Text size="sm" c="dimmed">Seconds of stall before virtual channel live recovery is triggered.</Text>
          <NumberInput label="Seconds" value={stallSec} onChange={v => setStallSec(Number(v))} min={0} />
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setStallOpen(false)}>Cancel</Button>
            <Button color="teal" loading={applyStall.isPending}
              onClick={() => { if (confirm(`Set stall threshold to ${stallSec}s?`)) applyStall.mutate(stallSec) }}>
              Apply
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  )
}

// ── Identity tab ──────────────────────────────────────────────────────────
function IdentityTab() {
  const [oidcModalOpen, setOidcModalOpen] = useState(false)
  const [oidcTargets, setOidcTargets] = useState<string[]>([])
  const [oidcBootstrap, setOidcBootstrap] = useState('')

  const oidcAudit = useQuery({
    queryKey: ['oidc-audit'],
    queryFn: () => fetch('/deck/oidc-migration-audit.json').then(r => r.ok ? r.json() : null),
    staleTime: 60_000,
  })
  const migrationAudit = useQuery({
    queryKey: ['migration-audit'],
    queryFn: () => fetch('/deck/migration-audit.json').then(r => r.ok ? r.json() : null),
    staleTime: 60_000,
  })
  const identityAudit = useQuery({
    queryKey: ['identity-audit'],
    queryFn: () => fetch('/deck/identity-migration-audit.json').then(r => r.ok ? r.json() : null),
    staleTime: 60_000,
  })

  const applyOidc = useMutation({
    mutationFn: () => fetch('/deck/oidc-migration-apply.json', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(boot().csrf ? { 'X-IPTVTunerr-CSRF': boot().csrf } : {}) },
      body: JSON.stringify({ targets: oidcTargets, bootstrap_password: oidcBootstrap || undefined }),
    }).then(r => { if (!r.ok) throw new Error(`${r.status}`); return r.json().catch(() => null) }),
    onSuccess: () => {
      notifications.show({ message: 'OIDC migration applied', color: 'teal' })
      setOidcModalOpen(false)
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  function renderWorkflowCard(
    label: string,
    query: ReturnType<typeof useQuery>,
    extra?: React.ReactNode,
  ) {
    const d = query.data as Record<string, unknown> | null | undefined
    return (
      <Paper withBorder p="md">
        <Text fw={600} mb="sm">{label}</Text>
        {query.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
          : query.isError || !d ? <Alert icon={<IconAlertCircle size={16} />} color="gray">{label} workflow not available.</Alert>
          : (
            <Stack gap="xs">
              <Group gap="md">
                <Box>
                  <Text size="xs" c="dimmed">Configured</Text>
                  <Badge size="sm" color={d.configured ? 'teal' : 'gray'}>{d.configured ? 'Yes' : 'No'}</Badge>
                </Box>
                {!!d.status && (
                  <Box>
                    <Text size="xs" c="dimmed">Status</Text>
                    <Badge size="sm" variant="outline">{String(d.status)}</Badge>
                  </Box>
                )}
                {!!d.targets && (
                  <Box>
                    <Text size="xs" c="dimmed">Targets</Text>
                    <Text size="sm">{Array.isArray(d.targets) ? (d.targets as string[]).join(', ') : String(d.targets)}</Text>
                  </Box>
                )}
              </Group>
              {!!d.summary && <Text size="sm" c="dimmed">{String(d.summary)}</Text>}
              {extra}
            </Stack>
          )}
      </Paper>
    )
  }

  return (
    <Stack gap="md">
      {renderWorkflowCard('OIDC / Keycloak / Authentik Migration', oidcAudit,
        <Button size="xs" color="blue" variant="outline" onClick={() => setOidcModalOpen(true)}>
          Apply OIDC Migration…
        </Button>
      )}
      {renderWorkflowCard('Content Migration (Emby/Jellyfin)', migrationAudit)}
      {renderWorkflowCard('User / Identity Migration', identityAudit)}

      <Modal opened={oidcModalOpen} onClose={() => setOidcModalOpen(false)} title="Apply OIDC Migration" size="sm">
        <Stack gap="sm">
          <MultiSelect
            label="Targets"
            data={[
              { value: 'keycloak', label: 'Keycloak' },
              { value: 'authentik', label: 'Authentik' },
            ]}
            value={oidcTargets}
            onChange={setOidcTargets}
            placeholder="Select targets"
          />
          <TextInput
            label="Bootstrap password (optional)"
            type="password"
            value={oidcBootstrap}
            onChange={e => setOidcBootstrap(e.currentTarget.value)}
            placeholder="Leave blank to use existing"
          />
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setOidcModalOpen(false)}>Cancel</Button>
            <Button color="blue" loading={applyOidc.isPending}
              onClick={() => { if (confirm('Apply OIDC migration? This may modify your identity provider configuration.')) applyOidc.mutate() }}>
              Apply
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────
export function Settings() {
  return (
    <Stack gap="md" h="100%" style={{ overflow: 'hidden' }}>
      <Text size="lg" fw={600}>Settings</Text>

      <Paper withBorder p={0} style={{ flex: 1, overflow: 'hidden' }}>
        <Tabs defaultValue="general" style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
          <Tabs.List px="md" pt="xs">
            <Tabs.Tab value="general">General</Tabs.Tab>
            <Tabs.Tab value="stream-profiles">Stream Profiles</Tabs.Tab>
            <Tabs.Tab value="provider">Provider</Tabs.Tab>
            <Tabs.Tab value="system" leftSection={<IconServer size={14} />}>System</Tabs.Tab>
            <Tabs.Tab value="identity" leftSection={<IconShield size={14} />}>Identity</Tabs.Tab>
          </Tabs.List>

          <ScrollArea style={{ flex: 1 }}>
            <Tabs.Panel value="general" p="md">
              <GeneralTab />
            </Tabs.Panel>
            <Tabs.Panel value="stream-profiles" p="md">
              <StreamProfilesTab />
            </Tabs.Panel>
            <Tabs.Panel value="provider" p="md">
              <ProviderTab />
            </Tabs.Panel>
            <Tabs.Panel value="system" p="md">
              <SystemTab />
            </Tabs.Panel>
            <Tabs.Panel value="identity" p="md">
              <IdentityTab />
            </Tabs.Panel>
          </ScrollArea>
        </Tabs>
      </Paper>
    </Stack>
  )
}
