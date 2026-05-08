import {
  Stack, Group, Text, Paper, Box, ActionIcon, Tooltip,
  ScrollArea, Badge, Button, Loader, Alert,
} from '@mantine/core'
import {
  IconChevronLeft, IconChevronRight, IconPlayerRecord, IconAlertCircle,
} from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'

import { guideApi, type GuideProgramme } from '../api/guide'

// ── Config ────────────────────────────────────────────────────────────────
const WINDOW_HOURS = 3
const CELL_MIN_WIDTH = 120 // px per 30-minute slot
const CHANNEL_COL_W = 160  // px

// ── Helpers ───────────────────────────────────────────────────────────────
function snapToHalfHour(d: Date): Date {
  const out = new Date(d)
  out.setMinutes(d.getMinutes() < 30 ? 0 : 30, 0, 0)
  return out
}

function addHours(d: Date, h: number): Date {
  return new Date(d.getTime() + h * 3600_000)
}

function progWidth(prog: GuideProgramme, from: Date, to: Date): { left: number; width: number } {
  const total = to.getTime() - from.getTime()
  const pStart = Math.max(prog.start_unix * 1000, from.getTime())
  const pStop  = Math.min(prog.stop_unix  * 1000, to.getTime())
  const left   = ((pStart - from.getTime()) / total) * 100
  const width  = ((pStop  - pStart)        / total) * 100
  return { left, width }
}

function formatTime(d: Date) {
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false })
}

// ── Slot header ───────────────────────────────────────────────────────────
function TimeHeader({ from, to }: { from: Date; to: Date }) {
  const slots: Date[] = []
  let cursor = new Date(from)
  while (cursor < to) {
    slots.push(new Date(cursor))
    cursor = new Date(cursor.getTime() + 30 * 60_000)
  }
  return (
    <Box style={{
      display: 'flex',
      marginLeft: CHANNEL_COL_W,
      borderBottom: '1px solid var(--mantine-color-dark-5)',
      background: 'var(--mantine-color-dark-8)',
      position: 'sticky', top: 0, zIndex: 2,
    }}>
      {slots.map((s, i) => (
        <Box key={i} style={{
          minWidth: CELL_MIN_WIDTH,
          flex: '0 0 auto',
          padding: '4px 8px',
          borderRight: '1px solid var(--mantine-color-dark-6)',
        }}>
          <Text size="xs" c="dimmed">{formatTime(s)}</Text>
        </Box>
      ))}
    </Box>
  )
}

// ── Now indicator ─────────────────────────────────────────────────────────
function NowBar({ from, to }: { from: Date; to: Date }) {
  const now = Date.now()
  if (now <= from.getTime() || now >= to.getTime()) return null
  const pct = ((now - from.getTime()) / (to.getTime() - from.getTime())) * 100
  return (
    <Box style={{
      position: 'absolute',
      left: `${pct}%`,
      top: 0, bottom: 0,
      width: 2,
      background: 'var(--mantine-color-teal-5)',
      zIndex: 1,
      pointerEvents: 'none',
    }} />
  )
}

// ── Programme cell ────────────────────────────────────────────────────────
function ProgCell({
  prog, from, to, onClick,
}: {
  prog: GuideProgramme
  from: Date
  to: Date
  onClick: (p: GuideProgramme) => void
}) {
  const { left, width } = progWidth(prog, from, to)
  const now = Date.now()
  const isNow = prog.start_unix * 1000 <= now && prog.stop_unix * 1000 > now
  const isPast = prog.stop_unix * 1000 < now

  return (
    <Box
      onClick={() => onClick(prog)}
      title={`${prog.title}${prog.sub_title ? ` — ${prog.sub_title}` : ''}`}
      style={{
        position: 'absolute',
        left: `${left}%`,
        width: `${width}%`,
        top: 2, bottom: 2,
        background: isNow
          ? 'var(--mantine-color-teal-9)'
          : isPast
            ? 'var(--mantine-color-dark-6)'
            : 'var(--mantine-color-dark-5)',
        border: '1px solid var(--mantine-color-dark-4)',
        borderRadius: 4,
        padding: '2px 6px',
        cursor: 'pointer',
        overflow: 'hidden',
        display: 'flex',
        alignItems: 'center',
        boxSizing: 'border-box',
        opacity: isPast ? 0.6 : 1,
        transition: 'background 0.1s',
      }}
    >
      <Text size="xs" lineClamp={1} style={{ userSelect: 'none' }}>
        {prog.title}
      </Text>
    </Box>
  )
}

// ── Channel row ───────────────────────────────────────────────────────────
function ChannelRow({
  channel, from, to, onProg,
}: {
  channel: { epg_id: string; name: string; icon?: string; programmes: GuideProgramme[] }
  from: Date
  to: Date
  onProg: (p: GuideProgramme) => void
}) {
  const totalSlots = (to.getTime() - from.getTime()) / (30 * 60_000)
  const gridW = totalSlots * CELL_MIN_WIDTH

  return (
    <Box style={{ display: 'flex', borderBottom: '1px solid var(--mantine-color-dark-6)', height: 44 }}>
      {/* Channel label */}
      <Box style={{
        width: CHANNEL_COL_W, minWidth: CHANNEL_COL_W,
        padding: '0 8px',
        display: 'flex', alignItems: 'center', gap: 6,
        borderRight: '1px solid var(--mantine-color-dark-5)',
        background: 'var(--mantine-color-dark-7)',
        position: 'sticky', left: 0, zIndex: 1,
      }}>
        {channel.icon && (
          <img src={channel.icon} alt="" style={{ width: 20, height: 20, objectFit: 'contain' }} />
        )}
        <Text size="xs" lineClamp={1}>{channel.name}</Text>
      </Box>
      {/* Programme cells */}
      <Box style={{ position: 'relative', width: gridW, minWidth: gridW, flex: '0 0 auto' }}>
        <NowBar from={from} to={to} />
        {channel.programmes.map((p, i) => (
          <ProgCell key={i} prog={p} from={from} to={to} onClick={onProg} />
        ))}
      </Box>
    </Box>
  )
}

// ── Programme detail popover ──────────────────────────────────────────────
function ProgDetail({
  prog, onClose, onRecord,
}: {
  prog: GuideProgramme | null
  onClose: () => void
  onRecord: (p: GuideProgramme) => void
}) {
  if (!prog) return null
  const start = new Date(prog.start)
  const stop  = new Date(prog.stop)
  const dur   = Math.round((stop.getTime() - start.getTime()) / 60_000)
  return (
    <Paper withBorder p="md" style={{ position: 'fixed', bottom: 24, right: 24, maxWidth: 360, zIndex: 200 }}>
      <Group justify="space-between" mb={4}>
        <Text fw={600} size="sm" lineClamp={1}>{prog.title}</Text>
        <Button size="xs" variant="subtle" onClick={onClose}>✕</Button>
      </Group>
      {prog.sub_title && <Text size="xs" c="dimmed" mb={4}>{prog.sub_title}</Text>}
      <Text size="xs" c="dimmed" mb={6}>
        {formatTime(start)} – {formatTime(stop)} ({dur} min)
      </Text>
      {prog.categories && prog.categories.length > 0 && (
        <Group gap={4} mb={6}>
          {prog.categories.map(c => (
            <Badge key={c} size="xs" variant="outline">{c}</Badge>
          ))}
        </Group>
      )}
      {prog.desc && (
        <Text size="xs" c="dimmed" lineClamp={4} mb={8}>{prog.desc}</Text>
      )}
      <Group gap="xs">
        <Button
          size="xs"
          color="red"
          leftSection={<IconPlayerRecord size={12} />}
          onClick={() => onRecord(prog)}
        >
          Record
        </Button>
      </Group>
    </Paper>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────
export function TvGuide() {
  const navigate = useNavigate()
  const [windowStart, setWindowStart] = useState(() => snapToHalfHour(new Date()))
  const [selectedProg, setSelectedProg] = useState<GuideProgramme | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  const windowEnd = addHours(windowStart, WINDOW_HOURS)

  const { data, isLoading, isError } = useQuery({
    queryKey: ['guide-grid', windowStart.toISOString()],
    queryFn: () => guideApi.grid(windowStart, windowEnd),
    staleTime: 5 * 60_000,
  })

  // Scroll now-bar into view on mount.
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollLeft = 0
    }
  }, [windowStart])

  function shiftWindow(hours: number) {
    setWindowStart(prev => addHours(prev, hours))
    setSelectedProg(null)
  }

  function jumpToNow() {
    setWindowStart(snapToHalfHour(new Date()))
    setSelectedProg(null)
  }

  function handleRecord(prog: GuideProgramme) {
    navigate('/dvr', {
      state: {
        prefill: {
          title: prog.title,
          start_at: prog.start,
          end_at: prog.stop,
        },
      },
    })
    setSelectedProg(null)
  }

  const channels = data?.channels ?? []

  return (
    <Stack gap="md" h="100%" style={{ overflow: 'hidden' }}>
      {/* Toolbar */}
      <Group justify="space-between">
        <Text size="lg" fw={600}>TV Guide</Text>
        <Group gap="xs">
          <Tooltip label="Previous 3 hours">
            <ActionIcon variant="subtle" onClick={() => shiftWindow(-WINDOW_HOURS)}>
              <IconChevronLeft size={16} />
            </ActionIcon>
          </Tooltip>
          <Button size="xs" variant="subtle" onClick={jumpToNow}>Now</Button>
          <Text size="sm" c="dimmed">
            {formatTime(windowStart)} – {formatTime(windowEnd)}
          </Text>
          <Tooltip label="Next 3 hours">
            <ActionIcon variant="subtle" onClick={() => shiftWindow(WINDOW_HOURS)}>
              <IconChevronRight size={16} />
            </ActionIcon>
          </Tooltip>
        </Group>
      </Group>

      <Paper withBorder style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        {isLoading ? (
          <Box style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Loader size="md" />
          </Box>
        ) : isError ? (
          <Alert icon={<IconAlertCircle size={16} />} color="red" m="md">
            Guide data unavailable. Check that the tuner is running and has EPG configured.
          </Alert>
        ) : channels.length === 0 ? (
          <Alert icon={<IconAlertCircle size={16} />} color="gray" m="md">
            No programme data in this time window. Try a different time or check your EPG sources.
          </Alert>
        ) : (
          <ScrollArea style={{ flex: 1 }} viewportRef={scrollRef} type="auto">
            <Box style={{ minWidth: CHANNEL_COL_W + (WINDOW_HOURS * 2) * CELL_MIN_WIDTH }}>
              <TimeHeader from={windowStart} to={windowEnd} />
              {channels.map(ch => (
                <ChannelRow
                  key={ch.epg_id}
                  channel={ch}
                  from={windowStart}
                  to={windowEnd}
                  onProg={setSelectedProg}
                />
              ))}
            </Box>
          </ScrollArea>
        )}
      </Paper>

      <ProgDetail
        prog={selectedProg}
        onClose={() => setSelectedProg(null)}
        onRecord={handleRecord}
      />
    </Stack>
  )
}
