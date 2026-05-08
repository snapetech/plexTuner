import {
  Modal, Stack, TextInput, Select, Switch, Button, Group, Text, Divider,
} from '@mantine/core'
import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { Channel, ChannelGroup } from '../../api/channels'
import { streamProfilesApi } from '../../api/settings'

interface Props {
  channel: Partial<Channel> | null
  groups: ChannelGroup[]
  opened: boolean
  onClose: () => void
  onSave: (ch: Partial<Channel>) => Promise<void>
}

const BUILTIN_PROFILES = [
  { value: '', label: 'Default (inherit)' },
  { value: 'proxy', label: 'Proxy (passthrough)' },
  { value: 'redirect', label: 'Redirect (HTTP 302)' },
]

export function ChannelEditModal({ channel, groups, opened, onClose, onSave }: Props) {
  const [form, setForm] = useState<Partial<Channel>>({})
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setForm(channel ?? {})
  }, [channel, opened])

  const { data: streamProfiles = [] } = useQuery({
    queryKey: ['stream-profiles'],
    queryFn: () => streamProfilesApi.list(),
    staleTime: 60_000,
  })

  const profileOptions = [
    ...BUILTIN_PROFILES,
    ...streamProfiles.map(p => ({ value: p.name, label: `${p.name} (${p.type})` })),
  ]

  const set = <K extends keyof Channel>(k: K, v: Channel[K]) =>
    setForm(f => ({ ...f, [k]: v }))

  async function handleSave() {
    setSaving(true)
    try { await onSave(form) } finally { setSaving(false) }
  }

  const groupOptions = groups.map(g => ({ value: String(g.id), label: g.name }))

  return (
    <Modal
      opened={opened}
      onClose={onClose}
      title={form.id ? `Edit — ${form.name}` : 'New Channel'}
      size="lg"
      overlayProps={{ backgroundOpacity: 0.5 }}
    >
      <Stack gap="sm">
        <TextInput
          label="Channel Name"
          value={form.name ?? ''}
          onChange={e => set('name', e.currentTarget.value)}
          required
        />
        <Group grow>
          <TextInput
            label="Channel #"
            value={form.channel_number ?? ''}
            onChange={e => set('channel_number', e.currentTarget.value)}
          />
          <Select
            label="Group"
            placeholder="None"
            data={groupOptions}
            value={form.group_id != null ? String(form.group_id) : null}
            onChange={v => set('group_id', v ? Number(v) : undefined)}
            clearable
          />
        </Group>

        <Divider label="Guide" labelPosition="left" />
        <Group grow>
          <TextInput
            label="TVG-ID"
            value={form.tvg_id ?? ''}
            onChange={e => set('tvg_id', e.currentTarget.value)}
            placeholder="leave blank for auto-match"
          />
          <TextInput
            label="Gracenote Station ID"
            value={form.gracenote_id ?? ''}
            onChange={e => set('gracenote_id', e.currentTarget.value)}
          />
        </Group>

        <Divider label="Stream" labelPosition="left" />
        <Select
          label="Stream Profile"
          description="Controls how streams are served. Overrides the global default."
          data={profileOptions}
          value={form.stream_profile ?? ''}
          onChange={v => set('stream_profile', v ?? '')}
        />

        <Divider label="Access" labelPosition="left" />
        <Select
          label="User Level"
          data={[
            { value: 'all', label: 'All users' },
            { value: 'standard', label: 'Standard' },
            { value: 'admin', label: 'Admin only' },
          ]}
          value={form.user_level ?? 'all'}
          onChange={v => set('user_level', v ?? 'all')}
        />
        <Group>
          <Switch
            label="Mature Content"
            checked={form.mature ?? false}
            onChange={e => set('mature', e.currentTarget.checked)}
          />
          <Switch
            label="Enabled"
            checked={form.enabled ?? true}
            onChange={e => set('enabled', e.currentTarget.checked)}
          />
        </Group>

        {form.id && (
          <Text size="xs" c="dimmed">
            To assign a logo, go to Logo Manager and then set logo_id via the API, or use the Bulk Edit to set it across channels.
          </Text>
        )}

        <Group justify="flex-end" mt="sm">
          <Button variant="default" onClick={onClose}>Cancel</Button>
          <Button onClick={handleSave} loading={saving}>Save</Button>
        </Group>
      </Stack>
    </Modal>
  )
}
