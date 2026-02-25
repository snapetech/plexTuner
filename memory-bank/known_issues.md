# Known issues

<!-- Add bugs, limitations, and design tradeoffs as they are discovered or fixed. -->

## Cluster / Plex

- **Plex DVR channel limit (~480) applies to the wizard only.** When users add the tuner via Plex's "Set up" wizard, Plex fetches our lineup and tries to save it; that path fails above ~480 channels. For **zero-touch, no wizard**: use `-register-plex=/path/to/Plex` so we write DVR + XMLTV URIs and attempt to sync the full lineup into Plex's DB. When `-register-plex` is set we do not cap (full catalog); lineup sync into the DB requires Plex to use a table we can discover (see [docs/adr/0001-zero-touch-plex-lineup.md](docs/adr/0001-zero-touch-plex-lineup.md)). If lineup sync fails (schema unknown), we still serve the full lineup over HTTP but Plex may only show 480 if it re-fetches via the wizard path.
- **Plex is not deployed by this repo.** Plex Media Server is expected to run in the cluster (or on the node) from a separate deploy (e.g. sibling `k3s/plex`, Helm, or node install). If Plex is missing in the cluster, see [docs/runbooks/plex-in-cluster.md](docs/runbooks/plex-in-cluster.md) for how to check, why it's missing, and how to restore it.
- **HDHR manifest: nodeSelector + imagePullPolicy Never.** If you pin the deployment to a node (for Plex hostPath), the image must be loaded on that node (e.g. `k3d image import` or build on that node). Otherwise you can see one healthy pod on another node and `ErrImageNeverPull` / stuck rollout on the selected node. Load the image on the chosen node or leave nodeSelector commented out to run on any node.

## Security

- **Credentials:** Secrets must live only in `.env` (or environment). `.env` is in `.gitignore`. Never commit `.env` or log secrets. Use `.env.example` as a template (no real values).

