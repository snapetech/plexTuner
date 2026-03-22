const bootData = JSON.parse(document.getElementById("deck-bootstrap")?.textContent || "{}");
const endpoints = {
  deckTelemetry: "/deck/telemetry.json",
  deckActivity: "/deck/activity.json",
  deckSettings: "/deck/settings.json",
  runtime: "/api/debug/runtime.json",
  health: "/api/healthz",
  guideHealth: "/api/guide/health.json",
  guideDoctor: "/api/guide/doctor.json",
  guideHighlights: "/api/guide/highlights.json?limit=8",
  epgStore: "/api/guide/epg-store.json",
  capsules: "/api/guide/capsules.json?limit=8",
  channelReport: "/api/channels/report.json",
  channelLeaderboard: "/api/channels/leaderboard.json?limit=8",
  channelDNA: "/api/channels/dna.json",
  autopilot: "/api/autopilot/report.json?limit=8",
  ghost: "/api/plex/ghost-report.json?observe=0s",
  provider: "/api/provider/profile.json",
  recorder: "/api/recordings/recorder.json?limit=8",
  attempts: "/api/debug/stream-attempts.json?limit=8",
  programmingCategories: "/api/programming/categories.json",
  programmingChannels: "/api/programming/channels.json",
  programmingOrder: "/api/programming/order.json",
  programmingBackups: "/api/programming/backups.json",
  programmingHarvest: "/api/programming/harvest.json",
  programmingHarvestImport: "/api/programming/harvest-import.json",
  programmingHarvestAssist: "/api/programming/harvest-assist.json",
  programmingRecipe: "/api/programming/recipe.json",
  programmingPreview: "/api/programming/preview.json?limit=12",
  virtualChannelSchedule: "/api/virtual-channels/schedule.json?horizon=3h",
  operatorActionsStatus: "/api/ops/actions/status.json",
  guideWorkflow: "/api/ops/workflows/guide-repair.json",
  streamWorkflow: "/api/ops/workflows/stream-investigate.json",
  diagnosticsWorkflow: "/api/ops/workflows/diagnostics.json",
  opsWorkflow: "/api/ops/workflows/ops-recovery.json"
};
const csrfHeaderName = "X-IPTVTunerr-Deck-CSRF";
const endpointCatalog = {
  deckTelemetry: { title: "Deck Telemetry", category: "Deck Memory", summary: "Read-only deck telemetry endpoint; browser trend memory stays local to the current operator session." },
  deckActivity: { title: "Deck Activity", category: "Deck Memory", summary: "Server-derived operator activity trail for the dedicated deck." },
  deckSettings: { title: "Deck Settings", category: "Deck Control", summary: "Runtime-backed deck posture and editable refresh cadence for the dedicated deck." },
  runtime: { title: "Runtime Snapshot", category: "Runtime", summary: "Effective config and capability snapshot for the running tuner." },
  health: { title: "Health", category: "Runtime", summary: "Basic liveness and loaded-channel count." },
  guideHealth: { title: "Guide Health", category: "Guide", summary: "Guide quality, placeholder count, and stale-channel pressure." },
  guideDoctor: { title: "Guide Doctor", category: "Guide", summary: "Guide parsing and repair diagnostics." },
  guideHighlights: { title: "Guide Highlights", category: "Guide", summary: "Current and starting-soon viewing moments." },
  epgStore: { title: "EPG Store", category: "Guide", summary: "SQLite guide storage and freshness horizon." },
  capsules: { title: "Catch-up Capsules", category: "Guide", summary: "Catch-up candidates derived from guide windows." },
  channelReport: { title: "Channel Report", category: "Intelligence", summary: "Channel quality and resilience summary." },
  channelLeaderboard: { title: "Leaderboard", category: "Intelligence", summary: "Top-ranked channels and health scoring." },
  channelDNA: { title: "Channel DNA", category: "Intelligence", summary: "Duplicate/provider grouping into stable channel identities." },
  autopilot: { title: "Autopilot", category: "Operations", summary: "Hot channels, decision count, optional multi-DNA consensus host (when enabled)." },
  ghost: { title: "Ghost Hunter", category: "Operations", summary: "Plex stale-session and hidden-grab signals." },
  provider: { title: "Provider Profile", category: "Routing", summary: "Tuner limits, CF/concurrency/mux counters, penalized hosts, advisory remediation_hints." },
  recorder: { title: "Recorder", category: "Operations", summary: "Catch-up recorder state, failures, and throughput." },
  attempts: { title: "Stream Attempts", category: "Routing", summary: "Recent fallback and failure evidence for live streams." },
  programmingCategories: { title: "Programming Categories", category: "Programming", summary: "Category inventory and bulk include/exclude controls for lineup curation." },
  programmingBrowse: { title: "Programming Browse", category: "Programming", summary: "Batch browse view for one category with cached guide and alternative-source status." },
  programmingChannels: { title: "Programming Channels", category: "Programming", summary: "Exact include/exclude channel controls for the saved programming recipe." },
  programmingOrder: { title: "Programming Order", category: "Programming", summary: "Manual lineup order mutations and order-mode state." },
  programmingBackups: { title: "Programming Backups", category: "Programming", summary: "Exact sibling backup groups that can collapse into one visible lineup row." },
  programmingHarvest: { title: "Programming Harvest", category: "Programming", summary: "Persisted Plex lineup-harvest report and deduped candidate lineup titles." },
  programmingHarvestImport: { title: "Programming Harvest Import", category: "Programming", summary: "Preview/apply a harvested lineup as a real Programming Manager recipe." },
  programmingHarvestAssist: { title: "Programming Harvest Assist", category: "Programming", summary: "Ranked local-market and exact-match recipe assists derived from harvested lineups." },
  programmingRecipe: { title: "Programming Recipe", category: "Programming", summary: "Durable saved recipe file backing category/channel/order decisions." },
  programmingPreview: { title: "Programming Preview", category: "Programming", summary: "Curated lineup preview with taxonomy buckets and backup groups." },
  virtualChannelSchedule: { title: "Virtual Channel Schedule", category: "Programming", summary: "Synthetic schedule horizon for published virtual channels." },
  operatorActionsStatus: { title: "Operator Action Status", category: "Deck Control", summary: "Availability and current status of safe operator actions." },
  guideWorkflow: { title: "Guide Workflow", category: "Workflows", summary: "Guided checklist for guide repair and freshness issues." },
  streamWorkflow: { title: "Stream Workflow", category: "Workflows", summary: "Guided lane for routing and upstream stream failures." },
  diagnosticsWorkflow: { title: "Diagnostics Workflow", category: "Workflows", summary: "Good-vs-bad capture plan, recent harness artifacts, and evidence-bundle intake for intermittent channel failures." },
  opsWorkflow: { title: "Ops Workflow", category: "Workflows", summary: "Guided lane for recorder, ghost, and Autopilot recovery." }
};

const actionDefinitions = {
  guide_refresh: {
    path: "/api/ops/actions/guide-refresh",
    label: "Run Guide Refresh",
    confirm: "Start a manual guide refresh now?"
  },
  stream_attempts_clear: {
    path: "/api/ops/actions/stream-attempts-clear",
    label: "Clear Attempt History",
    confirm: "Clear the volatile recent stream-attempt history?"
  },
  provider_profile_reset: {
    path: "/api/ops/actions/provider-profile-reset",
    label: "Reset Provider Penalties",
    confirm: "Reset learned provider behavior penalties and recent host failure state?"
  },
  autopilot_reset: {
    path: "/api/ops/actions/autopilot-reset",
    label: "Reset Autopilot Memory",
    confirm: "Clear remembered Autopilot playback decisions?"
  },
  ghost_visible_stop: {
    path: "/api/ops/actions/ghost-visible-stop",
    label: "Stop Visible Ghosts",
    confirm: "Stop visible stale Plex transcode sessions now?"
  },
  ghost_hidden_recover_dry_run: {
    path: "/api/ops/actions/ghost-hidden-recover?mode=dry-run",
    label: "Run Hidden Recovery Dry-Run",
    confirm: "Run the guarded hidden-grab recovery helper in dry-run mode?"
  },
  ghost_hidden_recover_restart: {
    path: "/api/ops/actions/ghost-hidden-recover?mode=restart",
    label: "Restart Hidden-Grabs",
    confirm: "Run the guarded hidden-grab recovery helper with restart mode?"
  },
  evidence_intake_start: {
    path: "/api/ops/actions/evidence-intake-start",
    label: "Create Evidence Bundle",
    confirm: "Create a new evidence-intake bundle scaffold under .diag/evidence?"
  },
  channel_diff_run: {
    path: "/api/ops/actions/channel-diff-run",
    label: "Run Channel Diff",
    confirm: "Run a bounded good-vs-bad channel diff capture using the current diagnostics suggestions?"
  },
  stream_compare_run: {
    path: "/api/ops/actions/stream-compare-run",
    label: "Run Stream Compare",
    confirm: "Run a bounded stream-compare capture for the currently suggested failing channel?"
  }
};

const modeTitles = {
  overview: ["Overview", "Live State"],
  guide: ["Guide", "Integrity"],
  routing: ["Routing", "Decision Trail"],
  ops: ["Operations", "Automation"],
  programming: ["Programming", "Lineup Curation"],
  settings: ["Settings", "Runtime Snapshot"]
};

const state = {
  mode: "overview",
  filter: "",
  payloads: {},
  selectedRaw: "runtime",
  modalView: { kind: "raw", key: "runtime" },
  refreshTimer: null,
  refreshRateSec: Number(bootData.defaultRefreshSec || 30),
  csrfToken: String(bootData.csrfToken || ""),
  actionFeedback: null,
  deckSettings: null,
  programmingSelectedChannelId: "",
  programmingSelectedCategoryId: "",
  programmingPlayer: {
    hls: null,
    url: "",
    videoId: ""
  },
  telemetry: {
    samples: [],
    maxSamples: 18
  },
  activity: {
    entries: []
  },
  sharedTelemetry: {
    samples: []
  }
};
const storageKeys = {
  prefs: "iptvtunerr.deck.prefs.v1"
};

const healthPill = document.getElementById("health-pill");
const sidebarHealth = document.getElementById("sidebar-health");
const crumbMode = document.getElementById("crumb-mode");
const crumbFocus = document.getElementById("crumb-focus");
const activeFilters = document.getElementById("active-filters");
const heroMeta = document.getElementById("hero-meta");
const alertBoard = document.getElementById("alert-board");
const actionDock = document.getElementById("action-dock");
const signalGrid = document.getElementById("signal-grid");
const historyBar = document.getElementById("history-bar");
const trendGrid = document.getElementById("trend-grid");
const sessionKV = document.getElementById("session-kv");
const decisionBoard = document.getElementById("decision-board");
const postureCopy = document.getElementById("posture-copy");
const statsEl = document.getElementById("stats");
const overviewStories = document.getElementById("overview-stories");
const overviewTimeline = document.getElementById("overview-timeline");
const fastLanes = document.getElementById("fast-lanes");
const guideList = document.getElementById("guide-list");
const guideTimeline = document.getElementById("guide-timeline");
const routingList = document.getElementById("routing-list");
const attemptTrail = document.getElementById("attempt-trail");
const opsList = document.getElementById("ops-list");
const channelList = document.getElementById("channel-list");
const programmingList = document.getElementById("programming-list");
const programmingCategories = document.getElementById("programming-categories");
const programmingPreview = document.getElementById("programming-preview");
const programmingBackups = document.getElementById("programming-backups");
const programmingPlayer = document.getElementById("programming-player");
const programmingDetail = document.getElementById("programming-detail");
const settingsList = document.getElementById("settings-list");
const deckSettingsForm = document.getElementById("deck-settings-form");
const endpointGrid = document.getElementById("endpoint-grid");
const rawSelect = document.getElementById("raw-select");
const rawOutput = document.getElementById("raw-output");
const rawModal = document.getElementById("raw-modal");
const modalRich = document.getElementById("modal-rich");
const modalOutput = document.getElementById("modal-output");
const modalTitle = document.getElementById("modal-title");
const modalLabel = document.getElementById("modal-label");
const refreshRate = document.getElementById("refresh-rate");

function esc(value) {
  return String(value ?? "").replace(/[&<>"]/g, (ch) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    "\"": "&quot;"
  }[ch]));
}

function pretty(value) {
  if (value === null || value === undefined || value === "") return "n/a";
  if (typeof value === "boolean") return value ? "enabled" : "disabled";
  if (Array.isArray(value)) return value.length ? value.join(", ") : "none";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function normalizeArray(value) {
  return Array.isArray(value) ? value : [];
}

function diagnosticsRunSummary(run) {
  if (!run || typeof run !== "object") return "";
  const parts = [];
  if (run.family) parts.push(String(run.family));
  if (run.verdict) parts.push(`verdict=${run.verdict}`);
  const summary = normalizeArray(run.summary).filter(Boolean).slice(0, 2).join(" | ");
  if (summary) parts.push(summary);
  return parts.join(" · ");
}

/** Compact operator view: /provider/profile.json is a flat JSON object (no legacy summary wrapper). */
function summarizeProviderProfile(body) {
  if (!body || typeof body !== "object") return null;
  if (body.error) return null;
  if (body.summary && typeof body.summary === "object") return body.summary;
  if (body.effective_tuner_limit === undefined && body.configured_tuner_limit === undefined) return null;
  const hints = normalizeArray(body.remediation_hints);
  const ia = body.intelligence?.autopilot || {};
  return {
    effective_tuner_limit: body.effective_tuner_limit,
    learned_tuner_limit: body.learned_tuner_limit,
    penalized_hosts: (body.penalized_hosts || []).length,
    quarantined_hosts: (body.quarantined_hosts || []).length,
    auto_host_quarantine: !!body.auto_host_quarantine,
    upstream_quarantine_skips_total: Number(body.upstream_quarantine_skips_total) || 0,
    cf_block_hits: body.cf_block_hits,
    concurrency_signals_seen: body.concurrency_signals_seen,
    remediation_hint_count: hints.length,
    last_hls_mux_outcome: body.last_hls_mux_outcome,
    autopilot_decisions: ia.decision_count ?? 0,
    autopilot_consensus_host: ia.consensus_host || "",
    autopilot_consensus_dna: ia.consensus_dna_count ?? 0,
    autopilot_consensus_hits: ia.consensus_hit_sum ?? 0,
    autopilot_consensus_runtime: !!ia.consensus_host_runtime_enabled
  };
}

function remediationHintsFromProfile(body) {
  return normalizeArray(body?.remediation_hints);
}

/** Meta line for /autopilot/report.json (consensus_* fields from tuner). */
function formatAutopilotConsensusMeta(ap) {
  if (!ap || typeof ap !== "object") return "";
  const host = ap.consensus_host;
  const runtime = !!ap.consensus_host_runtime_enabled;
  if (!host) {
    return runtime ? "Consensus: (no qualifying host yet) · IPTV_TUNERR_AUTOPILOT_CONSENSUS_HOST=on" : "";
  }
  const dna = ap.consensus_dna_count ?? 0;
  const hits = ap.consensus_hit_sum ?? 0;
  const run = runtime ? "runtime reorder on" : "runtime reorder off";
  return `Consensus: ${esc(host)} · ${dna} DNA · ${hits} hit-sum · ${run}`;
}

function createActionButton(actionKey, label = "") {
  const def = actionDefinitions[actionKey];
  if (!def) return "";
  return `<button class="tiny" type="button" data-action="${esc(actionKey)}">${esc(label || def.label)}</button>`;
}

function createWorkflowButton(workflowKey, label) {
  return `<button class="tiny" type="button" data-workflow="${esc(workflowKey)}">${esc(label)}</button>`;
}

function createCard(title, body, meta = "", tone = "", endpointKey = "", extraActions = "") {
  const inspectAction = endpointKey
    ? `<button class="tiny" type="button" data-inspect="${esc(endpointKey)}">Inspect Payload</button>`
    : "";
  const actions = inspectAction || extraActions
    ? `<div class="card-actions">${extraActions}${inspectAction}</div>`
    : "";
  return `<div class="card ${tone}"><strong>${esc(title)}</strong><div>${body}</div>${meta ? `<div class="meta">${meta}</div>` : ""}${actions}</div>`;
}

function createStat(label, value, note) {
  return `<div class="stat"><div class="label">${esc(label)}</div><div class="value">${esc(pretty(value))}</div><div class="note">${esc(note)}</div></div>`;
}

function createTimeline(title, body, tone = "") {
  return `<div class="timeline-item ${tone}"><strong>${esc(title)}</strong><div class="meta">${body}</div></div>`;
}

function endpointDetails(key) {
  return endpointCatalog[key] || {
    title: key,
    category: "Other",
    summary: endpoints[key] || ""
  };
}

function createKeyValueRows(entries) {
  return entries.map(([label, value]) =>
    `<div class="detail-line"><dt>${esc(label)}</dt><dd>${esc(pretty(value))}</dd></div>`
  ).join("");
}

function createSettingsCard(title, summary, entries, endpointKey = "", extraActions = "") {
  return createCard(
    title,
    `${summary}<div class="settings-kv">${createKeyValueRows(entries)}</div>`,
    "",
    "",
    endpointKey,
    extraActions
  );
}

function createTinyButton(attrs, label) {
  return `<button class="tiny" type="button" ${attrs}>${esc(label)}</button>`;
}

function channelName(channel) {
  return channel?.guide_name || channel?.GuideName || channel?.channel_id || channel?.ChannelID || "channel";
}

function channelDescriptor(channel, descriptorMap = null) {
  if (channel?.descriptor?.label || channel?.Descriptor?.Label) {
    return channel?.descriptor || channel?.Descriptor;
  }
  const id = programmingChannelID(channel);
  if (descriptorMap && id && descriptorMap[id]) {
    return descriptorMap[id];
  }
  return {};
}

function channelDescriptorLabel(channel, descriptorMap = null) {
  const descriptor = channelDescriptor(channel, descriptorMap);
  if (descriptor?.label || descriptor?.Label) {
    return descriptor.label || descriptor.Label;
  }
  return "";
}

function programmingCategoryButtons(category, recipe) {
  const selected = new Set(normalizeArray(recipe?.selected_categories));
  const excluded = new Set(normalizeArray(recipe?.excluded_categories));
  const id = category?.id || "";
  return [
    createTinyButton(`data-programming-category-select="${esc(id)}"`, state.programmingSelectedCategoryId === id ? "Browsing" : "Browse"),
    createTinyButton(`data-programming-post="category" data-programming-action="include" data-programming-id="${esc(id)}"`, selected.has(id) ? "Included" : "Include"),
    createTinyButton(`data-programming-post="category" data-programming-action="exclude" data-programming-id="${esc(id)}"`, excluded.has(id) ? "Excluded" : "Exclude"),
    createTinyButton(`data-programming-post="category" data-programming-action="remove" data-programming-id="${esc(id)}"`, "Clear"),
    createTinyButton(`data-open-path="/api/programming/categories.json?category=${encodeURIComponent(id)}" data-open-title="Programming Category · ${esc(category?.label || id)}"`, "Members")
  ].join("");
}

function programmingChannelButtons(channel, recipe) {
  const included = new Set(normalizeArray(recipe?.included_channel_ids));
  const excluded = new Set(normalizeArray(recipe?.excluded_channel_ids));
  const id = programmingChannelID(channel);
  const label = channelName(channel);
  return [
    createTinyButton(`data-programming-post="channel" data-programming-action="include" data-programming-id="${esc(id)}"`, included.has(id) ? "Pinned In" : "Include"),
    createTinyButton(`data-programming-post="channel" data-programming-action="exclude" data-programming-id="${esc(id)}"`, excluded.has(id) ? "Blocked" : "Exclude"),
    createTinyButton(`data-programming-post="channel" data-programming-action="remove" data-programming-id="${esc(id)}"`, "Clear"),
    createTinyButton(`data-open-path="/api/programming/channels.json?category=${encodeURIComponent(channel?.GroupTitle || channel?.group_title || "")}" data-open-title="Programming Channel · ${esc(label)}"`, "Context")
  ].join("");
}

function programmingStreamCompareButton(channel, label = "Stream Compare") {
  const id = programmingChannelID(channel);
  if (!id) return "";
  return createTinyButton(`data-action="stream_compare_run" data-action-payload="${esc(JSON.stringify({ channel_id: id }))}"`, label);
}

function programmingChannelDiffButton(goodChannel, badChannel, label = "Channel Diff") {
  const goodID = programmingChannelID(goodChannel);
  const badID = programmingChannelID(badChannel);
  if (!goodID || !badID) return "";
  return createTinyButton(`data-action="channel_diff_run" data-action-payload="${esc(JSON.stringify({ good_channel_id: goodID, bad_channel_id: badID }))}"`, label);
}

function programmingOrderButtons(channel, lineup, recipe) {
  const id = programmingChannelID(channel);
  const ids = normalizeArray(lineup).map((item) => programmingChannelID(item));
  const idx = ids.indexOf(id);
  const prev = idx > 0 ? ids[idx - 1] : "";
  const next = idx >= 0 && idx < ids.length - 1 ? ids[idx + 1] : "";
  return [
    createTinyButton(`data-programming-post="order" data-programming-action="prepend" data-programming-id="${esc(id)}"`, "Pin First"),
    prev ? createTinyButton(`data-programming-post="order" data-programming-action="before" data-programming-id="${esc(id)}" data-programming-anchor="${esc(prev)}"`, "Up") : "",
    next ? createTinyButton(`data-programming-post="order" data-programming-action="after" data-programming-id="${esc(id)}" data-programming-anchor="${esc(next)}"`, "Down") : "",
    createTinyButton(`data-programming-post="order" data-programming-action="remove" data-programming-id="${esc(id)}"`, "Drop Order"),
    createTinyButton(`data-programming-post="channel" data-programming-action="include" data-programming-id="${esc(id)}"`, normalizeArray(recipe?.included_channel_ids).includes(id) ? "Pinned In" : "Include")
  ].join("");
}

function programmingSelectButtons(channel, lineup, recipe) {
  const id = programmingChannelID(channel);
  const label = channelName(channel);
  const selected = id === state.programmingSelectedChannelId;
  return [
    createTinyButton(`data-programming-select="${esc(id)}"`, selected ? "Previewing" : "Preview"),
    createTinyButton(`data-open-path="/api/programming/channel-detail.json?channel_id=${encodeURIComponent(id)}" data-open-title="Programming Detail · ${esc(label)}"`, "Inspect Detail"),
    `<a class="tiny" href="${esc(programmingPreviewURL(id))}" target="_blank" rel="noreferrer">Open Stream</a>`,
    programmingOrderButtons(channel, lineup, recipe)
  ].filter(Boolean).join(" ");
}

function programmingHarvestButtons(row) {
  const title = esc(row?.lineup_title || "");
  return [
    createTinyButton(`data-open-path="/api/programming/harvest-import.json?lineup_title=${encodeURIComponent(row?.lineup_title || "")}&replace=1" data-open-title="Harvest Import Preview · ${title}"`, "Preview Import"),
    createTinyButton(`data-programming-harvest-import="${title}" data-programming-harvest-collapse="1"`, "Apply")
  ].join("");
}

function backupGroupSummary(group) {
  const members = normalizeArray(group?.members);
  return members.slice(0, 4).map((member) => {
    const name = member.guide_name || member.channel_id || "channel";
    const descriptor = channelDescriptorLabel(member);
    const source = member.source_tag ? ` · ${member.source_tag}` : "";
    return `${name}${descriptor ? ` · ${descriptor}` : ""}${source}`;
  }).join(" | ");
}

function renderDeckSettingsPanel(settings) {
  const refreshValue = Number(settings?.default_refresh_sec ?? state.refreshRateSec ?? 30);
  deckSettingsForm.innerHTML = `
    <div class="deck-settings-grid">
      <label>
        Default Refresh
        <select id="deck-settings-refresh">
          <option value="0"${refreshValue === 0 ? " selected" : ""}>Manual</option>
          <option value="15"${refreshValue === 15 ? " selected" : ""}>15s</option>
          <option value="30"${refreshValue === 30 ? " selected" : ""}>30s</option>
          <option value="60"${refreshValue === 60 ? " selected" : ""}>60s</option>
          <option value="120"${refreshValue === 120 ? " selected" : ""}>120s</option>
        </select>
      </label>
    </div>
    <div class="deck-settings-actions">
      <button id="deck-settings-save" type="button">Save Deck Preferences</button>
    </div>
    <div class="deck-settings-note">
      Session TTL: ${esc(pretty(settings?.effective_session_ttl_minutes))} minutes. Login rate limit: ${esc(pretty(settings?.login_failure_limit))} failures per ${esc(pretty(settings?.login_failure_window_minutes))} minutes. Authentication is configured from startup env only. ${settings?.state_persisted ? "Deck preferences persist across restarts." : "Without a web UI state file, deck preferences last only until this process restarts."}
    </div>
  `;
}

function createAlertCard(kind, title, items) {
  return `<article class="alert-card ${kind}"><h3>${esc(title)}</h3><ul>${items.map((item) => `<li>${esc(item)}</li>`).join("")}</ul></article>`;
}

function formatWhen(value) {
  if (!value) return "n/a";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function summarizeGuideRefresh(status) {
  if (!status) return "Guide refresh status unavailable.";
  if (status.in_flight) {
    return `Refresh in flight since ${formatWhen(status.last_started_at)}.`;
  }
  if (status.last_error) {
    return `Last refresh failed after ${pretty(status.last_duration_ms ? `${status.last_duration_ms} ms` : "unknown duration")}: ${status.last_error}.`;
  }
  if (status.last_ended_at) {
    return `Last refresh finished ${formatWhen(status.last_ended_at)}; cache ${status.cache_populated ? "populated" : "empty"}.`;
  }
  return "Guide refresh has not run yet in this process.";
}

function actionKind(result) {
  if (!result) return "";
  if (result.ok === true) return "ok";
  if (result.ok === false) return "bad";
  return "warn";
}

function setActionFeedback(result) {
  state.actionFeedback = {
    ok: result?.ok,
    action: result?.action || "operator_action",
    message: result?.message || "No response body returned.",
    detail: result?.detail || null,
    at: new Date().toISOString()
  };
}

function safeLocalStorage() {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

function loadPersistedState() {
  const storage = safeLocalStorage();
  if (!storage) return;
  try {
    const rawPrefs = storage.getItem(storageKeys.prefs);
    if (rawPrefs) {
      const prefs = JSON.parse(rawPrefs);
      if (prefs.mode && modeTitles[prefs.mode]) state.mode = prefs.mode;
      if (Number.isFinite(Number(prefs.refreshRateSec))) state.refreshRateSec = Number(prefs.refreshRateSec);
      if (prefs.selectedRaw && endpoints[prefs.selectedRaw]) state.selectedRaw = prefs.selectedRaw;
      if (prefs.programmingSelectedChannelId) state.programmingSelectedChannelId = String(prefs.programmingSelectedChannelId);
      if (prefs.programmingSelectedCategoryId) state.programmingSelectedCategoryId = String(prefs.programmingSelectedCategoryId);
    }
  } catch {
  }
}

function persistPrefs() {
  const storage = safeLocalStorage();
  if (!storage) return;
  try {
    storage.setItem(storageKeys.prefs, JSON.stringify({
      mode: state.mode,
      refreshRateSec: state.refreshRateSec,
      selectedRaw: state.selectedRaw,
      programmingSelectedChannelId: state.programmingSelectedChannelId,
      programmingSelectedCategoryId: state.programmingSelectedCategoryId
    }));
  } catch {}
}

function programmingChannelID(channel) {
  return String(channel?.ChannelID || channel?.channel_id || "").trim();
}

function pickProgrammingSelection(curatedLineup, detailPayload = {}) {
  const existing = String(state.programmingSelectedChannelId || "").trim();
  if (existing && curatedLineup.some((item) => programmingChannelID(item) === existing)) {
    return existing;
  }
  const detailID = programmingChannelID(detailPayload.Channel || detailPayload.channel);
  if (detailID && curatedLineup.some((item) => programmingChannelID(item) === detailID)) {
    return detailID;
  }
  return programmingChannelID(curatedLineup[0]);
}

function pickProgrammingCategorySelection(inventory, current = "") {
  const wanted = String(current || "").trim();
  if (wanted && inventory.some((item) => String(item?.id || "").trim() === wanted)) {
    return wanted;
  }
  return String(inventory[0]?.id || "").trim();
}

function findProgrammingChannel(lineup, id) {
  const wanted = String(id || "").trim();
  return lineup.find((item) => programmingChannelID(item) === wanted) || null;
}

function programmingPreviewURL(channelID) {
  const id = String(channelID || "").trim();
  return id ? `/api/stream/${encodeURIComponent(id)}?mux=hls` : "";
}

function destroyProgrammingPlayer() {
  if (state.programmingPlayer?.hls) {
    try {
      state.programmingPlayer.hls.destroy();
    } catch {}
  }
  state.programmingPlayer = { hls: null, url: "", videoId: "" };
}

let hlsLibraryPromise = null;

async function ensureHlsLibrary() {
  if (window.Hls) return window.Hls;
  if (!hlsLibraryPromise) {
    hlsLibraryPromise = new Promise((resolve, reject) => {
      const existing = document.querySelector('script[data-hlsjs="deck"]');
      if (existing) {
        existing.addEventListener("load", () => resolve(window.Hls));
        existing.addEventListener("error", () => reject(new Error("Failed to load hls.js")));
        return;
      }
      const script = document.createElement("script");
      script.src = "https://cdn.jsdelivr.net/npm/hls.js@1.5.7";
      script.async = true;
      script.dataset.hlsjs = "deck";
      script.onload = () => resolve(window.Hls);
      script.onerror = () => reject(new Error("Failed to load hls.js"));
      document.head.appendChild(script);
    });
  }
  return hlsLibraryPromise;
}

async function syncProgrammingPlayer() {
  const video = document.getElementById("programming-live-video");
  const fallback = document.getElementById("programming-live-fallback");
  if (!video) {
    destroyProgrammingPlayer();
    return;
  }
  const url = video.getAttribute("data-stream-url") || "";
  if (!url) {
    destroyProgrammingPlayer();
    return;
  }
  if (state.programmingPlayer.url === url && state.programmingPlayer.videoId === video.id) {
    return;
  }
  destroyProgrammingPlayer();
  state.programmingPlayer.url = url;
  state.programmingPlayer.videoId = video.id;
  video.muted = true;
  if (video.canPlayType("application/vnd.apple.mpegurl")) {
    video.src = url;
    if (fallback) fallback.hidden = true;
    return;
  }
  try {
    const HlsCtor = await ensureHlsLibrary();
    if (!HlsCtor?.isSupported?.()) {
      throw new Error("HLS preview is not supported in this browser.");
    }
    const hls = new HlsCtor({ enableWorker: true, lowLatencyMode: false });
    state.programmingPlayer.hls = hls;
    hls.loadSource(url);
    hls.attachMedia(video);
    if (fallback) fallback.hidden = true;
  } catch (err) {
    if (fallback) {
      fallback.hidden = false;
      fallback.textContent = String(err);
    }
  }
}

function clampPercent(value) {
  if (!Number.isFinite(value)) return 0;
  if (value < 0) return 0;
  if (value > 100) return 100;
  return Math.round(value);
}

function createSignalCard(title, value, percent, meta, tone = "") {
  return `
    <article class="signal-card">
      <strong>${esc(title)}</strong>
      <div class="signal-value">${esc(value)}</div>
      <div class="signal-bar"><div class="signal-fill ${tone}" style="width:${clampPercent(percent)}%"></div></div>
      <div class="meta">${esc(meta)}</div>
    </article>
  `;
}

function deltaLabel(delta, suffix = "%") {
  if (!Number.isFinite(delta) || Math.abs(delta) < 0.5) return { label: "flat", tone: "" };
  const rounded = Math.round(delta);
  return {
    label: `${rounded > 0 ? "+" : ""}${rounded}${suffix}`,
    tone: rounded > 0 ? "up" : "down"
  };
}

function sparkTone(value) {
  if (value >= 85) return "good";
  if (value >= 55) return "warn";
  return "bad";
}

function createTrendCard(title, value, history, meta, suffix = "%") {
  const values = history.map((item) => Number(item.value || 0));
  const latest = values.length ? values[values.length - 1] : 0;
  const previous = values.length > 1 ? values[values.length - 2] : latest;
  const delta = deltaLabel(latest - previous, suffix);
  const bars = values.length
    ? values.map((item) => `<span class="${sparkTone(item)}" style="height:${Math.max(8, clampPercent(item) * 0.62)}px"></span>`).join("")
    : `<span class="warn" style="height:8px"></span>`;
  return `
    <article class="trend-card">
      <strong>${esc(title)}</strong>
      <div class="trend-head">
        <div class="trend-value">${esc(value)}</div>
        <div class="trend-delta ${delta.tone}">${esc(delta.label)}</div>
      </div>
      <div class="sparkline">${bars}</div>
      <div class="trend-meta">${esc(meta)}</div>
    </article>
  `;
}

function scheduleRefresh() {
  if (state.refreshTimer) {
    clearInterval(state.refreshTimer);
    state.refreshTimer = null;
  }
  if (state.refreshRateSec > 0) {
    state.refreshTimer = setInterval(() => {
      reloadDeck();
    }, state.refreshRateSec * 1000);
  }
}

function authHeaders(extra = {}) {
  const headers = { ...extra };
  if (state.csrfToken) {
    headers[csrfHeaderName] = state.csrfToken;
  }
  return headers;
}

function filterCards(items) {
  const q = state.filter.trim().toLowerCase();
  if (!q) return items;
  return items.filter((html) => html.toLowerCase().includes(q));
}

async function fetchJSON(label, path) {
  const res = await fetch(path, { headers: { "Accept": "application/json" } });
  if (res.status === 401) {
    const next = encodeURIComponent(window.location.pathname + window.location.search + window.location.hash);
    window.location.assign(`/login?next=${next}`);
    throw new Error("deck session expired");
  }
  const text = await res.text();
  let body;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = { raw: text };
  }
  return { label, path, ok: res.ok, status: res.status, body };
}

function setHealth(ok, label) {
  healthPill.textContent = label;
  healthPill.className = `status-badge ${ok ? "ok" : "warn"}`;
  sidebarHealth.innerHTML = ok
    ? `<strong>System State</strong><p>Deck can reach the tuner. This is a live operator surface, not a static shell.</p>`
    : `<strong>System State</strong><p>Tuner is not reachable right now. The deck stays up, but payload cards reflect degraded state.</p>`;
}

function showModalRaw(text, title, label) {
  modalRich.hidden = true;
  modalRich.innerHTML = "";
  modalOutput.hidden = false;
  modalTitle.textContent = title;
  modalLabel.textContent = label;
  modalOutput.textContent = text;
}

function openModal() {
  rawModal.classList.add("open");
  rawModal.setAttribute("aria-hidden", "false");
}

function closeModal() {
  rawModal.classList.remove("open");
  rawModal.setAttribute("aria-hidden", "true");
}

function actionPathToKey(path) {
  const clean = String(path || "").replace(/^\/api/, "");
  return Object.keys(actionDefinitions).find((key) => actionDefinitions[key].path.replace(/^\/api/, "") === clean) || "";
}

function renderWorkflowAction(path) {
  const actionKey = actionPathToKey(path);
  if (actionKey) {
    return createActionButton(actionKey);
  }
  const endpointKey = Object.keys(endpoints).find((key) => endpoints[key].replace(/^\/api/, "") === path.replace(/^\/api/, ""));
  if (endpointKey) {
    return `<button class="tiny" type="button" data-inspect="${esc(endpointKey)}">Inspect ${esc(path)}</button>`;
  }
  return `<button class="tiny" type="button" data-open-path="${esc(`/api${path}`)}" data-open-title="${esc(path)}">Open ${esc(path)}</button>`;
}

function renderWorkflowModal(workflowKey) {
  const payload = state.payloads[workflowKey]?.body || {};
  const steps = normalizeArray(payload.steps);
  const actions = normalizeArray(payload.actions);
  const summary = payload.summary && typeof payload.summary === "object" ? payload.summary : {};
  modalOutput.hidden = true;
  modalOutput.textContent = "";
  modalRich.hidden = false;
  modalTitle.textContent = `Workflow · ${payload.name || workflowKey}`;
  modalLabel.textContent = endpoints[workflowKey] || "Workflow";
  modalRich.innerHTML = `
    <div class="workflow-box">
      <div class="workflow-summary">
        ${Object.keys(summary).length
          ? Object.entries(summary).map(([key, value]) =>
            `<div class="detail-line"><dt>${esc(key)}</dt><dd>${esc(pretty(value))}</dd></div>`
          ).join("")
          : `<div class="workflow-note">No summary payload returned.</div>`}
      </div>
      <div class="workflow-steps">
        ${steps.length
          ? steps.map((step, index) => `<div class="workflow-step"><strong>${index + 1}.</strong> ${esc(step)}</div>`).join("")
          : `<div class="workflow-note">No workflow steps returned.</div>`}
      </div>
      <div>
        <div class="workflow-note">Recommended controls and evidence surfaces</div>
        <div class="workflow-actions">${actions.length ? actions.map(renderWorkflowAction).join("") : `<span class="workflow-note">No workflow actions returned.</span>`}</div>
      </div>
    </div>
  `;
  state.modalView = { kind: "workflow", key: workflowKey };
  openModal();
}

function renderActionDock(operatorStatus, guideWorkflow, streamWorkflow, opsWorkflow) {
  const refreshStatus = operatorStatus.guide_refresh?.status || {};
  const banner = state.actionFeedback
    ? `<div class="action-banner ${actionKind(state.actionFeedback)}"><strong>${esc(state.actionFeedback.action)}</strong><div>${esc(state.actionFeedback.message)}</div><div class="meta">${esc(formatWhen(state.actionFeedback.at))}${state.actionFeedback.detail ? ` · detail: ${esc(pretty(state.actionFeedback.detail))}` : ""}</div></div>`
    : "";

  actionDock.innerHTML = `
    ${banner}
    <div class="action-grid">
      <article class="action-panel">
        <h3>Guide Recovery</h3>
        <p>When listings look stale or placeholder-heavy, start with the playbook and then run a manual refresh if needed.</p>
        <div class="meta">${esc(summarizeGuideRefresh(refreshStatus))}</div>
        <div class="action-row">
          ${createWorkflowButton("guideWorkflow", "Open Guide Workflow")}
          ${createActionButton("guide_refresh")}
          <button class="tiny" type="button" data-inspect="guideHealth">Inspect Guide Health</button>
        </div>
      </article>
      <article class="action-panel">
        <h3>Stream Triage</h3>
        <p>Use the failure workflow first, then clear volatile evidence or reset penalties when you want a clean retry lane.</p>
        <div class="meta">${esc(pretty(streamWorkflow.summary?.recent_attempt_count || 0))} recent attempts in the workflow snapshot.</div>
        <div class="action-row">
          ${createWorkflowButton("streamWorkflow", "Open Stream Workflow")}
          ${createActionButton("stream_attempts_clear")}
          ${createActionButton("provider_profile_reset")}
        </div>
      </article>
      <article class="action-panel">
        <h3>Automation Memory</h3>
        <p>Autopilot memory is useful when it is helping and dangerous when it is stale. Reset it deliberately, not casually.</p>
        <div class="meta">${operatorStatus.autopilot_reset?.available ? "Autopilot actions are available." : "Autopilot state is not configured in this process."}</div>
        <div class="action-row">
          ${createWorkflowButton("opsWorkflow", "Open Ops Workflow")}
          ${createActionButton("autopilot_reset")}
          <button class="tiny" type="button" data-inspect="autopilot">Inspect Autopilot</button>
          <button class="tiny" type="button" data-inspect="operatorActionsStatus">Inspect Action Status</button>
        </div>
      </article>
      <article class="action-panel">
        <h3>Ghost Hunter</h3>
        <p>Use visible-stop first for stale sessions. Hidden-grab recovery stays guarded and split into dry-run vs restart on purpose.</p>
        <div class="meta">${opsWorkflow.summary?.ghost?.recommended_action ? esc(pretty(opsWorkflow.summary.ghost.recommended_action)) : "Ghost Hunter guidance is available from the ops workflow."}</div>
        <div class="action-row">
          ${createActionButton("ghost_visible_stop")}
          ${createActionButton("ghost_hidden_recover_dry_run", "Hidden Recovery Dry-Run")}
          ${createActionButton("ghost_hidden_recover_restart", "Hidden Recovery Restart")}
        </div>
      </article>
    </div>
  `;
}

function applyMode(mode) {
  state.mode = mode;
  document.querySelectorAll("#mode-nav button").forEach((button) => {
    button.classList.toggle("active", button.dataset.mode === mode);
  });
  document.querySelectorAll(".section").forEach((section) => {
    section.classList.toggle("active", section.dataset.mode === mode);
  });
  const pair = modeTitles[mode] || ["Deck", "View"];
  crumbMode.textContent = pair[0];
  crumbFocus.textContent = pair[1];
  persistPrefs();
}

function fillRawSelectors() {
  rawSelect.innerHTML = "";
  const groups = {};
  Object.keys(endpoints).forEach((key) => {
    const meta = endpointDetails(key);
    const opt = document.createElement("option");
    opt.value = key;
    opt.textContent = `${meta.title} -> ${endpoints[key]}`;
    rawSelect.appendChild(opt);
    if (!groups[meta.category]) groups[meta.category] = [];
    groups[meta.category].push(key);
  });
  endpointGrid.innerHTML = Object.keys(groups).sort().map((category) => `
    <section class="endpoint-group">
      <div class="endpoint-group-head">
        <strong>${esc(category)}</strong>
        <span>${groups[category].length} surfaces</span>
      </div>
      <div class="endpoint-group-grid">
        ${groups[category].map((key) => {
          const meta = endpointDetails(key);
          return `
            <button type="button" data-select-raw="${esc(key)}">
              <strong>${esc(meta.title)}</strong>
              <span class="endpoint-path">${esc(endpoints[key])}</span>
              <span class="endpoint-copy">${esc(meta.summary)}</span>
            </button>
          `;
        }).join("")}
      </div>
    </section>
  `).join("");
  rawSelect.value = state.selectedRaw;
}

function renderFilters() {
  const deckHistoryCount = state.sharedTelemetry.samples.length || state.telemetry.samples.length;
  const memoryLabel = state.payloads.runtime?.body?.webui?.memory_persisted ? "Shared memory" : "Volatile memory";
  activeFilters.innerHTML = state.filter
    ? `<span class="pill">Filter active: ${esc(state.filter)}</span>`
    : `<span class="pill">Mode: ${esc(modeTitles[state.mode][0])}</span><span class="pill">Deck target: ${esc(bootData.tunerBase || "n/a")}</span><span class="pill">${esc(memoryLabel)}: ${deckHistoryCount} samples</span>`;
}

function deriveMetrics(payloads) {
  const health = payloads.health || { ok: false, status: 0, body: {} };
  const guideHealth = payloads.guideHealth?.body || {};
  const recorder = payloads.recorder?.body || {};
  const attempts = normalizeArray(payloads.attempts?.body);
  const opsWorkflow = payloads.opsWorkflow?.body || {};
  const totalGuideChannels = Number(guideHealth.summary?.total_channels || guideHealth.channel_count || 0);
  const guideReal = Number(guideHealth.summary?.channels_with_real_programmes || guideHealth.summary?.channels_with_programmes || 0);
  const guidePercent = totalGuideChannels > 0 ? (guideReal / totalGuideChannels) * 100 : 0;
  const failedAttempts = attempts.filter((item) => {
    const val = `${item.reason || item.result || item.status || ""}`.toLowerCase();
    return val && val !== "ok";
  });
  const streamPercent = attempts.length > 0 ? ((attempts.length - failedAttempts.length) / attempts.length) * 100 : 100;
  const recorderTotal = Number((recorder.summary?.completed_count || 0) + (recorder.summary?.failed_count || 0));
  const recorderPercent = recorderTotal > 0 ? ((recorder.summary?.completed_count || 0) / recorderTotal) * 100 : (recorder.summary ? 100 : 0);
  const opsRisk = Number((opsWorkflow.summary?.ghost?.stale_count || 0) + (opsWorkflow.summary?.recorder?.failed_count || 0) + (opsWorkflow.summary?.recorder?.interrupted_count || 0));
  const opsPercent = opsRisk > 0 ? Math.max(15, 100 - (opsRisk * 20)) : 100;
  return {
    healthOK: !!health.ok,
    guidePercent,
    streamPercent,
    recorderPercent,
    opsPercent,
    attemptsCount: attempts.length,
    failedAttemptCount: failedAttempts.length,
    recorderFailed: Number(recorder.summary?.failed_count || 0),
    recorderCompleted: Number(recorder.summary?.completed_count || 0),
    opsRisk,
    guideReal,
    totalGuideChannels
  };
}

function recordTelemetry(metrics) {
  const samples = state.telemetry.samples;
  const last = samples.length ? samples[samples.length - 1] : null;
  if (last && last.tick === metrics.attemptsCount && last.guidePercent === metrics.guidePercent && last.streamPercent === metrics.streamPercent && last.recorderPercent === metrics.recorderPercent && last.opsPercent === metrics.opsPercent && last.healthOK === metrics.healthOK) {
    return;
  }
  samples.push({
    at: Date.now(),
    tick: metrics.attemptsCount,
    healthOK: metrics.healthOK,
    guidePercent: metrics.guidePercent,
    streamPercent: metrics.streamPercent,
    recorderPercent: metrics.recorderPercent,
    opsPercent: metrics.opsPercent
  });
  if (samples.length > state.telemetry.maxSamples) {
    samples.splice(0, samples.length - state.telemetry.maxSamples);
  }
}

async function syncDeckTelemetry(metrics) {
  recordTelemetry(metrics);
  try {
    const res = await fetch(endpoints.deckTelemetry, { headers: authHeaders({ "Accept": "application/json" }) });
    const text = await res.text();
    const body = text ? JSON.parse(text) : {};
    state.sharedTelemetry.samples = normalizeArray(body.samples);
  } catch {
    state.sharedTelemetry.samples = [];
  }
}

async function clearDeckTelemetry() {
  state.telemetry.samples = [];
  try {
    const res = await fetch(endpoints.deckTelemetry, { method: "DELETE", headers: authHeaders({ "Accept": "application/json" }) });
    const text = await res.text();
    const body = text ? JSON.parse(text) : {};
    state.sharedTelemetry.samples = normalizeArray(body.samples);
  } catch {
    state.sharedTelemetry.samples = [];
  }
  renderDeck();
}

async function syncDeckActivity(entry) {
  if (entry) {
    state.activity.entries = normalizeArray(state.activity.entries);
  }
  try {
    const res = await fetch(endpoints.deckActivity, { headers: authHeaders({ "Accept": "application/json" }) });
    const text = await res.text();
    const body = text ? JSON.parse(text) : {};
    state.activity.entries = normalizeArray(body.entries);
  } catch {
    state.activity.entries = [];
  }
}

async function clearDeckActivity() {
  try {
    const res = await fetch(endpoints.deckActivity, { method: "DELETE", headers: authHeaders({ "Accept": "application/json" }) });
    const text = await res.text();
    const body = text ? JSON.parse(text) : {};
    state.activity.entries = normalizeArray(body.entries);
  } catch {
    state.activity.entries = [];
  }
  renderDeck();
}

function renderDeck() {
  const runtime = state.payloads.runtime?.body || {};
  const health = state.payloads.health || { ok: false, status: 0, body: {} };
  const guideHealth = state.payloads.guideHealth?.body || {};
  const guideDoctor = state.payloads.guideDoctor?.body || {};
  const guideHighlights = state.payloads.guideHighlights?.body || {};
  const epgStore = state.payloads.epgStore?.body || {};
  const capsules = state.payloads.capsules?.body || {};
  const channelReport = state.payloads.channelReport?.body || {};
  const leaderboard = state.payloads.channelLeaderboard?.body || {};
  const dna = state.payloads.channelDNA?.body || {};
  const autopilot = state.payloads.autopilot?.body || {};
  const ghost = state.payloads.ghost?.body || {};
  const provider = state.payloads.provider?.body || {};
  const providerSummary = summarizeProviderProfile(provider);
  const remediationHints = remediationHintsFromProfile(provider);
  const providerLoadErr = provider.error || (state.payloads.provider && state.payloads.provider.ok === false ? `HTTP ${state.payloads.provider.status || 0}` : null);
  const recorder = state.payloads.recorder?.body || {};
  const attempts = normalizeArray(state.payloads.attempts?.body);
  const programmingCategoriesPayload = state.payloads.programmingCategories?.body || {};
  const programmingChannelsPayload = state.payloads.programmingChannels?.body || {};
  const programmingOrderPayload = state.payloads.programmingOrder?.body || {};
  const programmingBackupsPayload = state.payloads.programmingBackups?.body || {};
  const programmingHarvestAssistPayload = state.payloads.programmingHarvestAssist?.body || {};
  const programmingRecipePayload = state.payloads.programmingRecipe?.body || {};
  const programmingPreviewPayload = state.payloads.programmingPreview?.body || {};
  const operatorStatus = state.payloads.operatorActionsStatus?.body || {};
  const guideWorkflow = state.payloads.guideWorkflow?.body || {};
  const streamWorkflow = state.payloads.streamWorkflow?.body || {};
  const diagnosticsWorkflow = state.payloads.diagnosticsWorkflow?.body || {};
  const diagnosticRuns = normalizeArray(diagnosticsWorkflow.summary?.diag_runs);
  const opsWorkflow = state.payloads.opsWorkflow?.body || {};
  const deckSettings = state.payloads.deckSettings?.body || state.deckSettings || {};
  const activity = normalizeArray(state.payloads.deckActivity?.body?.entries).length
    ? normalizeArray(state.payloads.deckActivity?.body?.entries)
    : state.activity.entries;
  const metrics = deriveMetrics(state.payloads);
  const failedAttempts = attempts.filter((item) => {
    const val = `${item.reason || item.result || item.status || ""}`.toLowerCase();
    return val && val !== "ok";
  });

  setHealth(health.ok, health.ok ? "Healthy" : `HTTP ${health.status}`);
  renderFilters();

  postureCopy.textContent = health.ok
    ? `Main listener ${runtime.listen_address || "unknown"} is answering and the deck can see live runtime state.`
    : "The deck is loaded, but the tuner target is degraded or still starting.";

  sessionKV.innerHTML = [
    ["Tuner base", runtime.base_url || bootData.tunerBase || "n/a"],
    ["Listen address", runtime.listen_address || "n/a"],
    ["Device", `${runtime.friendly_name || "IPTV Tunerr"} (${runtime.device_id || "n/a"})`],
    ["Generated", runtime.generated_at || bootData.now || "n/a"],
    ["Activity log", `${activity.length} shared entries`]
  ].map(([label, value]) =>
    `<div class="detail-line"><dt>${esc(label)}</dt><dd>${esc(pretty(value))}</dd></div>`
  ).join("");

  heroMeta.innerHTML = [
    `<div class="meta-pill"><strong>Active mode</strong><span>${esc(modeTitles[state.mode][0])}</span></div>`,
    `<div class="meta-pill"><strong>Refresh cadence</strong><span>${state.refreshRateSec > 0 ? `${state.refreshRateSec}s auto` : "manual only"}</span></div>`,
    `<div class="meta-pill"><strong>Payload posture</strong><span>${health.ok ? "live deck with reachable tuner" : "deck up, tuner degraded"}</span></div>`,
    `<div class="meta-pill"><strong>Deck memory</strong><span>${runtime.webui?.memory_persisted ? `persisted at ${pretty(runtime.webui?.state_file)}` : "process-only shared memory"}</span></div>`
  ].join("");

  const incidents = [];
  if (!health.ok) incidents.push(`Tuner health endpoint is returning HTTP ${health.status}.`);
  if ((guideHealth.summary?.stale_channels || 0) > 0) incidents.push(`${guideHealth.summary.stale_channels} guide channels are stale.`);
  if ((guideHealth.summary?.placeholder_only_channels || 0) > 0) incidents.push(`${guideHealth.summary.placeholder_only_channels} guide channels are placeholder-only.`);
  if ((recorder.summary?.failed_count || 0) > 0) incidents.push(`${recorder.summary.failed_count} recorder items are failed.`);
  if (failedAttempts.length > 0) incidents.push(`${failedAttempts.length} recent stream attempts are non-OK.`);
  remediationHints.filter((h) => h && h.severity === "warn").slice(0, 4).forEach((h) => {
    incidents.push(`Provider remediation (${h.code}): ${h.message || "see /api/provider/profile.json"}`);
  });
  if (incidents.length === 0) incidents.push("No immediate red flags in the currently loaded payloads.");

  const watchItems = [];
  watchItems.push(`Provider entry count: ${pretty(runtime.provider?.entry_count || runtime.provider?.base_urls?.length || 0)}.`);
  watchItems.push(`Block Cloudflare providers: ${pretty(runtime.provider?.block_cf_providers)}.`);
  watchItems.push(`Web UI LAN access: ${pretty(runtime.webui?.allow_lan)}.`);
  watchItems.push(`EPG SQLite incremental upsert: ${pretty(runtime.guide?.epg_sqlite_incremental_upsert)}.`);
  watchItems.push(`Deck memory is ${runtime.webui?.memory_persisted ? `persisted at ${pretty(runtime.webui?.state_file)}` : "volatile until the web UI process restarts"}.`);
  watchItems.push(`Deck auth user is ${pretty(deckSettings.auth_user || runtime.webui?.auth_user)}${(deckSettings.auth_default_password ?? runtime.webui?.auth_default_password) ? " with the default password still in place." : "."}`);
  if (remediationHints.length > 0) {
    watchItems.push(`${remediationHints.length} advisory remediation hint(s) on provider profile (Routing lane).`);
  }
  if (providerSummary?.autopilot_consensus_host) {
    watchItems.push(`Autopilot consensus host ${providerSummary.autopilot_consensus_host} (${providerSummary.autopilot_consensus_dna} DNA, ${providerSummary.autopilot_consensus_hits} hit-sum).`);
  } else if (providerSummary?.autopilot_consensus_runtime) {
    watchItems.push("Autopilot consensus reorder is enabled; no qualifying host in the current snapshot.");
  }
  if (
    providerSummary &&
    (providerSummary.auto_host_quarantine ||
      (providerSummary.upstream_quarantine_skips_total || 0) > 0 ||
      (providerSummary.quarantined_hosts || 0) > 0)
  ) {
    const skips = providerSummary.upstream_quarantine_skips_total || 0;
    const qh = providerSummary.quarantined_hosts || 0;
    const on = providerSummary.auto_host_quarantine ? "on" : "off";
    watchItems.push(`Host quarantine (${on}): ${skips} upstream URL(s) skipped cumulatively; ${qh} host(s) quarantined now.`);
  }

  const wins = [];
  if (health.ok) wins.push("Deck can reach the tuner through the dedicated proxy origin.");
  if ((guideHealth.summary?.channels_with_real_programmes || guideHealth.summary?.channels_with_programmes || 0) > 0) wins.push("Guide payload contains real programme coverage.");
  if ((channelReport.summary?.channels_with_backup_streams || 0) > 0) wins.push("Channel catalog includes backup-capable streams.");
  if (runtime.generated_at) wins.push("Runtime snapshot is available for settings auditing.");
  if (remediationHints.length === 0 && providerSummary) wins.push("No advisory provider remediation hints in the current profile snapshot.");
  if (providerSummary?.autopilot_consensus_host) {
    wins.push(`Autopilot reports a consensus host (${providerSummary.autopilot_consensus_host}) across multiple channel memories.`);
  }
  if (providerSummary && (providerSummary.upstream_quarantine_skips_total || 0) > 0) {
    wins.push(`Host quarantine skipped ${providerSummary.upstream_quarantine_skips_total} bad upstream URL(s) while backups existed (see /api/provider/profile.json).`);
  }
  if (wins.length === 0) wins.push("No positive confirmations yet because the deck is still in a degraded state.");

  alertBoard.innerHTML = [
    createAlertCard("incident", "Incidents", incidents),
    createAlertCard("watch", "Watch Items", watchItems),
    createAlertCard("win", "Confirmed Wins", wins)
  ].join("");
  renderActionDock(operatorStatus, guideWorkflow, streamWorkflow, opsWorkflow);

  signalGrid.innerHTML = [
    createSignalCard("Guide Confidence", `${clampPercent(metrics.guidePercent)}%`, metrics.guidePercent, `${metrics.guideReal} of ${metrics.totalGuideChannels || 0} channels carry listings.`, metrics.guidePercent >= 85 ? "good" : metrics.guidePercent >= 55 ? "warn" : "bad"),
    createSignalCard("Stream Stability", `${clampPercent(metrics.streamPercent)}%`, metrics.streamPercent, `${metrics.failedAttemptCount} non-OK outcomes across ${metrics.attemptsCount} recent attempts.`, metrics.streamPercent >= 85 ? "good" : metrics.streamPercent >= 55 ? "warn" : "bad"),
    createSignalCard("Recorder Yield", recorder.summary ? `${clampPercent(metrics.recorderPercent)}%` : "n/a", metrics.recorderPercent, recorder.summary ? `${metrics.recorderCompleted} completed vs ${metrics.recorderFailed} failed.` : "Recorder state is not configured.", recorder.summary ? (metrics.recorderPercent >= 85 ? "good" : metrics.recorderPercent >= 55 ? "warn" : "bad") : "warn"),
    createSignalCard("Ops Cleanliness", `${clampPercent(metrics.opsPercent)}%`, metrics.opsPercent, metrics.opsRisk > 0 ? `${metrics.opsRisk} active ops recovery signals need attention.` : "No recorder/ghost recovery pressure in the current snapshot.", metrics.opsPercent >= 85 ? "good" : metrics.opsPercent >= 55 ? "warn" : "bad")
  ].join("");

  const trendSamples = state.sharedTelemetry.samples.length ? state.sharedTelemetry.samples : state.telemetry.samples;
  const firstSample = trendSamples.length ? trendSamples[0] : null;
  const lastSample = trendSamples.length ? trendSamples[trendSamples.length - 1] : null;
  historyBar.innerHTML = `
    <div>
      <strong>Deck Memory</strong>
      <div class="history-copy">${trendSamples.length} samples in ${runtime.webui?.memory_persisted ? "persisted" : "shared in-process"} web UI memory. ${firstSample ? `Oldest: ${esc(formatWhen(firstSample.sampled_at || firstSample.at))}.` : "No saved history yet."} ${lastSample ? `Latest: ${esc(formatWhen(lastSample.sampled_at || lastSample.at))}.` : ""} ${runtime.webui?.memory_persisted ? `State file: ${esc(pretty(runtime.webui?.state_file))}.` : "Restarting the web UI clears this history."}</div>
    </div>
    <div class="controls">
      <button id="history-reset" type="button">${runtime.webui?.memory_persisted ? "Clear Persisted Memory" : "Clear Shared Memory"}</button>
    </div>
  `;

  trendGrid.innerHTML = [
    createTrendCard("Guide Trend", `${clampPercent(metrics.guidePercent)}%`, trendSamples.map((item) => ({ value: item.guidePercent || item.guide_percent })), `${trendSamples.length} refresh samples in ${runtime.webui?.memory_persisted ? "persisted" : "shared"} deck memory.`),
    createTrendCard("Stream Trend", `${clampPercent(metrics.streamPercent)}%`, trendSamples.map((item) => ({ value: item.streamPercent || item.stream_percent })), `${metrics.failedAttemptCount} failures in the current attempt window.`),
    createTrendCard("Recorder Trend", recorder.summary ? `${clampPercent(metrics.recorderPercent)}%` : "n/a", trendSamples.map((item) => ({ value: item.recorderPercent || item.recorder_percent })), recorder.summary ? "Completion ratio across the current loaded recorder summary." : "Recorder not configured in this process."),
    createTrendCard("Ops Trend", `${clampPercent(metrics.opsPercent)}%`, trendSamples.map((item) => ({ value: item.opsPercent || item.ops_percent })), metrics.opsRisk > 0 ? "Recovery pressure is rising inside shared deck memory." : "Recovery pressure is flat or absent.")
  ].join("");

  decisionBoard.innerHTML = filterCards([
    createCard("Is the guide believable?", `${pretty(guideHealth.summary?.channels_with_real_programmes || guideHealth.summary?.channels_with_programmes)} channels have programme data; ${pretty(guideHealth.summary?.placeholder_only_channels)} are placeholder-only.`, "", (guideHealth.summary?.placeholder_only_channels || 0) > 0 ? "tone-warn" : "tone-good", "guideHealth", `${createWorkflowButton("guideWorkflow", "Guide Playbook")}${createActionButton("guide_refresh", "Refresh Now")}`),
    createCard("Are streams falling back?", `${attempts.length} recent attempts tracked; ${attempts.slice(0, 2).map((item) => item.reason || item.result || item.status || "unknown").join(" | ") || "no recent evidence"}`, "", failedAttempts.length > 0 ? "tone-warn" : "tone-good", "attempts", `${createWorkflowButton("streamWorkflow", "Investigate")}${createActionButton("stream_attempts_clear", "Clear History")}`),
    createCard("Any provider remediation hints?", remediationHints.length
      ? remediationHints.slice(0, 4).map((h) => `${h.severity || "info"} · ${h.code}: ${h.message || ""}`).join(" · ")
      : "No advisory hints from current provider counters (see /api/provider/profile.json).", "", remediationHints.some((h) => h && h.severity === "warn") ? "tone-warn" : "", "provider", `<button class="tiny" type="button" data-open-path="/api/provider/profile.json" data-open-title="/provider/profile.json">Open JSON</button>`),
    createCard("Is recording alive?", recorder.summary ? `${pretty(recorder.summary.active_count)} active, ${pretty(recorder.summary.failed_count)} failed, ${pretty(recorder.summary.completed_count)} completed.` : pretty(recorder.error || "No recorder state file configured"), "", (recorder.summary?.failed_count || 0) > 0 ? "tone-warn" : "", "recorder", `${createWorkflowButton("opsWorkflow", "Ops Recovery")}`),
    createCard("What config am I really running?", `Read-only snapshot exposed at /api/debug/runtime.json and summarized in the Settings lane.`, "", "", "runtime", `<button class="tiny" type="button" data-inspect="operatorActionsStatus">Action Status</button>`)
  ]).join("");

  statsEl.innerHTML = filterCards([
    createStat("Health", health.ok ? "ok" : "degraded", health.ok ? `${health.body.channels || 0} channels loaded` : pretty(health.body.error || health.body.status)),
    createStat("Guide Channels", guideHealth.summary?.total_channels || guideHealth.channel_count || 0, `${guideHealth.summary?.channels_with_real_programmes || guideHealth.summary?.channels_with_programmes || 0} carrying listings`),
    createStat("Programmes", epgStore.programme_count || guideDoctor.summary?.programme_count || "n/a", epgStore.source_ready ? "SQLite horizon active" : "merged cache only"),
    createStat("Recorder", recorder.summary?.active_count ?? recorder.active_count ?? "n/a", recorder.summary ? `${recorder.summary.completed_count || 0} completed` : "not configured")
  ]).join("");

  overviewStories.innerHTML = filterCards([
    createCard("Guide posture", `${pretty(guideHealth.summary?.channels_with_real_programmes || guideHealth.summary?.channels_with_programmes)} real programme channels, ${pretty(guideHealth.summary?.stale_channels)} stale, ${pretty(guideHealth.summary?.placeholder_only_channels)} placeholder-only.`, "Guide confidence from /guide/health.json", (guideHealth.summary?.stale_channels || 0) > 0 ? "tone-warn" : "tone-good", "guideHealth"),
    createCard("Provider posture", providerSummary ? esc(JSON.stringify(providerSummary)) : esc(pretty(providerLoadErr || "Profile unavailable")), "Runtime provider behavior from /provider/profile.json", "", "provider"),
    createCard("Channel posture", channelReport.summary ? `${pretty(channelReport.summary.total_channels)} total, ${pretty(channelReport.summary.epg_linked_channels)} linked, ${pretty(channelReport.summary.channels_with_backup_streams)} backup-capable.` : pretty(channelReport.error), "Channel intelligence summary", "", "channelReport"),
    createCard("Ops posture", recorder.summary ? `${pretty(recorder.summary.active_count)} active recordings with automation memory and publishing state available.` : pretty(recorder.error || "No recorder state"), "Recorder + automation surface", "", "recorder")
  ]).join("");

  overviewTimeline.innerHTML = filterCards([
    createTimeline("Operator activity", activity.slice(0, 2).map((item) => `${item.title}: ${item.message || item.kind || "event"}`).join(" | ") || "No operator activity recorded yet."),
    createTimeline("Latest runtime snapshot", esc(runtime.generated_at || "unknown")),
    createTimeline("Guide highlights", normalizeArray(guideHighlights.current).slice(0, 2).map((item) => `${item.channel_name || item.channel_id}: ${item.title}`).join(" | ") || "No live highlights"),
    createTimeline("Catch-up pressure", normalizeArray(capsules.capsules || capsules.items).slice(0, 2).map((item) => `${item.channel_name || item.channel_id}: ${item.title}`).join(" | ") || pretty(capsules.error || "No capsules")),
    createTimeline("Fallback trail", attempts.slice(0, 2).map((item) => `${item.channel_name || item.channel_id || "channel"} -> ${item.reason || item.result || item.status || "unknown"}`).join(" | ") || "No recent attempts")
  ]).join("");

  fastLanes.innerHTML = filterCards([
    createCard("Guide lane", "Guide health, doctor, highlights, capsules, and SQLite horizon in one place.", `<button class="tiny" type="button" data-mode-jump="guide">Open Guide</button>`),
    createCard("Routing lane", "Fallback evidence, provider posture, attempt trail, and mux behavior.", `<button class="tiny" type="button" data-mode-jump="routing">Open Routing</button>`),
    createCard("Operations lane", "Recorder status, autopilot memory, Plex ghost state, and media hooks.", `<button class="tiny" type="button" data-mode-jump="ops">Open Ops</button>`),
    createCard("Programming lane", "Category-first curation, manual ordering, and exact backup grouping for the exposed lineup.", `<button class="tiny" type="button" data-mode-jump="programming">Open Programming</button>`),
    createCard("Settings lane", "Effective runtime config, endpoint index, and raw payload drill-down.", `<button class="tiny" type="button" data-mode-jump="settings">Open Settings</button>`)
  ]).join("");

  guideList.innerHTML = filterCards([
    createCard("Guide health", `${pretty(guideHealth.summary?.channels_with_real_programmes || guideHealth.summary?.channels_with_programmes)} channels with real listings, ${pretty(guideHealth.summary?.placeholder_only_channels)} placeholder-only, ${pretty(guideHealth.summary?.stale_channels)} stale.`, "", (guideHealth.summary?.stale_channels || 0) > 0 ? "tone-warn" : "tone-good", "guideHealth", `${createWorkflowButton("guideWorkflow", "Guide Workflow")}${createActionButton("guide_refresh", "Refresh Guide")}`),
    createCard("Guide doctor", guideDoctor.summary ? `${pretty(guideDoctor.summary.channel_count)} channels inspected, ${pretty(guideDoctor.summary.programme_count)} programmes parsed.` : pretty(guideDoctor.error), "", "", "guideDoctor"),
    createCard("SQLite horizon", epgStore.source_ready ? `${pretty(epgStore.programme_count)} programmes, DB file ${pretty(epgStore.db_file_bytes)} bytes, max stop ${pretty(epgStore.global_max_stop_unix)}.` : pretty(epgStore.error || "SQLite disabled"), "", "", "epgStore", `<button class="tiny" type="button" data-inspect="guideWorkflow">Open Workflow Payload</button>`),
    createCard("Provider EPG mode", `enabled=${pretty(runtime.guide?.provider_epg_enabled)}, incremental=${pretty(runtime.guide?.provider_epg_incremental)}, disk_cache=${pretty(runtime.guide?.provider_epg_disk_cache_path)}`, "", "", "runtime"),
    createCard("Guide shaping", `xmltv=${pretty(runtime.guide?.xmltv_url)}, prune_unlinked=${pretty(runtime.tuner?.epg_prune_unlinked)}, suffix=${pretty(runtime.guide?.provider_epg_url_suffix)}`, "", "", "runtime")
  ]).join("");

  guideTimeline.innerHTML = filterCards([
    createTimeline("Current highlights", normalizeArray(guideHighlights.current).slice(0, 3).map((item) => `${item.channel_name || item.channel_id}: ${item.title}`).join(" | ") || "No current guide moments"),
    createTimeline("Starting soon", normalizeArray(guideHighlights.starting_soon || guideHighlights.movies_starting_soon).slice(0, 3).map((item) => `${item.channel_name || item.channel_id}: ${item.title}`).join(" | ") || "No starting-soon highlights"),
    createTimeline("Capsule queue", normalizeArray(capsules.capsules || capsules.items).slice(0, 3).map((item) => `${item.channel_name || item.channel_id}: ${item.title}`).join(" | ") || pretty(capsules.error || "No catch-up capsules")),
    createTimeline("Guide freshness", guideHealth.generated_at || guideDoctor.generated_at || runtime.generated_at || "unknown")
  ]).join("");

  routingList.innerHTML = filterCards([
    createCard("Provider profile", providerSummary ? esc(JSON.stringify(providerSummary)) : esc(pretty(providerLoadErr || "Profile unavailable")), (() => {
      const parts = [];
      if (remediationHints.length) parts.push(`remediation_hints=${remediationHints.length} (open JSON for full text)`);
      if (providerSummary && (providerSummary.upstream_quarantine_skips_total || 0) > 0) {
        parts.push(`upstream_quarantine_skips_total=${providerSummary.upstream_quarantine_skips_total}`);
      }
      if (provider.client_behavior) parts.push(`client_behavior=${esc(JSON.stringify(provider.client_behavior))}`);
      return parts.join(" · ");
    })(), "", "provider", `${createActionButton("provider_profile_reset")}`),
    createCard("Attempt volume", `${attempts.length} recent attempts in buffer. Top host pressure: ${attempts.slice(0, 3).map((item) => item.upstream_url_host || item.upstream_host || "unknown").join(", ") || "none"}`, "", failedAttempts.length > 0 ? "tone-warn" : "", "attempts", `${createWorkflowButton("streamWorkflow", "Failure Workflow")}${createWorkflowButton("diagnosticsWorkflow", "Capture Workflow")}${createActionButton("stream_attempts_clear")}`),
    createCard("Fallback evidence", attempts.slice(0, 4).map((item) => `${item.channel_name || item.channel_id || "channel"} -> ${item.reason || item.result || item.status || "unknown"}`).join(" | ") || "No fallback evidence", "", "", "attempts", `<button class="tiny" type="button" data-inspect="streamWorkflow">Workflow Payload</button>`),
    createCard("Diagnostics capture", diagnosticsWorkflow.summary?.suggested_bad_channel_id || diagnosticsWorkflow.summary?.suggested_good_channel_id
      ? `good=${pretty(diagnosticsWorkflow.summary?.suggested_good_channel_id)} · bad=${pretty(diagnosticsWorkflow.summary?.suggested_bad_channel_id)}`
      : "No good/bad channel suggestion yet from recent attempts.", "", "", "diagnosticsWorkflow", `${createWorkflowButton("diagnosticsWorkflow", "Open Diagnostics")}${createActionButton("channel_diff_run")}${createActionButton("stream_compare_run")}${createActionButton("evidence_intake_start")}`),
    createCard("Latest diagnostics", diagnosticRuns.length
      ? diagnosticRuns.map((run) => diagnosticsRunSummary(run)).filter(Boolean).join(" || ")
      : "No recent channel-diff, stream-compare, multi-stream, or evidence bundle runs detected under .diag.", "", "", "diagnosticsWorkflow", `<button class="tiny" type="button" data-inspect="diagnosticsWorkflow">Workflow Payload</button>`),
    createCard("HDHR contract", "discover.json, lineup.json, lineup_status.json, device.xml, and guide.xml still live on the tuner and are proxied here under /api.", "", "", "runtime"),
    createCard("Mux choices", "Native TS/fMP4/HLS surfaces exist; the deck is a supervisor over those capabilities, not a stream path itself.", "", "", "provider")
  ]).join("");

  attemptTrail.innerHTML = filterCards(attempts.slice(0, 8).map((item) =>
    createTimeline(
      item.channel_name || item.channel_id || "channel",
      `${item.upstream_url_host || item.upstream_host || "host?"} · ${item.reason || item.result || item.status || "unknown"}${item.profile ? ` · profile=${item.profile}` : ""}`
    )
  )).join("") || createTimeline("Attempts", "No recent stream attempts recorded.");

  opsList.innerHTML = filterCards([
    createCard("Ops recovery workflow", `${pretty(opsWorkflow.summary?.recorder?.failed_count || 0)} recorder failures, ${pretty(opsWorkflow.summary?.ghost?.stale_count || 0)} ghost-stale sessions, ${pretty(opsWorkflow.summary?.autopilot?.decision_count || 0)} autopilot decisions in memory.`, "", ((opsWorkflow.summary?.ghost?.stale_count || 0) + (opsWorkflow.summary?.recorder?.failed_count || 0)) > 0 ? "tone-warn" : "tone-good", "opsWorkflow", `${createWorkflowButton("opsWorkflow", "Open Ops Workflow")}`),
    createCard("Recorder state", recorder.summary ? `${pretty(recorder.summary.active_count)} active, ${pretty(recorder.summary.completed_count)} completed, ${pretty(recorder.summary.failed_count)} failed.` : pretty(recorder.error || "Recorder not configured"), "", (recorder.summary?.failed_count || 0) > 0 ? "tone-warn" : "", "recorder", `${createWorkflowButton("opsWorkflow", "Recovery Playbook")}`),
    createCard("Autopilot memory", autopilot.hot_channels ? `${autopilot.hot_channels.length} hot channels with remembered preferences.` : pretty(autopilot.error || "Autopilot memory unavailable"), formatAutopilotConsensusMeta(autopilot), "", "autopilot", `${createActionButton("autopilot_reset")}`),
    createCard("Ghost hunter", ghost.summary ? esc(JSON.stringify(ghost.summary)) : esc(pretty(ghost.error || "Plex report unavailable")), "", "", "ghost", `${createActionButton("ghost_visible_stop")}${createActionButton("ghost_hidden_recover_dry_run", "Dry-Run Recovery")}<button class="tiny" type="button" data-inspect="opsWorkflow">Workflow Payload</button>`),
    createCard("Operator activity log", activity.slice(0, 4).map((item) => `${formatWhen(item.at)} · ${item.title}${item.message ? ` · ${item.message}` : ""}`).join(" | ") || "No operator activity recorded yet.", "", "", "deckActivity", `<button class="tiny" id="activity-reset" type="button">Clear Activity</button>`),
    createCard("Media hooks", runtime.media_servers ? `Emby host=${pretty(runtime.media_servers.emby_host_configured)}, Jellyfin host=${pretty(runtime.media_servers.jellyfin_host_configured)}` : "No runtime snapshot", "", "", "runtime"),
    createCard("Recorder files", `state=${pretty(runtime.recorder?.state_file)}, autopilot=${pretty(runtime.tuner?.autopilot_state_file)}`, "", "", "runtime", `<button class="tiny" type="button" data-inspect="operatorActionsStatus">Control Status</button>`)
  ]).join("");

  channelList.innerHTML = filterCards([
    createCard("Channel report", channelReport.summary ? `${pretty(channelReport.summary.total_channels)} total, ${pretty(channelReport.summary.epg_linked_channels)} linked, ${pretty(channelReport.summary.channels_with_backup_streams)} backup-stream channels.` : pretty(channelReport.error), "", "", "channelReport"),
    createCard("Leaderboard", normalizeArray(leaderboard.channels || leaderboard.items || leaderboard).slice(0, 5).map((item) => `${item.name || item.channel_name || item.channel_id} (${item.score || item.health_score || "n/a"})`).join(" | ") || "No leaderboard data", "", "", "channelLeaderboard"),
    createCard("DNA grouping", `${pretty(dna.summary?.channel_count || dna.channel_count)} channels, ${pretty(dna.summary?.dna_group_count || dna.group_count)} grouped identities.`, "", "", "channelDNA"),
    createCard("What this means", "This lane is where duplicate-provider chaos becomes operator-readable channel structure.")
  ]).join("");

  const programmingDetailPayload = state.payloads.programmingChannelDetail?.body || {};
  const virtualChannelSchedulePayload = state.payloads.virtualChannelSchedule?.body || {};
  const programmingRecipe = programmingRecipePayload.recipe || programmingPreviewPayload.recipe || {};
  const programmingInventory = normalizeArray(programmingCategoriesPayload.categories || programmingPreviewPayload.inventory);
  const programmingBrowsePayload = state.payloads.programmingBrowse?.body || {};
  const programmingBrowseItems = normalizeArray(programmingBrowsePayload.items);
  const curatedLineup = normalizeArray(programmingPreviewPayload.lineup);
  const programmingLineupDescriptors = programmingPreviewPayload.lineup_descriptors || {};
  const backupGroups = normalizeArray(programmingBackupsPayload.groups || programmingPreviewPayload.backup_groups);
  const harvestPayload = state.payloads.programmingHarvest?.body || {};
  const harvestLineups = normalizeArray(harvestPayload.lineups || programmingPreviewPayload.harvest_lineups);
  const harvestAssists = normalizeArray(programmingHarvestAssistPayload.assists);
  const bucketEntries = Object.entries(programmingPreviewPayload.buckets || {});
  const selectedProgrammingChannel = findProgrammingChannel(curatedLineup, state.programmingSelectedChannelId) || programmingDetailPayload.Channel || null;
  const selectedProgrammingID = programmingChannelID(selectedProgrammingChannel);
  const selectedProgrammingName = selectedProgrammingChannel ? channelName(selectedProgrammingChannel) : "No channel selected";
  const virtualScheduleSlots = normalizeArray(virtualChannelSchedulePayload.report?.slots || virtualChannelSchedulePayload.slots);
  const topCategoryCards = programmingInventory.slice(0, 8).map((category) =>
    createCard(
      category.label || category.id,
      `${pretty(category.count)} channels · ${pretty(category.epg_linked_count)} linked · source ${pretty(category.source_tag || "mixed")}`,
      normalizeArray(category.sample_channels).length ? `Samples: ${normalizeArray(category.sample_channels).join(" · ")}` : "",
      "",
      "programmingCategories",
      `<div class="card-actions">${programmingCategoryButtons(category, programmingRecipe)}</div>`
    )
  );
  const browseCards = programmingBrowseItems.slice(0, 10).map((item) =>
    createCard(
      channelName(item),
      `${channelDescriptorLabel(item) || "Descriptor unavailable"} · guide ${pretty(item.guide_status || "unknown")} · next hour ${pretty(item.next_hour_programme_count)}`,
      `${pretty(item.guide_number)} · ${pretty(item.tvg_id || "no tvg")} · backups ${pretty(item.exact_backup_count)} · curated=${pretty(item.curated)} · included=${pretty(item.included)} · excluded=${pretty(item.excluded)}${normalizeArray(item.next_hour_titles).length ? ` · ${normalizeArray(item.next_hour_titles).slice(0, 2).join(" | ")}` : ""}`,
      item.has_real_guide_programmes ? "tone-good" : (item.has_guide_programmes ? "tone-warn" : ""),
      "programmingBrowse",
      `<div class="card-actions">${programmingChannelButtons(item, programmingRecipe)}${createTinyButton(`data-programming-select="${esc(programmingChannelID(item))}"`, "Detail")}${programmingStreamCompareButton(item)}</div>`
    )
  );

  programmingList.innerHTML = filterCards([
    createCard("Recipe posture", `File ${pretty(programmingPreviewPayload.recipe_file || programmingRecipePayload.recipe_file)} · writable=${pretty(programmingPreviewPayload.recipe_writable ?? programmingRecipePayload.recipe_writable)} · order=${pretty(programmingRecipe.order_mode)} · collapse_backups=${pretty(programmingRecipe.collapse_exact_backups)}`, "", "", "programmingRecipe",
      `<button class="tiny" type="button" data-programming-toggle="collapse_backups">${programmingRecipe.collapse_exact_backups ? "Disable Backup Collapse" : "Enable Backup Collapse"}</button>
       <button class="tiny" type="button" data-programming-mode="source">Source Order</button>
       <button class="tiny" type="button" data-programming-mode="recommended">Recommended Order</button>
       <button class="tiny" type="button" data-inspect="programmingRecipe">Inspect Recipe</button>`),
    createCard("Preview impact", `${pretty(programmingPreviewPayload.raw_channels)} raw channels -> ${pretty(programmingPreviewPayload.curated_channels)} curated channels. ${pretty(programmingInventory.length)} categories inventoried and ${pretty(backupGroups.length)} exact backup groups detected.`, "", "", "programmingPreview",
      `<button class="tiny" type="button" data-inspect="programmingPreview">Inspect Preview</button>`),
    createCard("Harvested lineup candidates", harvestLineups.length
      ? harvestLineups.slice(0, 6).map((row) => `${row.lineup_title} (${row.best_channelmap_rows || 0})`).join(" | ")
      : "No persisted Plex lineup harvest report configured yet.", "", "", "programmingHarvest",
      `${harvestLineups[0] ? programmingHarvestButtons(harvestLineups[0]) : ""} <button class="tiny" type="button" data-inspect="programmingHarvest">Inspect Harvest</button>`),
    createCard("Harvest assists", harvestAssists.length
      ? harvestAssists.slice(0, 4).map((row) => `${row.lineup_title}: ${row.recommendation_reason || `${row.matched_channels} matched`}`).join(" | ")
      : "No ranked harvest assists available yet.", "", "", "programmingHarvestAssist",
      `<button class="tiny" type="button" data-inspect="programmingHarvestAssist">Inspect Assists</button>`),
    createCard("Virtual schedule horizon", virtualScheduleSlots.length
      ? virtualScheduleSlots.slice(0, 4).map((slot) => `${slot.display_name || slot.rule_name || slot.channel_id}: ${slot.asset_title || slot.asset_id || "asset"} @ ${formatWhen(slot.starts_at || slot.startsAt)}`).join(" | ")
      : "No virtual-channel schedule is configured yet.", "", "", "virtualChannelSchedule",
      `<button class="tiny" type="button" data-inspect="virtualChannelSchedule">Inspect Schedule</button>`),
    createCard("Recommended buckets", bucketEntries.length
      ? bucketEntries.sort((a, b) => b[1] - a[1]).map(([bucket, count]) => `${bucket}: ${count}`).slice(0, 8).join(" | ")
      : "No bucket counts yet.", "", "", "programmingPreview"),
    createCard("Pinned channels", normalizeArray(programmingChannelsPayload.included_channels).length
      ? normalizeArray(programmingChannelsPayload.included_channels).slice(0, 12).join(" · ")
      : "No explicitly included channels yet.", "", "", "programmingChannels"),
    createCard("Blocked channels", normalizeArray(programmingChannelsPayload.excluded_channels).length
      ? normalizeArray(programmingChannelsPayload.excluded_channels).slice(0, 12).join(" · ")
      : "No explicitly excluded channels yet.", "", "", "programmingChannels")
  ]).join("");

  programmingCategories.innerHTML = filterCards(topCategoryCards).join("") || createCard("Categories", "No category inventory is available yet.", "", "", "programmingCategories");
  if (state.programmingSelectedCategoryId) {
    programmingCategories.innerHTML += filterCards([
      createCard(
        `Browse · ${programmingBrowsePayload.category_label || state.programmingSelectedCategoryId}`,
        programmingBrowseItems.length
          ? `${pretty(programmingBrowsePayload.total_channels)} channels in category · source_ready=${pretty(programmingBrowsePayload.source_ready)} · horizon ${pretty(programmingBrowsePayload.horizon)}`
          : pretty(programmingBrowsePayload.error || "No browse rows returned for the selected category."),
        "",
        "",
        "programmingCategories",
        `<button class="tiny" type="button" data-open-path="/api/programming/browse.json?category=${encodeURIComponent(state.programmingSelectedCategoryId)}&limit=24&horizon=1h" data-open-title="Programming Browse · ${esc(programmingBrowsePayload.category_label || state.programmingSelectedCategoryId)}">Inspect Browse</button>`
      ),
      ...browseCards
    ]).join("");
  }

  programmingPreview.innerHTML = filterCards(curatedLineup.slice(0, 12).map((channel) =>
    createCard(
      channelName(channel),
      `${channelDescriptorLabel(channel, programmingLineupDescriptors) || "Descriptor unavailable"} · streams ${pretty((channel.StreamURLs || channel.stream_urls || []).length || (channel.StreamURL || channel.stream_url ? 1 : 0))}`,
      `${pretty(channel.GuideNumber || channel.guide_number)} · ${pretty(channel.TVGID || channel.tvg_id || "no tvg")} · ${pretty(channel.SourceTag || channel.source_tag || "no source")} · ${pretty(channel.GroupTitle || channel.group_title || "Uncategorized")}`,
      programmingChannelID(channel) === state.programmingSelectedChannelId ? "tone-good" : "",
      "programmingPreview",
      `<div class="card-actions">${programmingSelectButtons(channel, curatedLineup, programmingRecipe)}</div>`
    )
  )).join("") || createCard("Preview", "No curated lineup preview is available yet.", "", "", "programmingPreview");

  programmingBackups.innerHTML = filterCards(backupGroups.slice(0, 10).map((group) =>
    createCard(
      group.display_name || group.key,
      `${pretty(group.member_count)} members · ${pretty(group.backup_count)} backup sources · ${pretty(group.match_strategy)}`,
      backupGroupSummary(group),
      "",
      "programmingBackups",
      `<button class="tiny" type="button" data-inspect="programmingBackups">Inspect Groups</button>`
    )
  )).join("") || createCard("Exact backups", "No strong same-channel sibling groups detected in the current preview.", "", "", "programmingBackups");
  if (harvestLineups.length) {
    programmingBackups.innerHTML += filterCards(harvestLineups.slice(0, 4).map((row) =>
      createCard(
        row.lineup_title || "lineup",
        `${pretty(row.successes)} successful target(s) · best channelmap ${pretty(row.best_channelmap_rows)}`,
        normalizeArray(row.friendly_names).join(" · "),
        "",
        "programmingHarvest",
        `<div class="card-actions">${programmingHarvestButtons(row)}</div>`
      )
    )).join("");
  }

  const previewCardBody = selectedProgrammingID
    ? `
      <div class="preview-shell">
        <div class="preview-meta">${esc(selectedProgrammingName)} · ${esc(channelDescriptorLabel(programmingDetailPayload.Channel || selectedProgrammingChannel, programmingLineupDescriptors) || "Descriptor unavailable")} · ${esc(pretty(selectedProgrammingChannel?.GuideNumber || selectedProgrammingChannel?.guide_number))}</div>
        <video id="programming-live-video" class="preview-video" controls autoplay muted playsinline data-stream-url="${esc(programmingPreviewURL(selectedProgrammingID))}"></video>
        <div id="programming-live-fallback" class="detail-chip" hidden>Loading preview…</div>
        <div class="card-actions">
          <button class="tiny" type="button" data-open-path="/api/programming/channel-detail.json?channel_id=${encodeURIComponent(selectedProgrammingID)}" data-open-title="Programming Detail · ${esc(selectedProgrammingName)}">Inspect Detail</button>
          <a class="tiny" href="${esc(programmingPreviewURL(selectedProgrammingID))}" target="_blank" rel="noreferrer">Open HLS Stream</a>
        </div>
      </div>`
    : "Select a curated channel to preview it in-place.";
  if ((programmingPlayer.dataset.channelId || "") !== selectedProgrammingID) {
    programmingPlayer.dataset.channelId = selectedProgrammingID;
    programmingPlayer.innerHTML = `<div class="card embed-card"><strong>${esc(selectedProgrammingID ? `Preview · ${selectedProgrammingName}` : "Preview unavailable")}</strong><div>${selectedProgrammingID ? "Tunerr-native HLS preview through the same gateway path Programming Manager will expose for this channel." : "No curated channel is selected yet."}</div>${selectedProgrammingID ? `<div class="meta">Channel ${esc(pretty(selectedProgrammingID))} · mux=hls</div>` : ""}${previewCardBody}</div>`;
  }

  const upcomingProgrammes = normalizeArray(programmingDetailPayload.UpcomingProgrammes).slice(0, 4);
  const alternativeSources = normalizeArray(programmingDetailPayload.AlternativeSources).slice(0, 5);
  const alternativeSourceButtons = alternativeSources.slice(0, 3).map((item) =>
    programmingChannelDiffButton(item, programmingDetailPayload.Channel || selectedProgrammingChannel, `Diff vs ${channelName(item)}`)
  ).filter(Boolean).join(" ");
  programmingDetail.innerHTML = [
    `<div class="detail-chip"><strong>${esc(selectedProgrammingID ? selectedProgrammingName : "No selected channel")}</strong><span>${esc(selectedProgrammingID ? `${channelDescriptorLabel(programmingDetailPayload.Channel || selectedProgrammingChannel, programmingLineupDescriptors) || "Descriptor unavailable"} · ${pretty(programmingDetailPayload.CategoryLabel || selectedProgrammingChannel?.GroupTitle || selectedProgrammingChannel?.group_title || "Uncategorized")} · bucket ${pretty(programmingDetailPayload.Bucket || "unknown")} · curated=${pretty(programmingDetailPayload.Curated)}` : "Choose a curated channel to inspect its detail and preview path.")}</span></div>`,
    `<div class="detail-chip"><strong>Upcoming programmes</strong><span>${esc(upcomingProgrammes.length ? upcomingProgrammes.map((item) => `${item.title || item.Title || "programme"} @ ${formatWhen(item.start || item.Start)}`).join(" | ") : (selectedProgrammingID ? "No upcoming guide rows available for this channel yet." : "No channel selected."))}</span></div>`,
    `<div class="detail-chip"><strong>Alternative sources</strong><span>${esc(alternativeSources.length ? alternativeSources.map((item) => `${channelName(item)} · ${channelDescriptorLabel(item) || pretty(item.SourceTag || item.source_tag || "source?")}${(item.descriptor?.variant || item.Descriptor?.Variant) ? ` · ${item.descriptor?.variant || item.Descriptor?.Variant}` : ""}`).join(" | ") : "No exact alternative sources detected for the current selection.")}</span></div>`,
    `<div class="detail-chip"><strong>Virtual schedule context</strong><span>${esc(virtualScheduleSlots.length ? virtualScheduleSlots.slice(0, 4).map((slot) => `${slot.display_name || slot.rule_name || slot.channel_id} · ${slot.asset_title || slot.asset_id || "asset"} @ ${formatWhen(slot.starts_at || slot.startsAt)}`).join(" | ") : "No virtual-channel schedule configured yet.")}</span></div>`,
    selectedProgrammingID ? `<div class="detail-chip"><strong>Diagnostics</strong><span>Run bounded capture straight from the current Programming selection.</span><div class="card-actions">${programmingStreamCompareButton(programmingDetailPayload.Channel || selectedProgrammingChannel)}${alternativeSourceButtons}</div></div>` : ""
  ].join("");

  settingsList.innerHTML = filterCards([
    createSettingsCard("Deck security posture", "Dedicated web UI auth, session, and persistence posture.", [
      ["Enabled", runtime.webui?.enabled],
      ["Port", runtime.webui?.port],
      ["Allow LAN", runtime.webui?.allow_lan],
      ["Auth user", deckSettings.auth_user || runtime.webui?.auth_user],
      ["Default password", deckSettings.auth_default_password ?? runtime.webui?.auth_default_password],
      ["CSRF header", runtime.webui?.csrf_header],
      ["Session TTL (min)", deckSettings.effective_session_ttl_minutes],
      ["Login failure limit", deckSettings.login_failure_limit],
      ["Failure window (min)", deckSettings.login_failure_window_minutes],
      ["Memory persisted", runtime.webui?.memory_persisted],
      ["State file", runtime.webui?.state_file]
    ], "deckSettings"),
    createSettingsCard("Tuner + transport", "Live-tuning and mux posture that shapes real playback behavior.", [
      ["Tuner count", runtime.tuner?.count],
      ["Lineup cap", runtime.tuner?.lineup_max_channels],
      ["Guide offset", runtime.tuner?.guide_number_offset],
      ["Transcode mode", runtime.tuner?.stream_transcode],
      ["Buffer bytes", runtime.tuner?.stream_buffer_bytes],
      ["Public base URL", runtime.tuner?.stream_public_base_url],
      ["HLS mux CORS", runtime.tuner?.hls_mux_cors],
      ["Metrics enabled", runtime.tuner?.metrics_enable],
      ["Fetch CF reject", runtime.tuner?.fetch_cf_reject]
    ], "runtime"),
    createSettingsCard("Guide pipeline", "Guide sources, cache posture, and durable horizon controls.", [
      ["XMLTV URL", runtime.guide?.xmltv_url],
      ["XMLTV timeout", runtime.guide?.xmltv_timeout],
      ["XMLTV cache TTL", runtime.guide?.xmltv_cache_ttl],
      ["Provider EPG enabled", runtime.guide?.provider_epg_enabled],
      ["Provider EPG incremental", runtime.guide?.provider_epg_incremental],
      ["Provider disk cache", runtime.guide?.provider_epg_disk_cache_path],
      ["EPG SQLite path", runtime.guide?.epg_sqlite_path],
      ["EPG SQLite incremental", runtime.guide?.epg_sqlite_incremental_upsert],
      ["EPG retain past hours", runtime.guide?.epg_sqlite_retain_past_hours],
      ["HDHR guide URL", runtime.guide?.hdhr_guide_url]
    ], "runtime"),
    createSettingsCard("Provider ingress", "Provider-side policy, backup posture, and source breadth.", [
      ["Entry count", runtime.provider?.entry_count],
      ["Base URLs", runtime.provider?.base_urls],
      ["Cloudflare block", runtime.provider?.block_cf_providers],
      ["Strip stream hosts", runtime.provider?.strip_stream_hosts],
      ["Smoketest enabled", runtime.provider?.smoketest_enabled],
      ["Smoketest timeout", runtime.provider?.smoketest_timeout],
      ["Free source mode", runtime.provider?.free_source_mode],
      ["Free source count", runtime.provider?.free_source_count]
    ], "runtime"),
    createSettingsCard("Programming Manager", "Saved recipe file, category inventory, ordering mode, and backup collapse posture.", [
      ["Recipe file", programmingPreviewPayload.recipe_file || programmingRecipePayload.recipe_file],
      ["Recipe writable", programmingPreviewPayload.recipe_writable ?? programmingRecipePayload.recipe_writable],
      ["Harvest file", harvestPayload.harvest_file || programmingPreviewPayload.harvest_file],
      ["Harvest ready", harvestPayload.report_ready ?? programmingPreviewPayload.harvest_ready],
      ["Order mode", programmingRecipe.order_mode],
      ["Selected categories", normalizeArray(programmingRecipe.selected_categories).length],
      ["Excluded categories", normalizeArray(programmingRecipe.excluded_categories).length],
      ["Included channels", normalizeArray(programmingRecipe.included_channel_ids).length],
      ["Excluded channels", normalizeArray(programmingRecipe.excluded_channel_ids).length],
      ["Collapse exact backups", programmingRecipe.collapse_exact_backups],
      ["Raw channels", programmingPreviewPayload.raw_channels],
      ["Curated channels", programmingPreviewPayload.curated_channels]
    ], "programmingRecipe", `<button class="tiny" type="button" data-mode-jump="programming">Open Programming</button>`),
    createSettingsCard("HDHR + media hooks", "Discovery/network mode plus downstream media-server hooks.", [
      ["HDHR network mode", runtime.hdhr?.network_mode],
      ["Device ID", runtime.hdhr?.device_id],
      ["Discover port", runtime.hdhr?.discover_port],
      ["Control port", runtime.hdhr?.control_port],
      ["Friendly name", runtime.hdhr?.friendly_name],
      ["Recorder state", runtime.recorder?.state_file],
      ["Autopilot state", runtime.tuner?.autopilot_state_file],
      ["Emby host configured", runtime.media_servers?.emby_host_configured],
      ["Jellyfin host configured", runtime.media_servers?.jellyfin_host_configured]
    ], "runtime"),
    createSettingsCard("Control surface atlas", "Actions, workflows, raw surfaces, and legacy operator lanes still exposed through the deck.", [
      ["Raw endpoints", Object.keys(endpoints).length],
      ["Available actions", Object.values(operatorStatus).filter((item) => item?.available).length],
      ["Guide workflow", endpoints.guideWorkflow],
      ["Stream workflow", endpoints.streamWorkflow],
      ["Diagnostics workflow", endpoints.diagnosticsWorkflow],
      ["Ops workflow", endpoints.opsWorkflow],
      ["Legacy UI", runtime.webui?.legacy_ui],
      ["Legacy LAN policy", runtime.webui?.legacy_lan]
    ], "operatorActionsStatus", `${createWorkflowButton("guideWorkflow", "Guide Playbook")}${createWorkflowButton("streamWorkflow", "Stream Playbook")}${createWorkflowButton("diagnosticsWorkflow", "Diagnostics")}${createWorkflowButton("opsWorkflow", "Ops Playbook")}`)
  ]).join("");

  state.deckSettings = deckSettings;
  renderDeckSettingsPanel(deckSettings);
}

async function postProgramming(path, payload, successAction) {
  try {
    const res = await fetch(path, {
      method: "POST",
      headers: authHeaders({
        "Accept": "application/json",
        "Content-Type": "application/json"
      }),
      body: JSON.stringify(payload)
    });
    const text = await res.text();
    const body = text ? JSON.parse(text) : {};
    if (!res.ok) {
      throw new Error(body.error || `HTTP ${res.status}`);
    }
    setActionFeedback({ ok: true, action: successAction, message: "Programming recipe updated." });
    await syncDeckActivity({
      kind: "programming",
      title: successAction,
      message: "Programming recipe updated."
    });
    await reloadDeck();
    applyMode("programming");
  } catch (err) {
    setActionFeedback({ ok: false, action: successAction, message: String(err) });
    renderDeck();
  }
}

async function renderRaw() {
  const key = rawSelect.value || state.selectedRaw;
  state.selectedRaw = key;
  persistPrefs();
  rawOutput.textContent = "Loading...";
  try {
    const payload = await fetchJSON(key, endpoints[key]);
    const text = JSON.stringify(payload.body, null, 2);
    rawOutput.textContent = text;
    if (state.modalView.kind === "raw" && state.modalView.key === key) {
      showModalRaw(text, `Endpoint Viewer · ${key}`, endpoints[key] || "Raw Payload");
    }
  } catch (err) {
    rawOutput.textContent = String(err);
    if (state.modalView.kind === "raw" && state.modalView.key === key) {
      showModalRaw(String(err), `Endpoint Viewer · ${key}`, endpoints[key] || "Raw Payload");
    }
  }
}

async function openAdHocPayload(path, title = "Endpoint Viewer", label = path) {
  state.modalView = { kind: "adhoc", key: path, title, label };
  openModal();
  showModalRaw("Loading...", title, label);
  try {
    const payload = await fetchJSON(title, path);
    showModalRaw(JSON.stringify(payload.body, null, 2), title, label);
  } catch (err) {
    showModalRaw(String(err), title, label);
  }
}

async function reloadDeck() {
  const entries = await Promise.all(Object.entries(endpoints).map(([key, path]) =>
    fetchJSON(key, path).catch((error) => ({ label: key, path, ok: false, status: 0, body: { error: String(error) } }))
  ));
  state.payloads = Object.fromEntries(entries.map((item) => [item.label, item]));
  const programmingInventory = normalizeArray(state.payloads.programmingCategories?.body?.categories || state.payloads.programmingPreview?.body?.inventory);
  state.programmingSelectedCategoryId = pickProgrammingCategorySelection(programmingInventory, state.programmingSelectedCategoryId);
  const curatedLineup = normalizeArray(state.payloads.programmingPreview?.body?.lineup);
  state.programmingSelectedChannelId = pickProgrammingSelection(curatedLineup, state.payloads.programmingChannelDetail?.body || {});
  persistPrefs();
  if (state.programmingSelectedCategoryId) {
    state.payloads.programmingBrowse = await fetchJSON(
      "programmingBrowse",
      `/api/programming/browse.json?category=${encodeURIComponent(state.programmingSelectedCategoryId)}&limit=24&horizon=1h`
    ).catch((error) => ({ label: "programmingBrowse", path: "", ok: false, status: 0, body: { error: String(error) } }));
  } else {
    delete state.payloads.programmingBrowse;
  }
  if (state.programmingSelectedChannelId) {
    state.payloads.programmingChannelDetail = await fetchJSON(
      "programmingChannelDetail",
      `/api/programming/channel-detail.json?channel_id=${encodeURIComponent(state.programmingSelectedChannelId)}&limit=6`
    ).catch((error) => ({ label: "programmingChannelDetail", path: "", ok: false, status: 0, body: { error: String(error) } }));
  } else {
    delete state.payloads.programmingChannelDetail;
  }
  await syncDeckTelemetry(deriveMetrics(state.payloads));
  renderDeck();
  await syncProgrammingPlayer();
  rawSelect.value = state.selectedRaw;
  await renderRaw();
}

async function postAction(actionKey, payload = null, confirmOverride = "") {
  const def = actionDefinitions[actionKey];
  if (!def) return;
  const confirmText = confirmOverride || def.confirm || "";
  if (confirmText && !window.confirm(confirmText)) return;
  setActionFeedback({ ok: undefined, action: actionKey, message: "Submitting operator action..." });
  renderDeck();
  try {
    const headers = authHeaders({ "Accept": "application/json" });
    const init = { method: "POST", headers };
    if (payload !== null && payload !== undefined) {
      headers["Content-Type"] = "application/json";
      init.body = JSON.stringify(payload);
    }
    const res = await fetch(def.path, init);
    const text = await res.text();
    let body;
    try {
      body = text ? JSON.parse(text) : {};
    } catch {
      body = { ok: res.ok, action: actionKey, message: text || `HTTP ${res.status}` };
    }
    if (body.ok === undefined) body.ok = res.ok;
    if (!body.action) body.action = actionKey;
    if (!body.message) body.message = text || `HTTP ${res.status}`;
    setActionFeedback(body);
    await syncDeckActivity({
      kind: "action",
      title: body.action || actionKey,
      message: body.message || `HTTP ${res.status}`,
      detail: { ok: !!body.ok }
    });
    await reloadDeck();
    state.modalView = { kind: "static", key: actionKey };
    openModal();
    showModalRaw(JSON.stringify(body, null, 2), `Action Result · ${actionKey}`, def.path);
  } catch (err) {
    setActionFeedback({ ok: false, action: actionKey, message: String(err) });
    renderDeck();
  }
}

async function saveDeckSettings() {
  const refreshEl = document.getElementById("deck-settings-refresh");
  if (!refreshEl) return;
  const payload = {
    default_refresh_sec: Number(refreshEl.value || 0)
  };
  try {
    const res = await fetch(endpoints.deckSettings, {
      method: "POST",
      headers: authHeaders({
        "Accept": "application/json",
        "Content-Type": "application/json"
      }),
      body: JSON.stringify(payload)
    });
    const text = await res.text();
    const body = text ? JSON.parse(text) : {};
    if (!res.ok) {
      throw new Error(body.error || `HTTP ${res.status}`);
    }
    state.deckSettings = body;
    if (!safeLocalStorage()?.getItem(storageKeys.prefs)) {
      state.refreshRateSec = Number(body.default_refresh_sec || state.refreshRateSec || 30);
      refreshRate.value = String(state.refreshRateSec);
      scheduleRefresh();
    }
    await syncDeckActivity({
      kind: "settings",
      title: "deck_settings_saved",
      message: "Deck preferences updated.",
      detail: { default_refresh_sec: body.default_refresh_sec }
    });
    setActionFeedback({ ok: true, action: "deck_settings", message: "Deck preferences saved." });
    await reloadDeck();
  } catch (err) {
    setActionFeedback({ ok: false, action: "deck_settings", message: String(err) });
    renderDeck();
  }
}

async function refreshModal() {
  if (state.modalView.kind === "workflow") {
    await reloadDeck();
    renderWorkflowModal(state.modalView.key);
    return;
  }
  if (state.modalView.kind === "adhoc") {
    await openAdHocPayload(state.modalView.key, state.modalView.title, state.modalView.label);
    return;
  }
  if (state.modalView.kind === "static") {
    return;
  }
  await renderRaw();
  openModal();
}

function bindUI() {
  loadPersistedState();
  fillRawSelectors();
  refreshRate.value = String(state.refreshRateSec);
  document.querySelectorAll("#mode-nav button").forEach((button) => {
    button.addEventListener("click", () => applyMode(button.dataset.mode));
  });
  document.body.addEventListener("click", (event) => {
    const inspect = event.target.closest("[data-inspect]");
    if (inspect) {
      state.selectedRaw = inspect.getAttribute("data-inspect");
      rawSelect.value = state.selectedRaw;
      state.modalView = { kind: "raw", key: state.selectedRaw };
      renderRaw();
      openModal();
      return;
    }
    const action = event.target.closest("[data-action]");
    if (action) {
      let payload = null;
      const rawPayload = action.getAttribute("data-action-payload");
      if (rawPayload) {
        try {
          payload = JSON.parse(rawPayload);
        } catch (error) {
          setActionFeedback({ ok: false, action: action.getAttribute("data-action"), message: `invalid action payload: ${error}` });
          renderDeck();
          return;
        }
      }
      const confirmText = action.getAttribute("data-action-confirm") || "";
      postAction(action.getAttribute("data-action"), payload, confirmText);
      return;
    }
    const workflow = event.target.closest("[data-workflow]");
    if (workflow) {
      renderWorkflowModal(workflow.getAttribute("data-workflow"));
      return;
    }
    const openPath = event.target.closest("[data-open-path]");
    if (openPath) {
      openAdHocPayload(openPath.getAttribute("data-open-path"), openPath.getAttribute("data-open-title") || "Endpoint Viewer", openPath.getAttribute("data-open-path"));
      return;
    }
    const jump = event.target.closest("[data-mode-jump]");
    if (jump) {
      applyMode(jump.getAttribute("data-mode-jump"));
      return;
    }
    const programmingPost = event.target.closest("[data-programming-post]");
    if (programmingPost) {
      const kind = programmingPost.getAttribute("data-programming-post");
      const action = programmingPost.getAttribute("data-programming-action") || "";
      const id = programmingPost.getAttribute("data-programming-id") || "";
      const anchor = programmingPost.getAttribute("data-programming-anchor") || "";
      if (kind === "category") {
        postProgramming(endpoints.programmingCategories, { action, category_id: id }, `programming_category_${action}`);
        return;
      }
      if (kind === "channel") {
        postProgramming(endpoints.programmingChannels, { action, channel_id: id }, `programming_channel_${action}`);
        return;
      }
      if (kind === "order") {
        const payload = { action, channel_id: id };
        if (action === "before") payload.before_channel_id = anchor;
        if (action === "after") payload.after_channel_id = anchor;
        postProgramming(endpoints.programmingOrder, payload, `programming_order_${action}`);
        return;
      }
    }
    const programmingToggle = event.target.closest("[data-programming-toggle]");
    if (programmingToggle) {
      const recipe = state.payloads.programmingRecipe?.body?.recipe || state.payloads.programmingPreview?.body?.recipe || {};
      if (programmingToggle.getAttribute("data-programming-toggle") === "collapse_backups") {
        postProgramming(endpoints.programmingRecipe, {
          ...recipe,
          collapse_exact_backups: !recipe.collapse_exact_backups
        }, "programming_toggle_collapse_backups");
        return;
      }
    }
    const programmingMode = event.target.closest("[data-programming-mode]");
    if (programmingMode) {
      const recipe = state.payloads.programmingRecipe?.body?.recipe || state.payloads.programmingPreview?.body?.recipe || {};
      postProgramming(endpoints.programmingRecipe, {
        ...recipe,
        order_mode: programmingMode.getAttribute("data-programming-mode")
      }, "programming_order_mode");
      return;
    }
    const programmingHarvestImport = event.target.closest("[data-programming-harvest-import]");
    if (programmingHarvestImport) {
      postProgramming(endpoints.programmingHarvestImport, {
        lineup_title: programmingHarvestImport.getAttribute("data-programming-harvest-import") || "",
        replace: true,
        collapse_exact_backups: programmingHarvestImport.getAttribute("data-programming-harvest-collapse") === "1"
      }, "programming_harvest_import");
      return;
    }
    const programmingSelect = event.target.closest("[data-programming-select]");
    if (programmingSelect) {
      state.programmingSelectedChannelId = programmingSelect.getAttribute("data-programming-select") || "";
      persistPrefs();
      reloadDeck();
      return;
    }
    const programmingCategorySelect = event.target.closest("[data-programming-category-select]");
    if (programmingCategorySelect) {
      state.programmingSelectedCategoryId = programmingCategorySelect.getAttribute("data-programming-category-select") || "";
      persistPrefs();
      reloadDeck();
      return;
    }
    const rawButton = event.target.closest("[data-select-raw]");
    if (rawButton) {
      state.selectedRaw = rawButton.getAttribute("data-select-raw");
      rawSelect.value = state.selectedRaw;
      state.modalView = { kind: "raw", key: state.selectedRaw };
      renderRaw();
      openModal();
    }
  });
  document.getElementById("reload-all").addEventListener("click", reloadDeck);
  document.getElementById("open-modal").addEventListener("click", () => {
    state.modalView = { kind: "raw", key: state.selectedRaw };
    openModal();
    renderRaw();
  });
  document.getElementById("sign-out").addEventListener("click", () => {
    fetch("/logout", { method: "POST", headers: authHeaders({ "Accept": "text/html" }) })
      .finally(() => {
        window.location.assign("/login");
      });
  });
  document.getElementById("modal-close").addEventListener("click", closeModal);
  document.getElementById("modal-refresh").addEventListener("click", refreshModal);
  rawSelect.addEventListener("change", renderRaw);
  refreshRate.addEventListener("change", (event) => {
    state.refreshRateSec = Number(event.target.value || 0);
    scheduleRefresh();
    persistPrefs();
    renderFilters();
  });
  document.getElementById("deck-search").addEventListener("input", (event) => {
    state.filter = event.target.value || "";
    renderFilters();
    renderDeck();
  });
  rawModal.addEventListener("click", (event) => {
    if (event.target === rawModal) closeModal();
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") closeModal();
  });
  document.addEventListener("click", (event) => {
    if (event.target && event.target.id === "history-reset") {
      clearDeckTelemetry();
      return;
    }
    if (event.target && event.target.id === "activity-reset") {
      clearDeckActivity();
      return;
    }
    if (event.target && event.target.id === "deck-settings-save") {
      saveDeckSettings();
    }
  });
  window.deckMode = (mode) => applyMode(mode);
}

async function boot() {
  bindUI();
  applyMode(state.mode);
  persistPrefs();
  scheduleRefresh();
  await reloadDeck();
}

boot().catch((err) => {
  setHealth(false, "Load failed");
  rawOutput.textContent = String(err);
  modalOutput.textContent = String(err);
});
