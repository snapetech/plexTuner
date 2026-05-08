import {
  Stack, Group, Text, Paper, Tabs, TextInput, Select, SimpleGrid,
  Card, Image, Badge, ActionIcon, Tooltip, Alert, Loader, Center,
  Box, Anchor,
} from '@mantine/core'
import {
  IconSearch, IconAlertCircle, IconPlayerPlay, IconMovie, IconDeviceTv,
} from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { useState, useMemo } from 'react'

import { vodsApi, type VodStream, type SeriesStream } from '../api/vods'

// ── Movie card ─────────────────────────────────────────────────────────────
function MovieCard({ item }: { item: VodStream }) {
  const ext = item.container_extension ?? 'mp4'
  const playUrl = item.direct_source ?? ''
  const poster = item.stream_icon

  return (
    <Card withBorder padding="xs" style={{ position: 'relative', overflow: 'hidden' }}>
      <Card.Section>
        <Box style={{ aspectRatio: '2/3', background: 'var(--mantine-color-dark-6)', display: 'flex', alignItems: 'center', justifyContent: 'center', overflow: 'hidden' }}>
          {poster ? (
            <Image
              src={poster}
              alt={item.name}
              fit="cover"
              style={{ width: '100%', height: '100%' }}
              fallbackSrc={undefined}
            />
          ) : (
            <IconMovie size={40} style={{ opacity: 0.2 }} />
          )}
        </Box>
      </Card.Section>
      <Text size="xs" fw={500} mt={6} lineClamp={2} title={item.name}>
        {item.name}
      </Text>
      {item.category_name && (
        <Text size="xs" c="dimmed" truncate>{item.category_name}</Text>
      )}
      {playUrl && (
        <Tooltip label="Play in new tab">
          <ActionIcon
            size="sm"
            variant="filled"
            color="teal"
            style={{ position: 'absolute', top: 6, right: 6 }}
            component="a"
            href={playUrl}
            target="_blank"
            rel="noopener"
          >
            <IconPlayerPlay size={12} />
          </ActionIcon>
        </Tooltip>
      )}
      <Badge size="xs" variant="dot" color="gray" mt={4}>{ext}</Badge>
    </Card>
  )
}

// ── Series card ────────────────────────────────────────────────────────────
function SeriesCard({ item }: { item: SeriesStream }) {
  const poster = item.cover

  return (
    <Card withBorder padding="xs">
      <Card.Section>
        <Box style={{ aspectRatio: '2/3', background: 'var(--mantine-color-dark-6)', display: 'flex', alignItems: 'center', justifyContent: 'center', overflow: 'hidden' }}>
          {poster ? (
            <Image
              src={poster}
              alt={item.name}
              fit="cover"
              style={{ width: '100%', height: '100%' }}
              fallbackSrc={undefined}
            />
          ) : (
            <IconDeviceTv size={40} style={{ opacity: 0.2 }} />
          )}
        </Box>
      </Card.Section>
      <Text size="xs" fw={500} mt={6} lineClamp={2} title={item.name}>
        {item.name}
      </Text>
      {item.category_name && (
        <Text size="xs" c="dimmed" truncate>{item.category_name}</Text>
      )}
      {item.rating && (
        <Badge size="xs" variant="dot" color="yellow" mt={4}>{item.rating}</Badge>
      )}
    </Card>
  )
}

// ── Movies tab ─────────────────────────────────────────────────────────────
function MoviesTab() {
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<string | null>(null)

  const { data: movies, isLoading, error } = useQuery({
    queryKey: ['vods-movies'],
    queryFn: () => vodsApi.movies(),
    staleTime: 5 * 60_000,
    retry: false,
  })

  const categories = useMemo(() => {
    if (!movies) return []
    const seen = new Map<string, string>()
    for (const m of movies) {
      if (m.category_id && m.category_name) seen.set(m.category_id, m.category_name)
    }
    return Array.from(seen.entries())
      .sort((a, b) => a[1].localeCompare(b[1]))
      .map(([value, label]) => ({ value, label }))
  }, [movies])

  const filtered = useMemo(() => {
    if (!movies) return []
    let out = movies
    if (category) out = out.filter(m => m.category_id === category)
    if (search.trim()) {
      const q = search.toLowerCase()
      out = out.filter(m => m.name.toLowerCase().includes(q))
    }
    return out
  }, [movies, category, search])

  if (isLoading) {
    return <Center h={200}><Loader size="sm" color="teal" /></Center>
  }

  if (error) {
    return (
      <Alert icon={<IconAlertCircle size={16} />} color="red">
        {String(error instanceof Error ? error.message : error)}
        {' '}
        <Anchor size="sm" href="#" onClick={() => window.location.href = '/settings'}>
          Configure Xtream credentials in Settings → Provider
        </Anchor>
      </Alert>
    )
  }

  return (
    <Stack gap="sm">
      <Group gap="sm">
        <TextInput
          placeholder="Search movies…"
          leftSection={<IconSearch size={14} />}
          value={search}
          onChange={e => setSearch(e.currentTarget.value)}
          style={{ flex: 1 }}
        />
        <Select
          placeholder="All categories"
          data={categories}
          value={category}
          onChange={setCategory}
          clearable
          style={{ minWidth: 200 }}
        />
        <Text size="sm" c="dimmed">{filtered.length} titles</Text>
      </Group>

      {filtered.length === 0 ? (
        <Paper withBorder p="xl" ta="center">
          <Text c="dimmed" size="sm">No movies found.</Text>
        </Paper>
      ) : (
        <SimpleGrid cols={{ base: 3, sm: 4, md: 6, lg: 8, xl: 10 }} spacing="xs">
          {filtered.map(m => (
            <MovieCard key={String(m.stream_id)} item={m} />
          ))}
        </SimpleGrid>
      )}
    </Stack>
  )
}

// ── Series tab ─────────────────────────────────────────────────────────────
function SeriesTab() {
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<string | null>(null)

  const { data: series, isLoading, error } = useQuery({
    queryKey: ['vods-series'],
    queryFn: () => vodsApi.series(),
    staleTime: 5 * 60_000,
    retry: false,
  })

  const categories = useMemo(() => {
    if (!series) return []
    const seen = new Map<string, string>()
    for (const s of series) {
      if (s.category_id && s.category_name) seen.set(s.category_id, s.category_name)
    }
    return Array.from(seen.entries())
      .sort((a, b) => a[1].localeCompare(b[1]))
      .map(([value, label]) => ({ value, label }))
  }, [series])

  const filtered = useMemo(() => {
    if (!series) return []
    let out = series
    if (category) out = out.filter(s => s.category_id === category)
    if (search.trim()) {
      const q = search.toLowerCase()
      out = out.filter(s => s.name.toLowerCase().includes(q))
    }
    return out
  }, [series, category, search])

  if (isLoading) {
    return <Center h={200}><Loader size="sm" color="teal" /></Center>
  }

  if (error) {
    return (
      <Alert icon={<IconAlertCircle size={16} />} color="red">
        {String(error instanceof Error ? error.message : error)}
        {' '}
        <Anchor size="sm" href="#" onClick={e => { e.preventDefault(); window.location.href = '/settings' }}>
          Configure Xtream credentials in Settings → Provider
        </Anchor>
      </Alert>
    )
  }

  return (
    <Stack gap="sm">
      <Group gap="sm">
        <TextInput
          placeholder="Search series…"
          leftSection={<IconSearch size={14} />}
          value={search}
          onChange={e => setSearch(e.currentTarget.value)}
          style={{ flex: 1 }}
        />
        <Select
          placeholder="All categories"
          data={categories}
          value={category}
          onChange={setCategory}
          clearable
          style={{ minWidth: 200 }}
        />
        <Text size="sm" c="dimmed">{filtered.length} titles</Text>
      </Group>

      {filtered.length === 0 ? (
        <Paper withBorder p="xl" ta="center">
          <Text c="dimmed" size="sm">No series found.</Text>
        </Paper>
      ) : (
        <SimpleGrid cols={{ base: 3, sm: 4, md: 6, lg: 8, xl: 10 }} spacing="xs">
          {filtered.map(s => (
            <SeriesCard key={String(s.series_id)} item={s} />
          ))}
        </SimpleGrid>
      )}
    </Stack>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────
export function Vods() {
  return (
    <Stack gap="md" h="100%" style={{ overflow: 'hidden' }}>
      <Text size="lg" fw={600}>VODs</Text>

      <Paper withBorder p={0} style={{ flex: 1, overflow: 'hidden' }}>
        <Tabs defaultValue="movies" style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
          <Tabs.List px="md" pt="xs">
            <Tabs.Tab value="movies" leftSection={<IconMovie size={14} />}>Movies</Tabs.Tab>
            <Tabs.Tab value="series" leftSection={<IconDeviceTv size={14} />}>Series</Tabs.Tab>
          </Tabs.List>

          <Box style={{ flex: 1, overflow: 'auto', padding: 'var(--mantine-spacing-md)' }}>
            <Tabs.Panel value="movies">
              <MoviesTab />
            </Tabs.Panel>
            <Tabs.Panel value="series">
              <SeriesTab />
            </Tabs.Panel>
          </Box>
        </Tabs>
      </Paper>
    </Stack>
  )
}
