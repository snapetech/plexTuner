import { Drawer, AspectRatio, Text, Stack, Button } from '@mantine/core'
import { useEffect, useRef } from 'react'
import Hls from 'hls.js'
import type { Channel, Stream } from '../../api/channels'

interface Props {
  target: Channel | Stream | null
  opened: boolean
  onClose: () => void
}

function streamURL(target: Channel | Stream | null): string {
  if (!target) return ''
  if ('streams' in target) {
    // Channel — route through the webui /stream/ proxy using channel_number (tuner's GuideNumber)
    const ch = target as Channel
    const key = ch.channel_number || String(ch.id)
    return `${window.location.origin}/stream/${encodeURIComponent(key)}`
  }
  // Stream — use the raw URL
  return (target as Stream).url
}

export function PreviewDrawer({ target, opened, onClose }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const hlsRef   = useRef<Hls | null>(null)

  useEffect(() => {
    if (!opened || !target || !videoRef.current) return

    const url = streamURL(target)
    const video = videoRef.current

    if (Hls.isSupported()) {
      hlsRef.current?.destroy()
      const hls = new Hls({ enableWorker: false })
      hls.loadSource(url)
      hls.attachMedia(video)
      hlsRef.current = hls
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = url
    }

    return () => {
      hlsRef.current?.destroy()
      hlsRef.current = null
    }
  }, [opened, target])

  const name = target ? ('name' in target ? target.name : '') : ''

  return (
    <Drawer
      opened={opened}
      onClose={onClose}
      title={`Preview — ${name}`}
      position="right"
      size="lg"
      overlayProps={{ backgroundOpacity: 0.3 }}
    >
      <Stack gap="md">
        <AspectRatio ratio={16 / 9}>
          <video
            ref={videoRef}
            controls
            autoPlay
            playsInline
            style={{ width: '100%', height: '100%', background: '#000' }}
          />
        </AspectRatio>
        {target && 'url' in target && (
          <Text size="xs" c="dimmed" style={{ wordBreak: 'break-all' }}>
            {(target as Stream).url}
          </Text>
        )}
        <Button variant="default" onClick={onClose}>Close</Button>
      </Stack>
    </Drawer>
  )
}
