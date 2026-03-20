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
  operatorActionsStatus: "/api/ops/actions/status.json",
  guideWorkflow: "/api/ops/workflows/guide-repair.json",
  streamWorkflow: "/api/ops/workflows/stream-investigate.json",
  opsWorkflow: "/api/ops/workflows/ops-recovery.json"
};
const csrfHeaderName = "X-IPTVTunerr-Deck-CSRF";
const endpointCatalog = {
  deckTelemetry: { title: "Deck Telemetry", category: "Deck Memory", summary: "Shared trend samples across reloads and browsers." },
  deckActivity: { title: "Deck Activity", category: "Deck Memory", summary: "Shared operator activity trail for the dedicated deck." },
  deckSettings: { title: "Deck Settings", category: "Deck Control", summary: "Editable login user and default refresh cadence for the dedicated deck." },
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
  operatorActionsStatus: { title: "Operator Action Status", category: "Deck Control", summary: "Availability and current status of safe operator actions." },
  guideWorkflow: { title: "Guide Workflow", category: "Workflows", summary: "Guided checklist for guide repair and freshness issues." },
  streamWorkflow: { title: "Stream Workflow", category: "Workflows", summary: "Guided lane for routing and upstream stream failures." },
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
  }
};

const modeTitles = {
  overview: ["Overview", "Live State"],
  guide: ["Guide", "Integrity"],
  routing: ["Routing", "Decision Trail"],
  ops: ["Operations", "Automation"],
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

function renderDeckSettingsPanel(settings) {
  const refreshValue = Number(settings?.default_refresh_sec ?? state.refreshRateSec ?? 30);
  deckSettingsForm.innerHTML = `
    <div class="deck-settings-grid">
      <label>
        Deck Username
        <input id="deck-settings-user" type="text" value="${esc(settings?.auth_user || "admin")}" autocomplete="username">
      </label>
      <label>
        New Deck Password
        <input id="deck-settings-pass" type="password" value="" placeholder="Leave blank to keep current password" autocomplete="new-password">
      </label>
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
      <button id="deck-settings-save" type="button">Save Deck Settings</button>
    </div>
    <div class="deck-settings-note">
      Session TTL: ${esc(pretty(settings?.effective_session_ttl_minutes))} minutes. Login rate limit: ${esc(pretty(settings?.login_failure_limit))} failures per ${esc(pretty(settings?.login_failure_window_minutes))} minutes. ${settings?.state_persisted ? "Changes persist across restarts." : "Without a web UI state file, changes last only until this process restarts."}
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
      selectedRaw: state.selectedRaw
    }));
  } catch {}
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
  const sample = {
    sampled_at: new Date().toISOString(),
    health_ok: metrics.healthOK,
    guide_percent: metrics.guidePercent,
    stream_percent: metrics.streamPercent,
    recorder_percent: metrics.recorderPercent,
    ops_percent: metrics.opsPercent
  };
  try {
    const res = await fetch(endpoints.deckTelemetry, {
      method: "POST",
      headers: authHeaders({
        "Accept": "application/json",
        "Content-Type": "application/json"
      }),
      body: JSON.stringify(sample)
    });
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
  try {
    const res = await fetch(endpoints.deckActivity, {
      method: "POST",
      headers: authHeaders({
        "Accept": "application/json",
        "Content-Type": "application/json"
      }),
      body: JSON.stringify(entry)
    });
    const text = await res.text();
    const body = text ? JSON.parse(text) : {};
    state.activity.entries = normalizeArray(body.entries);
  } catch {
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
  const operatorStatus = state.payloads.operatorActionsStatus?.body || {};
  const guideWorkflow = state.payloads.guideWorkflow?.body || {};
  const streamWorkflow = state.payloads.streamWorkflow?.body || {};
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

  const wins = [];
  if (health.ok) wins.push("Deck can reach the tuner through the dedicated proxy origin.");
  if ((guideHealth.summary?.channels_with_real_programmes || guideHealth.summary?.channels_with_programmes || 0) > 0) wins.push("Guide payload contains real programme coverage.");
  if ((channelReport.summary?.channels_with_backup_streams || 0) > 0) wins.push("Channel catalog includes backup-capable streams.");
  if (runtime.generated_at) wins.push("Runtime snapshot is available for settings auditing.");
  if (remediationHints.length === 0 && providerSummary) wins.push("No advisory provider remediation hints in the current profile snapshot.");
  if (providerSummary?.autopilot_consensus_host) {
    wins.push(`Autopilot reports a consensus host (${providerSummary.autopilot_consensus_host}) across multiple channel memories.`);
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
    createCard("Provider profile", providerSummary ? esc(JSON.stringify(providerSummary)) : esc(pretty(providerLoadErr || "Profile unavailable")), remediationHints.length ? `remediation_hints=${remediationHints.length} (open JSON for full text)` : (provider.client_behavior ? `client_behavior=${esc(JSON.stringify(provider.client_behavior))}` : ""), "", "provider", `${createActionButton("provider_profile_reset")}`),
    createCard("Attempt volume", `${attempts.length} recent attempts in buffer. Top host pressure: ${attempts.slice(0, 3).map((item) => item.upstream_url_host || item.upstream_host || "unknown").join(", ") || "none"}`, "", failedAttempts.length > 0 ? "tone-warn" : "", "attempts", `${createWorkflowButton("streamWorkflow", "Failure Workflow")}${createActionButton("stream_attempts_clear")}`),
    createCard("Fallback evidence", attempts.slice(0, 4).map((item) => `${item.channel_name || item.channel_id || "channel"} -> ${item.reason || item.result || item.status || "unknown"}`).join(" | ") || "No fallback evidence", "", "", "attempts", `<button class="tiny" type="button" data-inspect="streamWorkflow">Workflow Payload</button>`),
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
    createCard("Ghost hunter", ghost.summary ? esc(JSON.stringify(ghost.summary)) : esc(pretty(ghost.error || "Plex report unavailable")), "", "", "ghost", `<button class="tiny" type="button" data-inspect="opsWorkflow">Workflow Payload</button>`),
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
      ["Ops workflow", endpoints.opsWorkflow],
      ["Legacy UI", runtime.webui?.legacy_ui],
      ["Legacy LAN policy", runtime.webui?.legacy_lan]
    ], "operatorActionsStatus", `${createWorkflowButton("guideWorkflow", "Guide Playbook")}${createWorkflowButton("streamWorkflow", "Stream Playbook")}${createWorkflowButton("opsWorkflow", "Ops Playbook")}`)
  ]).join("");

  state.deckSettings = deckSettings;
  renderDeckSettingsPanel(deckSettings);
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
  await syncDeckTelemetry(deriveMetrics(state.payloads));
  renderDeck();
  rawSelect.value = state.selectedRaw;
  await renderRaw();
}

async function postAction(actionKey) {
  const def = actionDefinitions[actionKey];
  if (!def) return;
  if (def.confirm && !window.confirm(def.confirm)) return;
  setActionFeedback({ ok: undefined, action: actionKey, message: "Submitting operator action..." });
  renderDeck();
  try {
    const res = await fetch(def.path, { method: "POST", headers: authHeaders({ "Accept": "application/json" }) });
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
  const userEl = document.getElementById("deck-settings-user");
  const passEl = document.getElementById("deck-settings-pass");
  const refreshEl = document.getElementById("deck-settings-refresh");
  if (!userEl || !passEl || !refreshEl) return;
  const payload = {
    auth_user: userEl.value.trim(),
    auth_pass: passEl.value,
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
      message: `Deck controls updated for ${body.auth_user}.`,
      detail: { default_refresh_sec: body.default_refresh_sec }
    });
    setActionFeedback({ ok: true, action: "deck_settings", message: "Deck settings saved." });
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
      postAction(action.getAttribute("data-action"));
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
