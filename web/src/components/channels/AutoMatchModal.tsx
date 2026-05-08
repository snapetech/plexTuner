import {
  Modal, Stack, Button, Group, Text, Tabs, TagsInput, Switch,
} from '@mantine/core'
import { useState } from 'react'

interface Props {
  channelCount: number
  opened: boolean
  onClose: () => void
  onRun: (opts: AutoMatchOpts) => Promise<void>
}

export interface AutoMatchOpts {
  ignore_prefixes: string[]
  ignore_suffixes: string[]
  ignore_strings: string[]
}

export function AutoMatchModal({ channelCount, opened, onClose, onRun }: Props) {
  const [advanced, setAdvanced] = useState(false)
  const [prefixes, setPrefixes] = useState<string[]>([])
  const [suffixes, setSuffixes] = useState<string[]>([])
  const [custom, setCustom] = useState<string[]>([])
  const [running, setRunning] = useState(false)

  async function handleRun() {
    setRunning(true)
    try {
      await onRun(
        advanced
          ? { ignore_prefixes: prefixes, ignore_suffixes: suffixes, ignore_strings: custom }
          : { ignore_prefixes: [], ignore_suffixes: [], ignore_strings: [] },
      )
    } finally {
      setRunning(false)
    }
  }

  return (
    <Modal opened={opened} onClose={onClose} title="Auto-Match EPG" size="md">
      <Stack gap="sm">
        <Text size="sm" c="dimmed">
          Match {channelCount > 0 ? `${channelCount} selected channel${channelCount !== 1 ? 's' : ''}` : 'all channels'} to EPG entries by channel name.
        </Text>

        <Switch
          label="Configure advanced options"
          checked={advanced}
          onChange={e => setAdvanced(e.currentTarget.checked)}
        />

        {advanced && (
          <Tabs defaultValue="prefixes">
            <Tabs.List>
              <Tabs.Tab value="prefixes">Prefixes</Tabs.Tab>
              <Tabs.Tab value="suffixes">Suffixes</Tabs.Tab>
              <Tabs.Tab value="custom">Custom strings</Tabs.Tab>
            </Tabs.List>
            <Tabs.Panel value="prefixes" pt="sm">
              <TagsInput
                label="Ignore prefixes (e.g. Prime:, US:)"
                placeholder="Type and press Enter"
                value={prefixes}
                onChange={setPrefixes}
              />
            </Tabs.Panel>
            <Tabs.Panel value="suffixes" pt="sm">
              <TagsInput
                label="Ignore suffixes (e.g. HD, 4K, +1)"
                placeholder="Type and press Enter"
                value={suffixes}
                onChange={setSuffixes}
              />
            </Tabs.Panel>
            <Tabs.Panel value="custom" pt="sm">
              <TagsInput
                label="Ignore strings anywhere (e.g. 24/7, LIVE)"
                placeholder="Type and press Enter"
                value={custom}
                onChange={setCustom}
              />
            </Tabs.Panel>
          </Tabs>
        )}

        <Text size="xs" c="dimmed">
          Channel display names are not modified. These settings only affect the matching algorithm.
        </Text>

        <Group justify="flex-end" mt="sm">
          <Button variant="default" onClick={onClose}>Cancel</Button>
          <Button onClick={handleRun} loading={running} color="teal">Run Auto-Match</Button>
        </Group>
      </Stack>
    </Modal>
  )
}
