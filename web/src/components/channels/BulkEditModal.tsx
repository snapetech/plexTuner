import {
  Modal, Stack, Select, Button, Group, Text, Checkbox,
} from '@mantine/core'
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { ChannelGroup } from '../../api/channels'
import { streamProfilesApi } from '../../api/settings'

interface BulkUpdate {
  group_id?: number
  stream_profile?: string
  user_level?: string
  mature?: boolean
  clear_epg?: boolean
}

interface Props {
  count: number
  groups: ChannelGroup[]
  opened: boolean
  onClose: () => void
  onApply: (update: BulkUpdate) => Promise<void>
}

const BUILTIN_PROFILES = [
  { value: '', label: '(unchanged)' },
  { value: 'proxy', label: 'Proxy (passthrough)' },
  { value: 'redirect', label: 'Redirect (HTTP 302)' },
]

export function BulkEditModal({ count, groups, opened, onClose, onApply }: Props) {
  const [groupId, setGroupId] = useState<string | null>(null)
  const [streamProfile, setStreamProfile] = useState<string | null>(null)
  const [userLevel, setUserLevel] = useState<string | null>(null)
  const [mature, setMature] = useState<boolean | undefined>(undefined)
  const [clearEpg, setClearEpg] = useState(false)
  const [applying, setApplying] = useState(false)

  const { data: streamProfiles = [] } = useQuery({
    queryKey: ['stream-profiles'],
    queryFn: () => streamProfilesApi.list(),
    staleTime: 60_000,
  })

  const profileOptions = [
    ...BUILTIN_PROFILES,
    ...streamProfiles.map(p => ({ value: p.name, label: `${p.name} (${p.type})` })),
  ]

  async function handleApply() {
    const update: BulkUpdate = {}
    if (groupId) update.group_id = Number(groupId)
    if (streamProfile) update.stream_profile = streamProfile
    if (userLevel) update.user_level = userLevel
    if (mature !== undefined) update.mature = mature
    if (clearEpg) update.clear_epg = true
    setApplying(true)
    try { await onApply(update) } finally { setApplying(false) }
  }

  return (
    <Modal
      opened={opened}
      onClose={onClose}
      title={`Bulk Edit — ${count} channel${count !== 1 ? 's' : ''}`}
      size="md"
    >
      <Stack gap="sm">
        <Text size="sm" c="dimmed">Only filled fields will be applied.</Text>
        <Select
          label="Set Group"
          placeholder="(unchanged)"
          data={groups.map(g => ({ value: String(g.id), label: g.name }))}
          value={groupId}
          onChange={setGroupId}
          clearable
        />
        <Select
          label="Set Stream Profile"
          data={profileOptions}
          value={streamProfile}
          onChange={setStreamProfile}
          clearable
        />
        <Select
          label="Set User Level"
          placeholder="(unchanged)"
          data={[
            { value: 'all', label: 'All users' },
            { value: 'standard', label: 'Standard' },
            { value: 'admin', label: 'Admin only' },
          ]}
          value={userLevel}
          onChange={setUserLevel}
          clearable
        />
        <Select
          label="Set Mature Content"
          placeholder="(unchanged)"
          data={[
            { value: 'true', label: 'Yes — mature' },
            { value: 'false', label: 'No — not mature' },
          ]}
          value={mature === undefined ? null : String(mature)}
          onChange={v => setMature(v === null ? undefined : v === 'true')}
          clearable
        />
        <Checkbox
          label="Clear EPG assignment"
          checked={clearEpg}
          onChange={e => setClearEpg(e.currentTarget.checked)}
        />
        <Group justify="flex-end" mt="sm">
          <Button variant="default" onClick={onClose}>Cancel</Button>
          <Button onClick={handleApply} loading={applying} color="teal">Apply</Button>
        </Group>
      </Stack>
    </Modal>
  )
}
