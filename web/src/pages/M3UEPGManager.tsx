import {
  Stack, Tabs, Group, Button, Text, Badge, ActionIcon, Tooltip,
  Paper, Table, ScrollArea, Modal, TextInput, Select, Switch,
  NumberInput, Divider, Box, Alert, Collapse, Checkbox,
  Textarea, Progress,
} from '@mantine/core'
import {
  IconPlus, IconRefresh, IconTrash, IconEdit, IconAlertCircle,
  IconFilter, IconUsers, IconHeartbeat,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { m3uAccountsApi, type M3UAccount, type M3UAccountInput, type M3UFilter } from '../api/m3u'
import { epgAccountsApi, type EPGAccount, type EPGAccountInput } from '../api/epg'
import { api } from '../api/client'

// ──────────────────────────────────────────────────────────────────
// M3U Account modal
// ──────────────────────────────────────────────────────────────────
function M3UAccountModal({
  opened, onClose, initial,
}: {
  opened: boolean
  onClose: () => void
  initial: M3UAccount | null
}) {
  const qc = useQueryClient()
  const isEdit = !!initial

  const [name, setName] = useState(initial?.name ?? '')
  const [accountType, setAccountType] = useState<'standard' | 'xtream'>(initial?.account_type ?? 'standard')
  const [url, setUrl] = useState(initial?.url ?? '')
  const [maxStreams, setMaxStreams] = useState<number>(initial?.max_streams ?? 0)
  const [userAgent, setUserAgent] = useState(initial?.user_agent ?? '')
  const [refreshHrs, setRefreshHrs] = useState<number>(initial?.refresh_interval_hrs ?? 24)
  const [staleRetention, setStaleRetention] = useState<number>(initial?.stale_retention_days ?? 7)
  const [vodScanning, setVodScanning] = useState(initial?.vod_scanning ?? false)
  const [isActive, setIsActive] = useState(initial?.is_active ?? true)
  const [advanced, setAdvanced] = useState(false)

  function reset(acct: M3UAccount | null) {
    setName(acct?.name ?? '')
    setAccountType(acct?.account_type ?? 'standard')
    setUrl(acct?.url ?? '')
    setMaxStreams(acct?.max_streams ?? 0)
    setUserAgent(acct?.user_agent ?? '')
    setRefreshHrs(acct?.refresh_interval_hrs ?? 24)
    setStaleRetention(acct?.stale_retention_days ?? 7)
    setVodScanning(acct?.vod_scanning ?? false)
    setIsActive(acct?.is_active ?? true)
    setAdvanced(false)
  }

  const save = useMutation({
    mutationFn: () => {
      const data: M3UAccountInput = {
        name, account_type: accountType, url, max_streams: maxStreams,
        user_agent: userAgent, refresh_interval_hrs: refreshHrs,
        stale_retention_days: staleRetention, vod_scanning: vodScanning, is_active: isActive,
      }
      return isEdit
        ? m3uAccountsApi.update(initial!.id, data)
        : m3uAccountsApi.create(data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['m3u-accounts'] })
      notifications.show({ message: isEdit ? 'Account updated' : 'Account added', color: 'teal' })
      onClose()
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Modal
      opened={opened}
      onClose={() => { reset(null); onClose() }}
      title={isEdit ? `Edit — ${initial?.name}` : 'Add M3U Account'}
      size="md"
    >
      <Stack gap="sm">
        <TextInput label="Name" value={name} onChange={e => setName(e.currentTarget.value)} required />
        <Select
          label="Type"
          data={[
            { value: 'standard', label: 'Standard M3U' },
            { value: 'xtream', label: 'Xtream Codes' },
          ]}
          value={accountType}
          onChange={v => setAccountType((v ?? 'standard') as 'standard' | 'xtream')}
        />
        <TextInput label="URL" value={url} onChange={e => setUrl(e.currentTarget.value)}
          placeholder="https://provider.example/get.php?..." />
        <Switch label="Active" checked={isActive} onChange={e => setIsActive(e.currentTarget.checked)} />

        <Collapse in={advanced}>
          <Stack gap="sm">
            <NumberInput label="Max streams (0 = unlimited)" value={maxStreams}
              onChange={v => setMaxStreams(Number(v))} min={0} />
            <NumberInput label="Refresh interval (hours)" value={refreshHrs}
              onChange={v => setRefreshHrs(Number(v))} min={1} />
            <NumberInput label="Stale retention (days)" value={staleRetention}
              onChange={v => setStaleRetention(Number(v))} min={0} />
            <TextInput label="User-Agent override" value={userAgent}
              onChange={e => setUserAgent(e.currentTarget.value)} />
            <Switch label="VOD scanning" checked={vodScanning}
              onChange={e => setVodScanning(e.currentTarget.checked)} />
          </Stack>
        </Collapse>

        <Button variant="subtle" size="xs" onClick={() => setAdvanced(p => !p)}>
          {advanced ? 'Hide advanced' : 'Show advanced'}
        </Button>

        <Group justify="flex-end" mt="sm">
          <Button variant="default" onClick={() => { reset(null); onClose() }}>Cancel</Button>
          <Button color="teal" loading={save.isPending} onClick={() => save.mutate()}>
            {isEdit ? 'Save' : 'Add'}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

// ──────────────────────────────────────────────────────────────────
// Filters editor modal
// ──────────────────────────────────────────────────────────────────
function FiltersModal({
  account, opened, onClose,
}: {
  account: M3UAccount | null
  opened: boolean
  onClose: () => void
}) {
  const qc = useQueryClient()
  const { data: filters = [] } = useQuery({
    queryKey: ['m3u-filters', account?.id],
    queryFn: () => m3uAccountsApi.listFilters(account!.id),
    enabled: !!account,
  })
  const [localFilters, setLocalFilters] = useState<M3UFilter[]>([])
  const [synced, setSynced] = useState(false)

  if (filters.length && !synced) {
    setLocalFilters(filters)
    setSynced(true)
  }
  if (!opened && synced) setSynced(false)

  function addFilter() {
    setLocalFilters(f => [...f, { field: 'group', pattern: '', exclude: false, case_sens: false }])
  }
  function removeFilter(i: number) {
    setLocalFilters(f => f.filter((_, j) => j !== i))
  }
  function updateFilter<K extends keyof M3UFilter>(i: number, key: K, val: M3UFilter[K]) {
    setLocalFilters(f => f.map((fi, j) => j === i ? { ...fi, [key]: val } : fi))
  }

  const save = useMutation({
    mutationFn: () => m3uAccountsApi.saveFilters(account!.id, localFilters),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['m3u-filters', account?.id] })
      notifications.show({ message: 'Filters saved', color: 'teal' })
      onClose()
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Modal opened={opened} onClose={onClose} title={`Filters — ${account?.name}`} size="lg">
      <Stack gap="sm">
        <Text size="xs" c="dimmed">
          Filters are evaluated in order. First matching include (non-exclude) filter wins.
          Exclude filters drop the stream regardless.
        </Text>
        {localFilters.map((f, i) => (
          <Paper key={i} withBorder p="xs">
            <Group gap="xs" wrap="nowrap" align="flex-end">
              <Select
                label="Field"
                size="xs"
                data={[
                  { value: 'group', label: 'Group' },
                  { value: 'name', label: 'Name' },
                  { value: 'url', label: 'URL' },
                ]}
                value={f.field}
                onChange={v => updateFilter(i, 'field', (v ?? 'group') as M3UFilter['field'])}
                style={{ width: 90 }}
              />
              <TextInput
                label="Pattern (regex)"
                size="xs"
                value={f.pattern}
                onChange={e => updateFilter(i, 'pattern', e.currentTarget.value)}
                style={{ flex: 1 }}
              />
              <Checkbox
                label="Exclude"
                size="xs"
                checked={f.exclude}
                onChange={e => updateFilter(i, 'exclude', e.currentTarget.checked)}
              />
              <Checkbox
                label="Case sensitive"
                size="xs"
                checked={f.case_sens}
                onChange={e => updateFilter(i, 'case_sens', e.currentTarget.checked)}
              />
              <ActionIcon size="sm" color="red" variant="subtle" onClick={() => removeFilter(i)}>
                <IconTrash size={14} />
              </ActionIcon>
            </Group>
          </Paper>
        ))}
        <Button size="xs" variant="subtle" leftSection={<IconPlus size={14} />} onClick={addFilter}>
          Add filter
        </Button>
        <Group justify="flex-end" mt="sm">
          <Button variant="default" onClick={onClose}>Cancel</Button>
          <Button color="teal" loading={save.isPending} onClick={() => save.mutate()}>Save</Button>
        </Group>
      </Stack>
    </Modal>
  )
}

// ──────────────────────────────────────────────────────────────────
// Groups viewer modal (read-only with toggles)
// ──────────────────────────────────────────────────────────────────
function GroupsModal({
  account, opened, onClose,
}: {
  account: M3UAccount | null
  opened: boolean
  onClose: () => void
}) {
  const qc = useQueryClient()
  const { data: groups = [] } = useQuery({
    queryKey: ['m3u-groups', account?.id],
    queryFn: () => m3uAccountsApi.listGroups(account!.id),
    enabled: !!account && opened,
  })

  const toggle = useMutation({
    mutationFn: ({ groupId, enabled }: { groupId: number; enabled: boolean }) =>
      m3uAccountsApi.updateGroup(account!.id, groupId, { enabled }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['m3u-groups', account?.id] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Modal opened={opened} onClose={onClose} title={`Groups — ${account?.name}`} size="lg">
      <ScrollArea mah={500}>
        <Table striped withRowBorders={false} fz="sm">
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Group</Table.Th>
              <Table.Th style={{ width: 80 }}>Streams</Table.Th>
              <Table.Th style={{ width: 80 }}>Enabled</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {groups.length === 0 ? (
              <Table.Tr>
                <Table.Td colSpan={3}>
                  <Text size="sm" c="dimmed" ta="center">No groups found</Text>
                </Table.Td>
              </Table.Tr>
            ) : groups.map(g => (
              <Table.Tr key={g.id}>
                <Table.Td><Text size="sm">{g.name}</Text></Table.Td>
                <Table.Td><Text size="xs" c="dimmed">{g.stream_count ?? 0}</Text></Table.Td>
                <Table.Td>
                  <Switch
                    size="xs"
                    checked={g.enabled}
                    onChange={e => toggle.mutate({ groupId: g.id, enabled: e.currentTarget.checked })}
                  />
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </ScrollArea>
      <Group justify="flex-end" mt="sm">
        <Button variant="default" onClick={onClose}>Close</Button>
      </Group>
    </Modal>
  )
}

// ──────────────────────────────────────────────────────────────────
// EPG Account modal
// ──────────────────────────────────────────────────────────────────
function EPGAccountModal({
  opened, onClose, initial,
}: {
  opened: boolean
  onClose: () => void
  initial: EPGAccount | null
}) {
  const qc = useQueryClient()
  const isEdit = !!initial

  const [name, setName] = useState(initial?.name ?? '')
  const [sourceType, setSourceType] = useState<'xmltv' | 'sd' | 'dummy'>(initial?.source_type ?? 'xmltv')
  const [url, setUrl] = useState(initial?.url ?? '')
  const [apiKey, setApiKey] = useState(initial?.api_key ?? '')
  const [refreshHrs, setRefreshHrs] = useState<number>(initial?.refresh_interval_hrs ?? 12)
  const [priority, setPriority] = useState<number>(initial?.priority ?? 0)
  const [isActive, setIsActive] = useState(initial?.is_active ?? true)
  const [dummyConfig, setDummyConfig] = useState(initial?.dummy_config_json ?? '')

  function reset() {
    setName(initial?.name ?? '')
    setSourceType(initial?.source_type ?? 'xmltv')
    setUrl(initial?.url ?? '')
    setApiKey(initial?.api_key ?? '')
    setRefreshHrs(initial?.refresh_interval_hrs ?? 12)
    setPriority(initial?.priority ?? 0)
    setIsActive(initial?.is_active ?? true)
    setDummyConfig(initial?.dummy_config_json ?? '')
  }

  const save = useMutation({
    mutationFn: () => {
      const data: EPGAccountInput = {
        name, source_type: sourceType, url, api_key: apiKey,
        refresh_interval_hrs: refreshHrs, priority, is_active: isActive,
        dummy_config_json: dummyConfig,
      }
      return isEdit
        ? epgAccountsApi.update(initial!.id, data)
        : epgAccountsApi.create(data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['epg-accounts'] })
      notifications.show({ message: isEdit ? 'EPG account updated' : 'EPG account added', color: 'teal' })
      onClose()
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Modal
      opened={opened}
      onClose={() => { reset(); onClose() }}
      title={isEdit ? `Edit — ${initial?.name}` : 'Add EPG Source'}
      size="md"
    >
      <Stack gap="sm">
        <TextInput label="Name" value={name} onChange={e => setName(e.currentTarget.value)} required />
        <Select
          label="Source type"
          data={[
            { value: 'xmltv', label: 'XMLTV' },
            { value: 'sd', label: 'Schedules Direct' },
            { value: 'dummy', label: 'Dummy pattern' },
          ]}
          value={sourceType}
          onChange={v => setSourceType((v ?? 'xmltv') as 'xmltv' | 'sd' | 'dummy')}
        />

        {sourceType !== 'dummy' && (
          <TextInput
            label={sourceType === 'sd' ? 'Username' : 'XMLTV URL'}
            value={url}
            onChange={e => setUrl(e.currentTarget.value)}
          />
        )}

        {sourceType === 'sd' && (
          <TextInput
            label="Password"
            type="password"
            value={apiKey}
            onChange={e => setApiKey(e.currentTarget.value)}
          />
        )}

        {sourceType === 'dummy' && (
          <Textarea
            label="Dummy pattern config (JSON)"
            value={dummyConfig}
            onChange={e => setDummyConfig(e.currentTarget.value)}
            minRows={5}
            placeholder='{"title_template": "{{name}}", "duration_mins": 60}'
          />
        )}

        <NumberInput label="Refresh interval (hours)" value={refreshHrs}
          onChange={v => setRefreshHrs(Number(v))} min={1} />
        <NumberInput label="Priority (higher = preferred)" value={priority}
          onChange={v => setPriority(Number(v))} />
        <Switch label="Active" checked={isActive}
          onChange={e => setIsActive(e.currentTarget.checked)} />

        <Group justify="flex-end" mt="sm">
          <Button variant="default" onClick={() => { reset(); onClose() }}>Cancel</Button>
          <Button color="teal" loading={save.isPending} onClick={() => save.mutate()}>
            {isEdit ? 'Save' : 'Add'}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

// ──────────────────────────────────────────────────────────────────
// M3U Accounts tab
// ──────────────────────────────────────────────────────────────────
function M3UTab() {
  const qc = useQueryClient()
  const { data: accounts = [], isLoading } = useQuery({
    queryKey: ['m3u-accounts'],
    queryFn: () => m3uAccountsApi.list(),
  })

  const [modalOpen, setModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<M3UAccount | null>(null)
  const [filtersTarget, setFiltersTarget] = useState<M3UAccount | null>(null)
  const [groupsTarget, setGroupsTarget] = useState<M3UAccount | null>(null)

  const del = useMutation({
    mutationFn: (id: number) => m3uAccountsApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['m3u-accounts'] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const refresh = useMutation({
    mutationFn: (id: number) => m3uAccountsApi.refresh(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['m3u-accounts'] })
      notifications.show({ message: 'Refresh triggered', color: 'teal' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <>
      <Group justify="space-between" mb="sm">
        <Text fw={500}>M3U Accounts</Text>
        <Button size="xs" leftSection={<IconPlus size={14} />} color="teal"
          onClick={() => { setEditTarget(null); setModalOpen(true) }}>
          Add Account
        </Button>
      </Group>

      {isLoading ? (
        <Text size="sm" c="dimmed">Loading…</Text>
      ) : accounts.length === 0 ? (
        <Alert icon={<IconAlertCircle size={16} />} color="gray">
          No M3U accounts yet. Add one to start pulling streams.
        </Alert>
      ) : (
        <ScrollArea>
          <Table striped highlightOnHover withRowBorders={false} fz="sm">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Name</Table.Th>
                <Table.Th>Type</Table.Th>
                <Table.Th>Streams</Table.Th>
                <Table.Th>Refresh</Table.Th>
                <Table.Th>Last refreshed</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th style={{ width: 140 }} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {accounts.map(a => (
                <Table.Tr key={a.id}>
                  <Table.Td><Text size="sm">{a.name}</Text></Table.Td>
                  <Table.Td>
                    <Badge size="xs" color={a.account_type === 'xtream' ? 'grape' : 'blue'} variant="outline">
                      {a.account_type}
                    </Badge>
                  </Table.Td>
                  <Table.Td><Text size="xs" c="dimmed">{a.stream_count ?? 0}</Text></Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {a.refresh_cron || `${a.refresh_interval_hrs}h`}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {a.last_refreshed_at
                        ? new Date(a.last_refreshed_at).toLocaleString()
                        : 'Never'}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Badge size="xs" color={a.is_active ? 'teal' : 'gray'}>
                      {a.is_active ? 'Active' : 'Disabled'}
                    </Badge>
                  </Table.Td>
                  <Table.Td>
                    <Group gap={4} wrap="nowrap">
                      <Tooltip label="Refresh now">
                        <ActionIcon size="xs" variant="subtle" color="teal"
                          onClick={() => refresh.mutate(a.id)}>
                          <IconRefresh size={14} />
                        </ActionIcon>
                      </Tooltip>
                      <Tooltip label="Filters">
                        <ActionIcon size="xs" variant="subtle" color="violet"
                          onClick={() => setFiltersTarget(a)}>
                          <IconFilter size={14} />
                        </ActionIcon>
                      </Tooltip>
                      <Tooltip label="Groups">
                        <ActionIcon size="xs" variant="subtle" color="cyan"
                          onClick={() => setGroupsTarget(a)}>
                          <IconUsers size={14} />
                        </ActionIcon>
                      </Tooltip>
                      <Tooltip label="Edit">
                        <ActionIcon size="xs" variant="subtle" color="yellow"
                          onClick={() => { setEditTarget(a); setModalOpen(true) }}>
                          <IconEdit size={14} />
                        </ActionIcon>
                      </Tooltip>
                      <Tooltip label="Delete">
                        <ActionIcon size="xs" variant="subtle" color="red"
                          onClick={() => {
                            if (confirm(`Delete "${a.name}"?`)) del.mutate(a.id)
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

      <M3UAccountModal
        opened={modalOpen}
        onClose={() => setModalOpen(false)}
        initial={editTarget}
      />
      <FiltersModal
        account={filtersTarget}
        opened={!!filtersTarget}
        onClose={() => setFiltersTarget(null)}
      />
      <GroupsModal
        account={groupsTarget}
        opened={!!groupsTarget}
        onClose={() => setGroupsTarget(null)}
      />
    </>
  )
}

// ──────────────────────────────────────────────────────────────────
// EPG Accounts tab
// ──────────────────────────────────────────────────────────────────
function EPGTab() {
  const qc = useQueryClient()
  const { data: accounts = [], isLoading } = useQuery({
    queryKey: ['epg-accounts'],
    queryFn: () => epgAccountsApi.list(),
  })

  const [modalOpen, setModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<EPGAccount | null>(null)

  const del = useMutation({
    mutationFn: (id: number) => epgAccountsApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['epg-accounts'] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const refresh = useMutation({
    mutationFn: (id: number) => epgAccountsApi.refresh(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['epg-accounts'] })
      notifications.show({ message: 'Refresh triggered', color: 'teal' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const sourceTypeColor = (t: string) => ({ xmltv: 'orange', sd: 'blue', dummy: 'gray' }[t] ?? 'gray')

  return (
    <>
      <Group justify="space-between" mb="sm">
        <Text fw={500}>EPG Sources</Text>
        <Button size="xs" leftSection={<IconPlus size={14} />} color="teal"
          onClick={() => { setEditTarget(null); setModalOpen(true) }}>
          Add Source
        </Button>
      </Group>

      {isLoading ? (
        <Text size="sm" c="dimmed">Loading…</Text>
      ) : accounts.length === 0 ? (
        <Alert icon={<IconAlertCircle size={16} />} color="gray">
          No EPG sources yet. Add an XMLTV feed or Schedules Direct account.
        </Alert>
      ) : (
        <ScrollArea>
          <Table striped highlightOnHover withRowBorders={false} fz="sm">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Name</Table.Th>
                <Table.Th>Type</Table.Th>
                <Table.Th>Priority</Table.Th>
                <Table.Th>Refresh</Table.Th>
                <Table.Th>Last refreshed</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th style={{ width: 100 }} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {accounts.map(a => (
                <Table.Tr key={a.id}>
                  <Table.Td><Text size="sm">{a.name}</Text></Table.Td>
                  <Table.Td>
                    <Badge size="xs" color={sourceTypeColor(a.source_type)} variant="outline">
                      {a.source_type}
                    </Badge>
                  </Table.Td>
                  <Table.Td><Text size="xs" c="dimmed">{a.priority}</Text></Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {a.refresh_cron || `${a.refresh_interval_hrs}h`}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Text size="xs" c="dimmed">
                      {a.last_refreshed_at
                        ? new Date(a.last_refreshed_at).toLocaleString()
                        : 'Never'}
                    </Text>
                  </Table.Td>
                  <Table.Td>
                    <Badge size="xs" color={a.is_active ? 'teal' : 'gray'}>
                      {a.is_active ? 'Active' : 'Disabled'}
                    </Badge>
                  </Table.Td>
                  <Table.Td>
                    <Group gap={4} wrap="nowrap">
                      <Tooltip label="Refresh now">
                        <ActionIcon size="xs" variant="subtle" color="teal"
                          onClick={() => refresh.mutate(a.id)}>
                          <IconRefresh size={14} />
                        </ActionIcon>
                      </Tooltip>
                      <Tooltip label="Edit">
                        <ActionIcon size="xs" variant="subtle" color="yellow"
                          onClick={() => { setEditTarget(a); setModalOpen(true) }}>
                          <IconEdit size={14} />
                        </ActionIcon>
                      </Tooltip>
                      <Tooltip label="Delete">
                        <ActionIcon size="xs" variant="subtle" color="red"
                          onClick={() => {
                            if (confirm(`Delete "${a.name}"?`)) del.mutate(a.id)
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

      <EPGAccountModal
        opened={modalOpen}
        onClose={() => setModalOpen(false)}
        initial={editTarget}
      />
    </>
  )
}

// ──────────────────────────────────────────────────────────────────
// EPG Health tab
// ──────────────────────────────────────────────────────────────────
function GuideHealthTab() {
  const qc = useQueryClient()

  const health = useQuery({
    queryKey: ['guide-health'],
    queryFn: () => api.get<Record<string, unknown>>('/api/guide/health.json'),
    staleTime: 60_000,
  })
  const doctor = useQuery({
    queryKey: ['guide-doctor'],
    queryFn: () => api.get<Record<string, unknown>>('/api/guide/doctor.json'),
    staleTime: 60_000,
  })
  const highlights = useQuery({
    queryKey: ['guide-highlights'],
    queryFn: () => api.get<Record<string, unknown>>('/api/guide/highlights.json?limit=6'),
    staleTime: 60_000,
  })
  const capsules = useQuery({
    queryKey: ['guide-capsules'],
    queryFn: () => api.get<Record<string, unknown>>('/api/guide/capsules.json?limit=6'),
    staleTime: 60_000,
  })
  const epgStore = useQuery({
    queryKey: ['epg-store'],
    queryFn: () => api.get<Record<string, unknown>>('/api/guide/epg-store.json'),
    staleTime: 60_000,
  })

  const guideRefresh = useMutation({
    mutationFn: () => api.post('/api/ops/actions/guide-refresh'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['guide-health'] })
      qc.invalidateQueries({ queryKey: ['guide-doctor'] })
      notifications.show({ message: 'Guide refresh triggered', color: 'teal' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const h = health.data
  const guidePercent = Number(h?.guide_percent ?? 0)

  const doctorIssues = Array.isArray(doctor.data?.issues) ? doctor.data!.issues as string[] : []
  const highlightList = Array.isArray(highlights.data) ? highlights.data as Record<string,unknown>[] :
    Array.isArray((highlights.data as Record<string,unknown>|null)?.items) ? (highlights.data as Record<string,unknown>)!.items as Record<string,unknown>[] : []
  const capsuleList = Array.isArray(capsules.data) ? capsules.data as Record<string,unknown>[] :
    Array.isArray((capsules.data as Record<string,unknown>|null)?.items) ? (capsules.data as Record<string,unknown>)!.items as Record<string,unknown>[] : []
  const es = epgStore.data

  return (
    <ScrollArea>
      <Stack gap="md">
        <Group justify="flex-end">
          <Button size="xs" color="teal" leftSection={<IconRefresh size={14} />}
            loading={guideRefresh.isPending}
            onClick={() => { if (confirm('Run guide refresh now?')) guideRefresh.mutate() }}>
            Run Guide Refresh
          </Button>
        </Group>

        {/* Guide Health */}
        <Paper withBorder p="md">
          <Text fw={600} mb="sm">Guide Health</Text>
          {health.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : health.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Guide health unavailable.</Alert>
            : (
              <Stack gap="sm">
                <Box>
                  <Group justify="space-between" mb={4}>
                    <Text size="sm">Guide coverage</Text>
                    <Text size="sm" fw={500}>{guidePercent.toFixed(1)}%</Text>
                  </Group>
                  <Progress
                    value={guidePercent}
                    color={guidePercent >= 80 ? 'teal' : guidePercent >= 50 ? 'orange' : 'red'}
                    size="sm"
                  />
                </Box>
                <Table withRowBorders={false} fz="sm">
                  <Table.Tbody>
                    <Table.Tr><Table.Td c="dimmed" w={180}>Linked channels</Table.Td><Table.Td>{String(h?.linked_count ?? '—')}</Table.Td></Table.Tr>
                    <Table.Tr><Table.Td c="dimmed">Placeholder</Table.Td><Table.Td>{String(h?.placeholder_count ?? '—')}</Table.Td></Table.Tr>
                    <Table.Tr><Table.Td c="dimmed">Stale</Table.Td><Table.Td>{String(h?.stale_count ?? '—')}</Table.Td></Table.Tr>
                    <Table.Tr><Table.Td c="dimmed">Total channels</Table.Td><Table.Td>{String(h?.total_channels ?? '—')}</Table.Td></Table.Tr>
                    {!!h?.freshness && <Table.Tr><Table.Td c="dimmed">Freshness</Table.Td><Table.Td>{String(h.freshness)}</Table.Td></Table.Tr>}
                  </Table.Tbody>
                </Table>
              </Stack>
            )}
        </Paper>

        {/* Doctor */}
        <Paper withBorder p="md">
          <Text fw={600} mb="sm">Guide Doctor</Text>
          {doctor.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : doctor.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Doctor unavailable.</Alert>
            : (
              <Stack gap="xs">
                {!!doctor.data?.summary && <Text size="sm">{String(doctor.data.summary)}</Text>}
                {doctorIssues.length === 0
                  ? <Alert color="teal" p="xs"><Text size="sm">No issues detected.</Text></Alert>
                  : doctorIssues.map((issue, i) => (
                    <Alert key={i} icon={<IconAlertCircle size={14} />} color="orange" p="xs">
                      <Text size="xs">{issue}</Text>
                    </Alert>
                  ))}
              </Stack>
            )}
        </Paper>

        {/* Highlights */}
        <Paper withBorder p="md">
          <Text fw={600} mb="sm">Current / Starting Soon</Text>
          {highlights.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : highlights.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Highlights unavailable.</Alert>
            : highlightList.length === 0 ? <Text size="sm" c="dimmed">No highlights.</Text>
            : (
              <Table withRowBorders={false} fz="sm" striped>
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>Channel</Table.Th>
                    <Table.Th>Title</Table.Th>
                    <Table.Th>Start</Table.Th>
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {highlightList.map((hl, i) => (
                    <Table.Tr key={i}>
                      <Table.Td>{String(hl.channel_name ?? hl.channel ?? '—')}</Table.Td>
                      <Table.Td>{String(hl.title ?? '—')}</Table.Td>
                      <Table.Td c="dimmed">{hl.start ? new Date(String(hl.start)).toLocaleTimeString() : '—'}</Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            )}
        </Paper>

        {/* Catch-up Capsules */}
        <Paper withBorder p="md">
          <Text fw={600} mb="sm">Catch-up Capsules</Text>
          {capsules.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : capsules.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">Capsules unavailable.</Alert>
            : capsuleList.length === 0 ? <Text size="sm" c="dimmed">No capsule candidates.</Text>
            : (
              <Table withRowBorders={false} fz="sm" striped>
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>Channel</Table.Th>
                    <Table.Th>Title</Table.Th>
                    <Table.Th>Start</Table.Th>
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {capsuleList.map((c, i) => (
                    <Table.Tr key={i}>
                      <Table.Td>{String(c.channel_name ?? c.channel ?? '—')}</Table.Td>
                      <Table.Td>{String(c.title ?? '—')}</Table.Td>
                      <Table.Td c="dimmed">{c.start ? new Date(String(c.start)).toLocaleTimeString() : '—'}</Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            )}
        </Paper>

        {/* EPG Store */}
        <Paper withBorder p="md">
          <Text fw={600} mb="sm">EPG Store</Text>
          {epgStore.isLoading ? <Text size="sm" c="dimmed">Loading…</Text>
            : epgStore.isError ? <Alert icon={<IconAlertCircle size={16} />} color="gray">EPG store unavailable.</Alert>
            : (
              <Table withRowBorders={false} fz="sm">
                <Table.Tbody>
                  <Table.Tr><Table.Td c="dimmed" w={180}>Entry count</Table.Td><Table.Td>{String(es?.entry_count ?? '—')}</Table.Td></Table.Tr>
                  {!!es?.horizon && <Table.Tr><Table.Td c="dimmed">Horizon</Table.Td><Table.Td>{String(es.horizon)}</Table.Td></Table.Tr>}
                  {!!es?.horizon_hours && <Table.Tr><Table.Td c="dimmed">Horizon (hours)</Table.Td><Table.Td>{String(es.horizon_hours)}</Table.Td></Table.Tr>}
                </Table.Tbody>
              </Table>
            )}
        </Paper>
      </Stack>
    </ScrollArea>
  )
}

// ──────────────────────────────────────────────────────────────────
// Page
// ──────────────────────────────────────────────────────────────────
export function M3UEPGManager() {
  return (
    <Stack gap="md" h="100%" style={{ overflow: 'hidden' }}>
      <Group justify="space-between">
        <Text size="lg" fw={600}>M3U &amp; EPG Manager</Text>
      </Group>

      <Paper withBorder p="md" style={{ flex: 1, overflow: 'hidden' }}>
        <Tabs defaultValue="m3u" keepMounted={false}>
          <Tabs.List>
            <Tabs.Tab value="m3u">M3U Accounts</Tabs.Tab>
            <Tabs.Tab value="epg">EPG Sources</Tabs.Tab>
            <Tabs.Tab value="health" leftSection={<IconHeartbeat size={14} />}>Health</Tabs.Tab>
          </Tabs.List>
          <Divider />
          <Box pt="md">
            <Tabs.Panel value="m3u"><M3UTab /></Tabs.Panel>
            <Tabs.Panel value="epg"><EPGTab /></Tabs.Panel>
            <Tabs.Panel value="health"><GuideHealthTab /></Tabs.Panel>
          </Box>
        </Tabs>
      </Paper>
    </Stack>
  )
}
