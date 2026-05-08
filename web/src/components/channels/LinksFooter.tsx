import {
  Paper, Group, Button, Text, Popover, Stack, Select, Switch, CopyButton, ActionIcon, Tooltip,
} from '@mantine/core'
import { IconCopy, IconCheck, IconDeviceTv, IconScreenShare, IconFileCode } from '@tabler/icons-react'
import { useState } from 'react'
import { boot } from '../../api/client'

interface Props {
  profileId?: number
}

interface LinkOpts {
  cached_logos: boolean
  direct: boolean
  tvg_source: 'tvg_id' | 'channel_name'
  days_fwd: string
  days_back: string
}

function buildURL(kind: 'hdhr' | 'm3u' | 'epg', profileId: number | undefined, opts: LinkOpts) {
  const b = boot()
  const base = `http://localhost:${b.port}`
  const params = new URLSearchParams()
  if (profileId) params.set('profile', String(profileId))
  if (kind === 'm3u') {
    if (opts.cached_logos) params.set('cached_logos', '1')
    if (opts.direct) params.set('direct', '1')
    params.set('tvg_source', opts.tvg_source)
  }
  if (kind === 'epg') {
    if (opts.cached_logos) params.set('cached_logos', '1')
    params.set('tvg_source', opts.tvg_source)
    if (opts.days_fwd !== '0') params.set('days_fwd', opts.days_fwd)
    if (opts.days_back !== '0') params.set('days_back', opts.days_back)
  }
  const qs = params.toString()
  const paths: Record<string, string> = {
    hdhr: '/api/discover.json',
    m3u: '/api/live.m3u',
    epg: '/api/guide.xml',
  }
  return base + paths[kind] + (qs ? '?' + qs : '')
}

function CopyLink({ url }: { url: string }) {
  return (
    <CopyButton value={url} timeout={2000}>
      {({ copied, copy }) => (
        <Tooltip label={copied ? 'Copied!' : 'Copy URL'}>
          <ActionIcon variant="subtle" color={copied ? 'teal' : 'gray'} onClick={copy} size="sm">
            {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
          </ActionIcon>
        </Tooltip>
      )}
    </CopyButton>
  )
}

export function LinksFooter({ profileId }: Props) {
  const [opts, setOpts] = useState<LinkOpts>({
    cached_logos: false,
    direct: false,
    tvg_source: 'tvg_id',
    days_fwd: '7',
    days_back: '0',
  })
  const setOpt = <K extends keyof LinkOpts>(k: K, v: LinkOpts[K]) =>
    setOpts(o => ({ ...o, [k]: v }))

  const hdhrURL = buildURL('hdhr', profileId, opts)
  const m3uURL  = buildURL('m3u', profileId, opts)
  const epgURL  = buildURL('epg', profileId, opts)

  return (
    <Paper withBorder p="sm" radius="sm">
      <Group gap="sm" wrap="nowrap" align="center">
        <Text size="xs" c="dimmed" fw={500}>Links</Text>

        {/* HDHR */}
        <Group gap={4}>
          <IconDeviceTv size={14} color="var(--mantine-color-lime-5)" />
          <Text size="xs" c="lime" fw={500}>HDHR</Text>
          <CopyLink url={hdhrURL} />
        </Group>

        {/* M3U */}
        <Popover withArrow shadow="md">
          <Popover.Target>
            <Button size="xs" variant="subtle" color="violet" leftSection={<IconScreenShare size={14} />}>
              M3U
            </Button>
          </Popover.Target>
          <Popover.Dropdown>
            <Stack gap="xs">
              <Switch label="Cached logos" checked={opts.cached_logos} onChange={e => setOpt('cached_logos', e.currentTarget.checked)} size="xs" />
              <Switch label="Direct stream URLs" checked={opts.direct} onChange={e => setOpt('direct', e.currentTarget.checked)} size="xs" />
              <Select
                label="TVG-ID source"
                size="xs"
                data={[{ value: 'tvg_id', label: 'TVG-ID' }, { value: 'channel_name', label: 'Channel name' }]}
                value={opts.tvg_source}
                onChange={v => setOpt('tvg_source', (v ?? 'tvg_id') as LinkOpts['tvg_source'])}
              />
              <Group gap={4} mt="xs">
                <CopyLink url={m3uURL} />
                <Text size="xs" c="dimmed" style={{ wordBreak: 'break-all' }}>{m3uURL}</Text>
              </Group>
            </Stack>
          </Popover.Dropdown>
        </Popover>

        {/* EPG */}
        <Popover withArrow shadow="md">
          <Popover.Target>
            <Button size="xs" variant="subtle" color="gray" leftSection={<IconFileCode size={14} />}>
              EPG
            </Button>
          </Popover.Target>
          <Popover.Dropdown>
            <Stack gap="xs">
              <Switch label="Cached logos" checked={opts.cached_logos} onChange={e => setOpt('cached_logos', e.currentTarget.checked)} size="xs" />
              <Select
                label="TVG-ID source"
                size="xs"
                data={[{ value: 'tvg_id', label: 'TVG-ID' }, { value: 'channel_name', label: 'Channel name' }]}
                value={opts.tvg_source}
                onChange={v => setOpt('tvg_source', (v ?? 'tvg_id') as LinkOpts['tvg_source'])}
              />
              <Select
                label="Days forward"
                size="xs"
                data={['0','1','3','7','14'].map(v => ({ value: v, label: v === '0' ? 'All' : v }))}
                value={opts.days_fwd}
                onChange={v => setOpt('days_fwd', v ?? '7')}
              />
              <Select
                label="Days back"
                size="xs"
                data={['0','1','3','7','14','30'].map(v => ({ value: v, label: v === '0' ? 'None' : v }))}
                value={opts.days_back}
                onChange={v => setOpt('days_back', v ?? '0')}
              />
              <Group gap={4} mt="xs">
                <CopyLink url={epgURL} />
                <Text size="xs" c="dimmed" style={{ wordBreak: 'break-all' }}>{epgURL}</Text>
              </Group>
            </Stack>
          </Popover.Dropdown>
        </Popover>
      </Group>
    </Paper>
  )
}
