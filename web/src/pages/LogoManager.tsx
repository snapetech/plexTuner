import {
  Stack, Group, Text, Paper, SimpleGrid, Image, ActionIcon, Tooltip,
  Button, Alert, FileButton, Loader, Badge, Box,
} from '@mantine/core'
import { IconUpload, IconTrash, IconAlertCircle, IconPhoto } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useRef, useState, useCallback } from 'react'

import { logosApi, type Logo } from '../api/logos'

function formatBytes(n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

export function LogoManager() {
  const qc = useQueryClient()
  const [dragging, setDragging] = useState(false)
  const dragCounter = useRef(0)

  const { data: logos = [], isLoading } = useQuery({
    queryKey: ['logos'],
    queryFn: () => logosApi.list(),
  })

  const upload = useMutation({
    mutationFn: (file: File) => logosApi.upload(file),
    onSuccess: (logo) => {
      qc.invalidateQueries({ queryKey: ['logos'] })
      notifications.show({ message: `Uploaded ${logo.filename}`, color: 'teal' })
    },
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const del = useMutation({
    mutationFn: (id: number) => logosApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['logos'] }),
    onError: (e: Error) => notifications.show({ message: e.message, color: 'red' }),
  })

  const handleFiles = useCallback((files: File[] | FileList | null) => {
    if (!files) return
    const arr = Array.from(files)
    const valid = arr.filter(f => f.type.startsWith('image/'))
    if (valid.length !== arr.length) {
      notifications.show({ message: 'Only image files are accepted', color: 'orange' })
    }
    valid.forEach(f => upload.mutate(f))
  }, [upload])

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    dragCounter.current = 0
    setDragging(false)
    handleFiles(e.dataTransfer.files)
  }, [handleFiles])

  const onDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    dragCounter.current++
    setDragging(true)
  }, [])

  const onDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    dragCounter.current--
    if (dragCounter.current === 0) setDragging(false)
  }, [])

  return (
    <Stack gap="md" h="100%" style={{ overflow: 'hidden' }}>
      <Group justify="space-between">
        <Text size="lg" fw={600}>Logo Manager</Text>
        <Group gap="xs">
          {upload.isPending && <Loader size="xs" color="teal" />}
          <FileButton onChange={f => f && handleFiles([f])} accept="image/*">
            {props => (
              <Button size="xs" leftSection={<IconUpload size={14} />} color="teal" {...props}>
                Upload
              </Button>
            )}
          </FileButton>
        </Group>
      </Group>

      <Box
        onDrop={onDrop}
        onDragEnter={onDragEnter}
        onDragLeave={onDragLeave}
        onDragOver={e => e.preventDefault()}
        style={{
          flex: 1, overflow: 'auto',
          border: `2px dashed ${dragging ? 'var(--mantine-color-teal-6)' : 'transparent'}`,
          borderRadius: 'var(--mantine-radius-md)',
          transition: 'border-color 0.15s',
        }}
      >
        {isLoading ? (
          <Text size="sm" c="dimmed">Loading…</Text>
        ) : logos.length === 0 ? (
          <Paper withBorder p="xl" ta="center">
            <IconPhoto size={48} style={{ opacity: 0.3 }} />
            <Text mt="sm" c="dimmed" size="sm">
              No logos yet. Upload images or drag &amp; drop files here.
            </Text>
          </Paper>
        ) : (
          <Stack gap="sm">
            <Alert icon={<IconAlertCircle size={16} />} color="gray" variant="light">
              Drag &amp; drop image files anywhere on this page to upload. Max 2 MB per file.
            </Alert>
            <SimpleGrid cols={{ base: 3, sm: 4, md: 6, lg: 8 }} spacing="sm">
              {logos.map(logo => (
                <LogoCard
                  key={logo.id}
                  logo={logo}
                  onDelete={() => {
                    if (confirm(`Delete logo "${logo.filename}"?`)) del.mutate(logo.id)
                  }}
                />
              ))}
            </SimpleGrid>
          </Stack>
        )}
      </Box>
    </Stack>
  )
}

function LogoCard({ logo, onDelete }: { logo: Logo; onDelete: () => void }) {
  const src = logo.url ?? `/api/v2/logos/${logo.id}/image`
  return (
    <Paper withBorder p="xs" style={{ position: 'relative' }}>
      <Box style={{ aspectRatio: '1', overflow: 'hidden', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Image
          src={src}
          alt={logo.filename}
          fit="contain"
          style={{ maxHeight: 80, maxWidth: '100%' }}
          fallbackSrc="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='80' height='80'%3E%3C/svg%3E"
        />
      </Box>
      <Text size="xs" c="dimmed" truncate mt={4} title={logo.filename}>
        {logo.filename}
      </Text>
      <Text size="xs" c="dimmed">{formatBytes(logo.size_bytes)}</Text>
      <Badge
        size="xs"
        variant="dot"
        color="gray"
        style={{ position: 'absolute', top: 4, right: 24 }}
      >
        {logo.content_type.replace('image/', '')}
      </Badge>
      <Tooltip label="Delete">
        <ActionIcon
          size="xs"
          variant="subtle"
          color="red"
          style={{ position: 'absolute', top: 4, right: 4 }}
          onClick={onDelete}
        >
          <IconTrash size={12} />
        </ActionIcon>
      </Tooltip>
    </Paper>
  )
}
