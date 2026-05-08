import {
  Stack, Group, Text, Paper, Table, Badge, ActionIcon, Tooltip,
  Button, Modal, TextInput, Switch, Textarea, Alert, ScrollArea,
  Divider,
} from '@mantine/core'
import {
  IconPlus, IconTrash, IconEdit, IconAlertCircle, IconPlugConnected,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { pluginsApi, type Plugin, type PluginInput } from '../api/plugins'

// ── Plugin modal ──────────────────────────────────────────────────────────
function PluginModal({
  opened, onClose, initial,
}: {
  opened: boolean
  onClose: () => void
  initial: Plugin | null
}) {
  const qc = useQueryClient()
  const isEdit = !!initial

  const [name, setName] = useState(initial?.name ?? '')
  const [version, setVersion] = useState(initial?.version ?? '')
  const [description, setDescription] = useState(initial?.description ?? '')
  const [path, setPath] = useState(initial?.path ?? '')
  const [manifest, setManifest] = useState(initial?.manifest ?? '')
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)

  function reset() {
    setName(''); setVersion(''); setDescription(''); setPath('')
    setManifest(''); setEnabled(true)
  }

  const save = useMutation({
    mutationFn: () => {
      const data: PluginInput = {
        name, version: version || undefined,
        description: description || undefined,
        path, manifest: manifest || undefined,
        enabled,
      }
      return isEdit ? pluginsApi.update(initial!.id, data) : pluginsApi.create(data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['plugins'] })
      notifications.show({ message: isEdit ? 'Plugin updated' : 'Plugin registered', color: 'teal' })
      reset(); onClose()
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Modal
      opened={opened}
      onClose={() => { reset(); onClose() }}
      title={isEdit ? `Edit — ${initial?.name}` : 'Register Plugin'}
      size="md"
    >
      <Stack gap="sm">
        <TextInput label="Name" value={name} onChange={e => setName(e.currentTarget.value)} required />
        <TextInput label="Version" value={version} onChange={e => setVersion(e.currentTarget.value)} placeholder="1.0.0" />
        <TextInput label="Description" value={description} onChange={e => setDescription(e.currentTarget.value)} />
        <TextInput label="Path / entry point" value={path} onChange={e => setPath(e.currentTarget.value)} required placeholder="/opt/plugins/my-plugin.so" />
        <Textarea
          label="Manifest JSON"
          value={manifest}
          onChange={e => setManifest(e.currentTarget.value)}
          placeholder='{"capabilities": []}'
          autosize
          minRows={3}
          maxRows={8}
          styles={{ input: { fontFamily: 'monospace', fontSize: 12 } }}
        />
        <Switch label="Enabled" checked={enabled} onChange={e => setEnabled(e.currentTarget.checked)} />
      </Stack>
      <Divider my="sm" />
      <Group justify="flex-end">
        <Button variant="default" onClick={() => { reset(); onClose() }}>Cancel</Button>
        <Button color="teal" loading={save.isPending} onClick={() => save.mutate()}>
          {isEdit ? 'Save' : 'Register'}
        </Button>
      </Group>
    </Modal>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────
export function Plugins() {
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<Plugin | null>(null)

  const { data: plugins = [], isLoading } = useQuery({
    queryKey: ['plugins'],
    queryFn: () => pluginsApi.list(),
  })

  const toggle = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      enabled ? pluginsApi.enable(id) : pluginsApi.disable(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['plugins'] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const del = useMutation({
    mutationFn: (id: number) => pluginsApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['plugins'] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  return (
    <Stack gap="md" h="100%" style={{ overflow: 'hidden' }}>
      <Group justify="space-between">
        <Text size="lg" fw={600}>Plugins</Text>
        <Button size="xs" leftSection={<IconPlus size={14} />} color="teal"
          onClick={() => { setEditTarget(null); setModalOpen(true) }}>
          Register Plugin
        </Button>
      </Group>

      <Paper withBorder p="md" style={{ flex: 1, overflow: 'hidden' }}>
        {isLoading ? (
          <Text size="sm" c="dimmed">Loading…</Text>
        ) : plugins.length === 0 ? (
          <Alert icon={<IconAlertCircle size={16} />} color="gray">
            No plugins registered.{' '}
            <Text span size="sm">
              Register a plugin by providing its path and manifest.
            </Text>
          </Alert>
        ) : (
          <ScrollArea>
            <Table striped highlightOnHover withRowBorders={false} fz="sm">
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>Name</Table.Th>
                  <Table.Th>Version</Table.Th>
                  <Table.Th>Path</Table.Th>
                  <Table.Th>Status</Table.Th>
                  <Table.Th>Registered</Table.Th>
                  <Table.Th style={{ width: 90 }} />
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {plugins.map(p => (
                  <Table.Tr key={p.id}>
                    <Table.Td>
                      <Group gap="xs">
                        <IconPlugConnected size={14} style={{ opacity: 0.6 }} />
                        <Text size="sm" fw={500}>{p.name}</Text>
                      </Group>
                      {p.description && (
                        <Text size="xs" c="dimmed">{p.description}</Text>
                      )}
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">{p.version ?? '—'}</Text>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed" style={{ fontFamily: 'monospace', maxWidth: 240, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {p.path}
                      </Text>
                    </Table.Td>
                    <Table.Td>
                      <Badge
                        size="xs"
                        color={p.enabled ? 'teal' : 'gray'}
                        style={{ cursor: 'pointer' }}
                        onClick={() => toggle.mutate({ id: p.id, enabled: !p.enabled })}
                      >
                        {p.enabled ? 'enabled' : 'disabled'}
                      </Badge>
                    </Table.Td>
                    <Table.Td>
                      <Text size="xs" c="dimmed">
                        {new Date(p.created_at).toLocaleDateString()}
                      </Text>
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
                              if (confirm(`Delete plugin "${p.name}"?`)) del.mutate(p.id)
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

      <PluginModal
        opened={modalOpen}
        onClose={() => { setModalOpen(false); setEditTarget(null) }}
        initial={editTarget}
      />
    </Stack>
  )
}
