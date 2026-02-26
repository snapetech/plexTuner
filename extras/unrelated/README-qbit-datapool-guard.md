# qbit-datapool-guard

Starts **qbittorrent-nox** in a screen session named **qbit** only when `/mnt/datapool_lvm_media` is mounted from `/dev/mapper/datapool_lvm-media`. When that mount disappears, qbittorrent is stopped promptly (no heavy polling or I/O).

## Install on your node

1. Copy files:
   ```bash
   sudo cp extras/unrelated/qbit-datapool-guard.sh /usr/local/bin/
   sudo chmod +x /usr/local/bin/qbit-datapool-guard.sh
   sudo cp extras/unrelated/qbit-datapool-guard.service /etc/systemd/system/
   ```

2. Set the user that should run qbittorrent:
   ```bash
   sudo systemctl edit qbit-datapool-guard
   ```
   Add under `[Service]`:
   ```ini
   User=YOUR_USER
   Group=YOUR_GROUP
   ```

3. Enable and start:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now qbit-datapool-guard
   ```

## Behaviour

- **After boot:** The service waits until `/mnt/datapool_lvm_media` is mounted and its SOURCE is `/dev/mapper/datapool_lvm-media`, then starts `screen -dmS qbit qbittorrent-nox`.
- **While mounted:** It checks every 5 seconds that the mount is still there (lightweight `mountpoint`/`findmnt` check, no extra I/O).
- **When mount is gone:** It kills the screen session (and qbittorrent) and goes back to waiting for the mount. When you remount, it starts qbittorrent again.

## Optional env vars (in service override)

- `QBIT_MOUNT_POINT` – default `/mnt/datapool_lvm_media`
- `QBIT_EXPECTED_DEVICE` – default `/dev/mapper/datapool_lvm-media` (if `findmnt` shows e.g. `/dev/dm-3`, set this to that value)
- `QBIT_SCREEN_NAME` – default `qbit`
- `QBIT_CHECK_INTERVAL` – seconds between checks (default `5`)

Example override:
```bash
sudo systemctl edit qbit-datapool-guard
```
```ini
[Service]
Environment="QBIT_CHECK_INTERVAL=10"
```

## Attach to the session

```bash
screen -r qbit
```
Detach: `Ctrl+A`, then `D`.
