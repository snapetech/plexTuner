import {
  Stack, Group, Text, Paper, Table, Badge, ActionIcon, Tooltip,
  Button, Modal, TextInput, Select, Switch, NumberInput,
  MultiSelect, Divider, Alert, ScrollArea, PasswordInput, Tabs,
} from '@mantine/core'
import {
  IconPlus, IconTrash, IconEdit, IconAlertCircle, IconUser,
  IconShield, IconUsers,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { usersApi, type User, type UserInput } from '../api/users'
import { profilesApi } from '../api/channels'

const ROLE_COLOR: Record<string, string> = {
  admin: 'red', standard: 'blue', streamer: 'teal',
}

// ── User modal ────────────────────────────────────────────────────────────
function UserModal({
  opened, onClose, initial,
}: {
  opened: boolean
  onClose: () => void
  initial: User | null
}) {
  const qc = useQueryClient()
  const isEdit = !!initial

  const { data: profilesData } = useQuery({
    queryKey: ['profiles'],
    queryFn: () => profilesApi.list(),
  })
  const profiles = profilesData ?? []

  const [username, setUsername] = useState(initial?.username ?? '')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<'admin' | 'standard' | 'streamer'>(initial?.role ?? 'standard')
  const [xcPassword, setXcPassword] = useState(initial?.xc_password ?? '')
  const [hideMature, setHideMature] = useState(initial?.hide_mature ?? false)
  const [streamLimit, setStreamLimit] = useState<number>(initial?.stream_limit ?? 0)
  const [epgBack, setEpgBack] = useState<number>(initial?.epg_days_back ?? 0)
  const [epgFwd, setEpgFwd] = useState<number>(initial?.epg_days_fwd ?? 7)
  const [profileIds, setProfileIds] = useState<string[]>(
    (initial?.profile_ids ?? []).map(String)
  )

  function reset() {
    setUsername(''); setPassword(''); setRole('standard'); setXcPassword('')
    setHideMature(false); setStreamLimit(0); setEpgBack(0); setEpgFwd(7); setProfileIds([])
  }

  const save = useMutation({
    mutationFn: () => {
      const data: UserInput = {
        username, password: password || undefined, role,
        xc_password: xcPassword, hide_mature: hideMature,
        stream_limit: streamLimit, epg_days_back: epgBack, epg_days_fwd: epgFwd,
        profile_ids: profileIds.map(Number),
      }
      return isEdit ? usersApi.update(initial!.id, data) : usersApi.create(data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] })
      notifications.show({ message: isEdit ? 'User updated' : 'User created', color: 'teal' })
      reset(); onClose()
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Modal
      opened={opened}
      onClose={() => { reset(); onClose() }}
      title={isEdit ? `Edit — ${initial?.username}` : 'New User'}
      size="md"
    >
      <Tabs defaultValue="account">
        <Tabs.List>
          <Tabs.Tab value="account">Account</Tabs.Tab>
          <Tabs.Tab value="access">Access</Tabs.Tab>
          <Tabs.Tab value="epg">EPG &amp; Prefs</Tabs.Tab>
        </Tabs.List>
        <Tabs.Panel value="account" pt="sm">
          <Stack gap="sm">
            <TextInput label="Username" value={username}
              onChange={e => setUsername(e.currentTarget.value)} required />
            <PasswordInput
              label={isEdit ? 'New password (leave blank to keep)' : 'Password'}
              value={password}
              onChange={e => setPassword(e.currentTarget.value)}
              required={!isEdit}
            />
            <Select
              label="Role"
              data={[
                { value: 'admin', label: 'Admin' },
                { value: 'standard', label: 'Standard' },
                { value: 'streamer', label: 'Streamer' },
              ]}
              value={role}
              onChange={v => setRole((v ?? 'standard') as typeof role)}
            />
            <TextInput
              label="Xtream Codes password"
              value={xcPassword}
              onChange={e => setXcPassword(e.currentTarget.value)}
              placeholder="For XC API compatibility"
            />
          </Stack>
        </Tabs.Panel>
        <Tabs.Panel value="access" pt="sm">
          <Stack gap="sm">
            <MultiSelect
              label="Allowed channel profiles (empty = all)"
              data={profiles.map(p => ({ value: String(p.id), label: p.name }))}
              value={profileIds}
              onChange={setProfileIds}
              placeholder="All profiles"
              clearable
            />
            <NumberInput
              label="Max concurrent streams (0 = unlimited)"
              value={streamLimit}
              onChange={v => setStreamLimit(Number(v))}
              min={0}
            />
            <Switch
              label="Hide mature content"
              checked={hideMature}
              onChange={e => setHideMature(e.currentTarget.checked)}
            />
          </Stack>
        </Tabs.Panel>
        <Tabs.Panel value="epg" pt="sm">
          <Stack gap="sm">
            <NumberInput
              label="EPG days back (catch-up)"
              value={epgBack}
              onChange={v => setEpgBack(Number(v))}
              min={0}
            />
            <NumberInput
              label="EPG days forward"
              value={epgFwd}
              onChange={v => setEpgFwd(Number(v))}
              min={1}
            />
          </Stack>
        </Tabs.Panel>
      </Tabs>

      <Divider my="sm" />
      <Group justify="flex-end">
        <Button variant="default" onClick={() => { reset(); onClose() }}>Cancel</Button>
        <Button color="teal" loading={save.isPending} onClick={() => save.mutate()}>
          {isEdit ? 'Save' : 'Create'}
        </Button>
      </Group>
    </Modal>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────
export function Users() {
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<User | null>(null)

  const { data: users = [], isLoading } = useQuery({
    queryKey: ['users'],
    queryFn: () => usersApi.list(),
  })

  const del = useMutation({
    mutationFn: (id: number) => usersApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const roleIcon = (role: string) => {
    if (role === 'admin') return <IconShield size={14} />
    if (role === 'streamer') return <IconUsers size={14} />
    return <IconUser size={14} />
  }

  return (
    <Stack gap="md" h="100%" style={{ overflow: 'hidden' }}>
      <Group justify="space-between">
        <Text size="lg" fw={600}>Users</Text>
        <Button size="xs" leftSection={<IconPlus size={14} />} color="teal"
          onClick={() => { setEditTarget(null); setModalOpen(true) }}>
          New User
        </Button>
      </Group>

      <Paper withBorder p="md" style={{ flex: 1, overflow: 'hidden' }}>
        {isLoading ? (
          <Text size="sm" c="dimmed">Loading…</Text>
        ) : users.length === 0 ? (
          <Alert icon={<IconAlertCircle size={16} />} color="gray">
            No users yet. Create an admin account to enable authentication.
          </Alert>
        ) : (
          <ScrollArea>
            <Table striped highlightOnHover withRowBorders={false} fz="sm">
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>Username</Table.Th>
                  <Table.Th>Role</Table.Th>
                  <Table.Th>Stream limit</Table.Th>
                  <Table.Th>Profiles</Table.Th>
                  <Table.Th>Created</Table.Th>
                  <Table.Th style={{ width: 80 }} />
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {users.map(u => (
                  <Table.Tr key={u.id}>
                    <Table.Td>
                      <Group gap="xs">
                        {roleIcon(u.role)}
                        <Text size="sm">{u.username}</Text>
                      </Group>
                    </Table.Td>
                    <Table.Td>
                      <Badge size="xs" color={ROLE_COLOR[u.role] ?? 'gray'}>{u.role}</Badge>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">
                        {u.stream_limit === 0 ? 'Unlimited' : u.stream_limit}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">
                        {u.profile_ids.length === 0 ? 'All' : u.profile_ids.length}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">
                        {new Date(u.created_at).toLocaleDateString()}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Group gap={4} wrap="nowrap">
                        <Tooltip label="Edit">
                          <ActionIcon size="xs" variant="subtle" color="yellow"
                            onClick={() => { setEditTarget(u); setModalOpen(true) }}>
                            <IconEdit size={14} />
                          </ActionIcon>
                        </Tooltip>
                        <Tooltip label="Delete">
                          <ActionIcon size="xs" variant="subtle" color="red"
                            onClick={() => {
                              if (confirm(`Delete user "${u.username}"?`)) del.mutate(u.id)
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
      </Paper>

      <UserModal
        opened={modalOpen}
        onClose={() => { setModalOpen(false); setEditTarget(null) }}
        initial={editTarget}
      />
    </Stack>
  )
}
