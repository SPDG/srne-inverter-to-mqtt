const REFRESH_INTERVAL_MS = 5_000;

const fmt = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 });
let latestStatus = null;
let latestConfig = null;
let refreshTimer = null;
let haYamlMode = localStorage.getItem("srneHaYamlMode") || "dashboard";
let rawDataItems = [];
let rawDataPaused = false;
const controlDrafts = new Map();

const els = {
  rail: document.getElementById("rail"),
  railToggle: document.getElementById("rail-toggle"),
  deviceTitle: document.getElementById("device-title"),
  summary: document.getElementById("summary"),
  topStatus: document.getElementById("top-status"),
  refreshStatus: document.getElementById("refresh-status"),
  powerOverview: document.getElementById("power-overview"),
  services: document.getElementById("services"),
  heroKpiGrid: document.getElementById("hero-kpi-grid"),
  energyGrid: document.getElementById("energy-grid"),
  telemetryGrid: document.getElementById("telemetry-grid"),
  telemetrySubtitle: document.getElementById("telemetry-subtitle"),
  rawDataSearch: document.getElementById("raw-data-search"),
  rawDataClear: document.getElementById("raw-data-clear"),
  rawDataPause: document.getElementById("raw-data-pause"),
  rawDataCount: document.getElementById("raw-data-count"),
  rawDataBody: document.getElementById("raw-data-body"),
  controlsGrid: document.getElementById("controls-grid"),
  maintenanceSection: document.getElementById("maintenance-section"),
  maintenanceGrid: document.getElementById("maintenance-grid"),
  writeResult: document.getElementById("write-result"),
  stormChargePanel: document.getElementById("storm-charge-panel"),
  stormChargeBadge: document.getElementById("storm-charge-badge"),
  stormChargeTitle: document.getElementById("storm-charge-title"),
  stormChargeDetail: document.getElementById("storm-charge-detail"),
  stormChargeStats: document.getElementById("storm-charge-stats"),
  stormChargeForm: document.getElementById("storm-charge-form"),
  stormTargetSOC: document.getElementById("storm-target-soc"),
  stormMaxCurrent: document.getElementById("storm-max-current"),
  stormTimeoutMinutes: document.getElementById("storm-timeout-minutes"),
  stormPowerPreview: document.getElementById("storm-power-preview"),
  stormPowerDetail: document.getElementById("storm-power-detail"),
  stormChargeStart: document.getElementById("storm-charge-start"),
  stormChargeCancel: document.getElementById("storm-charge-cancel"),
  stormChargeResult: document.getElementById("storm-charge-result"),
  inverterClockForm: document.getElementById("inverter-clock-form"),
  inverterClockInput: document.getElementById("inverter-clock-input"),
  inverterClockCurrent: document.getElementById("inverter-clock-current"),
  inverterClockTime: document.getElementById("inverter-clock-time"),
  inverterClockDate: document.getElementById("inverter-clock-date"),
  inverterClockStatus: document.getElementById("inverter-clock-status"),
  inverterClockResult: document.getElementById("inverter-clock-result"),
  refreshInverterClock: document.getElementById("refresh-inverter-clock"),
  useBrowserClock: document.getElementById("use-browser-clock"),
  haYaml: document.getElementById("ha-yaml"),
  settingsRuntime: document.getElementById("settings-runtime"),
  serialPorts: document.getElementById("serial-ports"),
  saveResult: document.getElementById("save-result"),
  refreshPorts: document.getElementById("refresh-ports"),
  reloadConfig: document.getElementById("reload-config"),
  configForm: document.getElementById("config-form"),
  deviceName: document.getElementById("device-name"),
  deviceSlaveID: document.getElementById("device-slave-id"),
  deviceInverterType: document.getElementById("device-inverter-type"),
  serialPort: document.getElementById("serial-port"),
  serialNetworkProtocol: document.getElementById("serial-network-protocol"),
  serialBaudRate: document.getElementById("serial-baud-rate"),
  serialParity: document.getElementById("serial-parity"),
  serialTimeout: document.getElementById("serial-timeout"),
  mqttBroker: document.getElementById("mqtt-broker"),
  mqttUsername: document.getElementById("mqtt-username"),
  mqttPassword: document.getElementById("mqtt-password"),
  mqttClientID: document.getElementById("mqtt-client-id"),
  mqttTopicPrefix: document.getElementById("mqtt-topic-prefix"),
  mqttDiscoveryPrefix: document.getElementById("mqtt-discovery-prefix"),
  httpListen: document.getElementById("http-listen"),
  loggingLevel: document.getElementById("logging-level"),
  mqttRetain: document.getElementById("mqtt-retain"),
};

async function fetchJSON(url, options = {}) {
  const response = await fetch(url, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload.error || `Request failed: ${response.status}`);
  }
  return payload;
}

async function loadStatus({ forceControls = false } = {}) {
  const status = await fetchJSON("/api/v1/status");
  latestStatus = status;
  renderStatus(status, { forceControls });
}

async function loadPorts() {
  els.serialPorts.innerHTML = "<li>Scanning...</li>";
  const ports = await fetchJSON("/api/v1/serial/ports");
  renderPorts(ports);
}

async function loadConfig() {
  const cfg = await fetchJSON("/api/v1/config");
  fillConfigForm(cfg);
}

async function loadInverterClock() {
  renderInverterClockLoading();
  try {
    const value = await fetchJSON("/api/v1/inverter/clock");
    renderInverterClock(value);
  } catch (error) {
    renderInverterClockError(error);
  }
}

function renderInverterClockLoading() {
  els.inverterClockTime.textContent = "--:--:--";
  els.inverterClockDate.textContent = "Reading inverter clock...";
  els.inverterClockCurrent.classList.remove("invalid");
  els.inverterClockStatus.textContent = "Reading";
  els.inverterClockStatus.className = "status-pill clock-reading-status";
}

function renderInverterClock(value) {
  const pad = (part) => String(part).padStart(2, "0");
  const valid = Boolean(value.valid);
  els.inverterClockTime.textContent = valid
    ? `${pad(value.hour)}:${pad(value.minute)}:${pad(value.second)}`
    : "Invalid clock";
  els.inverterClockDate.textContent = valid
    ? `${value.year}-${pad(value.month)}-${pad(value.day)}`
    : value.formatted || "The inverter returned an invalid date or time.";
  els.inverterClockCurrent.classList.toggle("invalid", !valid);
  els.inverterClockStatus.textContent = valid ? "Clock ready" : "Invalid clock";
  els.inverterClockStatus.className = `status-pill ${valid ? "ok" : "bad"}`;
}

function renderInverterClockError(error) {
  els.inverterClockTime.textContent = "Read failed";
  els.inverterClockDate.textContent = error.message;
  els.inverterClockCurrent.classList.add("invalid");
  els.inverterClockStatus.textContent = "Unavailable";
  els.inverterClockStatus.className = "status-pill bad";
}

function localDateTimeInputValue(value = new Date()) {
  const pad = (part) => String(part).padStart(2, "0");
  return `${value.getFullYear()}-${pad(value.getMonth() + 1)}-${pad(value.getDate())}T${pad(value.getHours())}:${pad(value.getMinutes())}:${pad(value.getSeconds())}`;
}

async function setInverterClock(event) {
  event.preventDefault();
  const dateTime = els.inverterClockInput.value;
  if (!dateTime) {
    return;
  }
  els.inverterClockResult.textContent = "Writing and verifying clock...";
  try {
    const value = await fetchJSON("/api/v1/inverter/clock", {
      method: "POST",
      body: JSON.stringify({ dateTime }),
    });
    renderInverterClock(value);
    els.inverterClockResult.textContent = "Inverter clock updated.";
  } catch (error) {
    els.inverterClockResult.textContent = error.message;
  }
}

function renderStatus(status, { forceControls = false } = {}) {
  const telemetry = status.telemetry || [];
  const byId = Object.fromEntries(telemetry.map((item) => [item.id, item]));

  renderHeader(status, byId);
  renderServices(status.service || {});
  renderPowerOverview(byId);
  renderHeroKpis(byId);
  renderEnergy(byId);
  renderStormCharge(status.stormCharge || {}, byId);
  renderTelemetry(telemetry);
  if (!rawDataPaused) {
    renderRawData(telemetry);
  }
  reconcileControlDrafts(telemetry);
  if (forceControls || (!isControlInteractionActive() && controlDrafts.size === 0)) {
    renderControls(telemetry);
  }
  renderSettings(status);
  renderHAConfig(status);
}

function renderRawData(items) {
  if (Array.isArray(items)) {
    rawDataItems = [...items].sort((left, right) => {
      const addressDifference = Number(left.address) - Number(right.address);
      return addressDifference || String(left.id).localeCompare(String(right.id));
    });
  }

  const query = String(els.rawDataSearch?.value || "").trim().toLowerCase();
  const filtered = query
    ? rawDataItems.filter((item) => rawSearchText(item).includes(query))
    : rawDataItems;

  const countText = query
    ? `${filtered.length} of ${rawDataItems.length} parameters`
    : `${rawDataItems.length} parameters`;
  els.rawDataCount.textContent = rawDataPaused ? `${countText} · paused` : countText;
  els.rawDataCount.classList.toggle("paused", rawDataPaused);

  if (!filtered.length) {
    const message = rawDataItems.length ? "No parameters match this search." : "Waiting for Modbus data.";
    els.rawDataBody.innerHTML = `<tr><td colspan="10" class="raw-empty">${escapeHTML(message)}</td></tr>`;
    return;
  }

  els.rawDataBody.innerHTML = filtered.map((item) => {
    const address = item.synthetic ? "-" : `0x${Number(item.address).toString(16).padStart(4, "0").toUpperCase()}`;
    const source = item.synthetic ? "Computed" : item.writeOnly ? "Control" : "Modbus";
    const sourceClass = `raw-source ${source.toLowerCase()}`;
    const access = item.writeOnly ? "Write only" : item.writable ? "Read / Write" : "Read only";
    const accessClass = item.writable ? "raw-access writable" : "raw-access";
    return `
      <tr>
        <td><code class="raw-address">${escapeHTML(address)}</code></td>
        <td class="raw-name">${escapeHTML(item.name)}</td>
        <td><code>${escapeHTML(item.id)}</code></td>
        <td class="raw-value">${escapeHTML(String(item.rendered ?? "-"))}</td>
        <td><code>${escapeHTML(rawValueText(item))}</code></td>
        <td>${escapeHTML(item.unit || "-")}</td>
        <td><span class="raw-group">${escapeHTML(item.group || "-")}</span></td>
        <td><span class="${sourceClass}">${source}</span></td>
        <td><span class="${accessClass}">${access}</span></td>
        <td class="raw-updated">${escapeHTML(formatUpdatedAt(item.updatedAt))}</td>
      </tr>
    `;
  }).join("");
}

function rawValueText(item) {
  if (item.synthetic) {
    return "-";
  }
  if (Array.isArray(item.rawWords) && item.rawWords.length > 1) {
    return item.rawWords.map((word) => `0x${Number(word).toString(16).padStart(4, "0").toUpperCase()}`).join(" ");
  }
  return String(item.raw ?? "-");
}

function rawSearchText(item) {
  const address = Number(item.address);
  return [
    item.name,
    item.id,
    item.rendered,
    item.raw,
    item.value,
    item.unit,
    item.group,
    item.synthetic ? "computed calculated derived application" : item.writeOnly ? "control" : "modbus register",
    item.writable ? "read write writable" : "read only",
    String(address),
    `0x${address.toString(16).padStart(4, "0")}`,
  ].map((value) => String(value ?? "").toLowerCase()).join(" ");
}

function renderStormCharge(storm, byId) {
  if (!els.stormChargePanel) {
    return;
  }
  const supported = latestStatus?.device?.inverterType === "spi_h3p";
  els.stormChargePanel.hidden = !supported;
  if (!supported) {
    return;
  }
  const active = Boolean(storm.active);
  const phase = String(storm.phase || "idle");
  const settings = storm.settings || {};
  const voltage = Number(storm.batteryVoltage || byId.battery_voltage?.value || 0);
  const currentSOC = Number(storm.currentSoc || byId.battery_soc?.value || 0);
  const phaseLabel = phase.replaceAll("_", " ");

  els.stormChargePanel.classList.toggle("active", active);
  els.stormChargeBadge.className = `status-pill ${active ? "storm-active" : phase === "error" ? "bad" : ""}`;
  els.stormChargeBadge.textContent = phaseLabel;
  els.stormChargeTitle.textContent = active ? `Charging to ${settings.targetSoc}%` : stormPhaseTitle(phase);
  els.stormChargeDetail.textContent = active
    ? `${storm.remaining || "-"} remaining before the safety timeout.`
    : storm.reason ? `Last result: ${storm.reason.replaceAll("_", " ")}.` : "Choose a target, current limit and timeout.";

  setDefinitionList("storm-charge-stats", [
    ["Battery", `${fmt.format(currentSOC)}% at ${fmt.format(voltage)} V`],
    ["Target", `${settings.targetSoc ?? "-"}%`],
    ["Limit", `${settings.maxCurrentA ?? "-"} A`],
    ["Approx. power", `${fmt.format(storm.estimatedPowerWatts || 0)} W`],
    ["Deadline", storm.deadline ? new Date(storm.deadline).toLocaleString() : "-"],
    ["BMS limit", storm.bmsChargeLimitA ? `${fmt.format(storm.bmsChargeLimitA)} A` : "-"],
  ]);

  const editing = els.stormChargeForm.contains(document.activeElement);
  if (!editing) {
    els.stormTargetSOC.value = settings.targetSoc ?? 95;
    els.stormMaxCurrent.value = settings.maxCurrentA ?? 50;
    els.stormTimeoutMinutes.value = Math.round(durationToMinutes(settings.timeout || "12h"));
  }
  [els.stormTargetSOC, els.stormMaxCurrent, els.stormTimeoutMinutes].forEach((input) => {
    input.disabled = active;
  });
  els.stormChargeStart.disabled = active;
  els.stormChargeCancel.disabled = !active;
  updateStormPowerPreview();
}

function stormPhaseTitle(phase) {
  return {
    idle: "Ready",
    completed: "Target reached",
    cancelled: "Cancelled and restored",
    timed_out: "Timed out and restored",
    error: "Stopped with an error",
    restoring: "Restoring previous settings",
  }[phase] || phase.replaceAll("_", " ");
}

function durationToMinutes(value) {
  const input = String(value).trim();
  const segment = /([0-9.]+)(ms|h|m|s)/g;
  const factors = { ms: 1 / 60000, s: 1 / 60, m: 1, h: 60 };
  let total = 0;
  let cursor = 0;
  let match;
  while ((match = segment.exec(input)) !== null) {
    if (match.index !== cursor) {
      return 720;
    }
    total += Number(match[1]) * factors[match[2]];
    cursor = segment.lastIndex;
  }
  return cursor === input.length && cursor > 0 ? total : 720;
}

function updateStormPowerPreview() {
  if (!els.stormPowerPreview) {
    return;
  }
  const current = Number(els.stormMaxCurrent.value || 0);
  const voltage = Number(latestStatus?.stormCharge?.batteryVoltage || 0);
  const watts = Math.round(current * voltage);
  els.stormPowerPreview.textContent = voltage > 0 ? `${fmt.format(watts)} W` : "-";
  els.stormPowerDetail.textContent = voltage > 0
    ? `${fmt.format(current)} A × ${fmt.format(voltage)} V; actual power is controlled by the inverter and BMS.`
    : "Calculated from live battery voltage.";
}

function stormChargePayload() {
  return {
    targetSoc: Number(els.stormTargetSOC.value),
    maxCurrentA: Number(els.stormMaxCurrent.value),
    timeout: `${Number(els.stormTimeoutMinutes.value)}m`,
  };
}

async function startStormCharge(event) {
  event.preventDefault();
  const payload = stormChargePayload();
  const watts = Math.round(payload.maxCurrentA * Number(latestStatus?.stormCharge?.batteryVoltage || 0));
  const message = `Start storm charge to ${payload.targetSoc}% with a ${payload.maxCurrentA} A limit (about ${watts} W)? The load will temporarily move to utility.`;
  if (!window.confirm(message)) {
    return;
  }
  els.stormChargeResult.textContent = "Starting and saving previous settings...";
  try {
    await fetchJSON("/api/v1/storm-charge/start", { method: "POST", body: JSON.stringify(payload) });
    els.stormChargeResult.textContent = "Storm charge started.";
    await loadStatus({ forceControls: true });
  } catch (error) {
    els.stormChargeResult.textContent = error.message;
  }
}

async function cancelStormCharge() {
  if (!window.confirm("Cancel storm charge and restore all previous inverter settings now?")) {
    return;
  }
  els.stormChargeResult.textContent = "Cancelling and restoring settings...";
  try {
    await fetchJSON("/api/v1/storm-charge/cancel", { method: "POST" });
    els.stormChargeResult.textContent = "Storm charge cancelled; previous settings restored.";
    await loadStatus({ forceControls: true });
  } catch (error) {
    els.stormChargeResult.textContent = error.message;
  }
}

function renderHeader(status, byId) {
  const deviceName = status.device?.name || "srne-main";
  const soc = valueText(byId.battery_soc);
  const load = valueText(byId.load_power);
  const pv = valueText(byId.pv_power);

  els.deviceTitle.textContent = deviceName;
  els.summary.textContent = `${status.device?.port || "serial not set"} · slave ${status.device?.slaveId ?? "-"} · SOC ${soc} · Load ${load} · PV ${pv}`;

  els.topStatus.innerHTML = "";
  Object.values(status.service || {})
    .sort((a, b) => a.name.localeCompare(b.name))
    .forEach((svc) => {
      const pill = document.createElement("span");
      pill.className = `status-pill ${svc.connected ? "ok" : "bad"}`;
      pill.textContent = `${svc.name}: ${svc.status}`;
      els.topStatus.appendChild(pill);
    });
}

function renderServices(services) {
  els.services.innerHTML = "";
  Object.values(services)
    .sort((a, b) => a.name.localeCompare(b.name))
    .forEach((svc) => {
      const div = document.createElement("div");
      div.className = "metric-tile";
      div.innerHTML = `
        <div class="tile-top">
          <span class="tile-label">${escapeHTML(svc.name)}</span>
          <span class="dot ${svc.connected ? "ok" : "bad"}"></span>
        </div>
        <strong class="${svc.connected ? "ok-text" : "bad-text"}">${escapeHTML(svc.status)}</strong>
        ${svc.lastError ? `<small>${escapeHTML(svc.lastError)}</small>` : `<small>${formatUpdatedAt(svc.lastSuccess || svc.updatedAt)}</small>`}
      `;
      els.services.appendChild(div);
    });
}

function renderHeroKpis(byId) {
  const items = [
    byId.today_production,
    byId.today_load_consumption,
    byId.today_energy_import,
    byId.system_energy_efficiency_total,
    byId.system_energy_losses_total,
    byId.total_load_consumption,
  ].filter(Boolean);

  renderTelemetryCards(els.heroKpiGrid, items, "Waiting for energy counters.");
}

function renderPowerOverview(byId) {
  const batteryCurrent = Number(byId.battery_current?.value ?? byId.battery_current?.rendered ?? 0);
  const batteryVoltage = Number(byId.battery_voltage?.value ?? byId.battery_voltage?.rendered ?? 0);
  const gridPower = Number(byId.grid_power?.value ?? byId.grid_power?.rendered ?? 0);
  const pvPower = Number(byId.pv_power?.value ?? byId.pv_power?.rendered ?? 0);
  const machineState = String(byId.machine_state?.rendered ?? byId.machine_state?.value ?? "");
  const batteryPower = Math.round(Math.abs(batteryCurrent * batteryVoltage));
  const gridActive = machineState === "AC Power Operation" || gridPower > 20;
  const direction = batteryCurrent > 0.1 ? "Discharging" : batteryCurrent < -0.1 ? "Charging" : "Idle";
  const activeLabel = gridActive ? "Grid Active" : "Inverter Active";
  const solarActive = pvPower > 20;
  const batteryDischarging = batteryCurrent > 0.1;
  const batteryCharging = batteryCurrent < -0.1;
  const mapClasses = [
    gridActive ? "grid-active" : "inverter-active",
    solarActive ? "solar-active" : "",
    batteryDischarging ? "battery-discharge" : "",
    batteryCharging ? "battery-charge" : "",
    gridActive && batteryCharging ? "grid-charge" : "",
  ].filter(Boolean).join(" ");

  els.powerOverview.innerHTML = `
    <div class="power-tile solar">
      <div class="tile-top">
        <span class="tile-label">${escapeHTML(activeLabel)}</span>
        <span class="speed-chip">${escapeHTML(direction)}</span>
      </div>
      <div class="energy-map ${mapClasses}">
        <svg class="energy-lines" viewBox="0 0 420 280" aria-hidden="true">
          <path class="energy-line grid-to-load" d="M84 154 H326"></path>
          <path class="energy-line solar-to-load" d="M210 66 V112 C210 145 238 154 326 154"></path>
          <path class="energy-line battery-to-load" d="M210 224 V188 C210 164 238 154 326 154"></path>
          <path class="energy-line solar-to-battery" d="M210 66 V224"></path>
          <path class="energy-line grid-to-battery" d="M84 154 H168 C196 154 210 178 210 224"></path>
        </svg>
        <div class="energy-node solar-node">
          <span class="node-icon inverter-symbol"></span>
          <span>Solar</span>
          <strong>${valueText(byId.pv_power)}</strong>
        </div>
        <div class="energy-node grid-node ${gridActive ? "active" : ""}">
          <span class="node-icon grid-symbol"></span>
          <span>Grid</span>
          <strong>${valueText(byId.grid_power)}</strong>
          <small>${valueText(byId.grid_voltage)} / ${valueText(byId.grid_frequency)}</small>
        </div>
        <div class="energy-node battery-node ${batteryCharging || batteryDischarging ? "active" : ""}">
          <span class="node-icon battery-symbol"></span>
          <span>Battery</span>
          <strong>${fmt.format(batteryPower)} W</strong>
          <small>SOC ${valueText(byId.battery_soc)}</small>
        </div>
        <div class="energy-node load-node active">
          <span class="node-icon load-symbol"></span>
          <span>Output</span>
          <strong>${valueText(byId.load_power)}</strong>
          <small>Load</small>
        </div>
      </div>
    </div>
  `;
}

function renderEnergy(byId) {
  const ids = [
    "today_production",
    "today_load_consumption",
    "today_energy_import",
    "total_production",
    "total_load_consumption",
    "total_energy_import",
    "battery_charge_energy_total_estimate",
    "battery_discharge_energy_total_estimate",
    "system_energy_losses_total",
    "system_energy_efficiency_total",
  ];
  renderTelemetryCards(els.energyGrid, ids.map((id) => byId[id]).filter(Boolean), "Waiting for energy counters.");
}

function renderTelemetry(items) {
  const hidden = new Set([
    "reset_machine",
    "battery_charge_energy_total_estimate",
    "battery_discharge_energy_total_estimate",
    "system_energy_losses_total",
    "system_energy_efficiency_total",
  ]);
  const sensors = items.filter((item) => !item.writable && !item.rawOnly && !hidden.has(item.id));
  els.telemetrySubtitle.textContent = `${sensors.length} sensors`;
  renderTelemetryCards(els.telemetryGrid, sensors, "Waiting for Modbus data.");
}

function renderTelemetryCards(target, items, emptyMessage) {
  if (!items.length) {
    target.innerHTML = `<div class="telemetry-empty">${escapeHTML(emptyMessage)}</div>`;
    return;
  }

  target.innerHTML = items.map((item) => `
    <article class="telemetry-card">
      <div class="telemetry-label">${escapeHTML(item.name)}</div>
      <div class="telemetry-value">${escapeHTML(valueText(item))}</div>
      <div class="telemetry-meta">0x${Number(item.address).toString(16).padStart(4, "0")} · ${escapeHTML(item.group)} · ${formatUpdatedAt(item.updatedAt)}</div>
    </article>
  `).join("");
}

function renderControls(items) {
  const controls = items.filter((item) => item.writable && !item.writeOnly);
  const maintenance = items.filter((item) => item.writable && item.writeOnly);

  renderControlCards(els.controlsGrid, controls, "No writable settings are exposed.");
  if (!maintenance.length) {
    els.maintenanceSection.hidden = true;
    els.maintenanceGrid.innerHTML = "";
  } else {
    els.maintenanceSection.hidden = false;
    renderControlCards(els.maintenanceGrid, maintenance, "No maintenance actions exposed.");
  }

  attachWriteHandlers();
}

function renderControlCards(target, items, emptyMessage) {
  if (!items.length) {
    target.innerHTML = `<div class="telemetry-empty">${escapeHTML(emptyMessage)}</div>`;
    return;
  }

  target.innerHTML = items.map((item) => {
    const control = renderWriteControl(item);
    return `
      <article class="control-card">
        <div class="telemetry-label">${escapeHTML(item.name)}</div>
        <div class="telemetry-value">${escapeHTML(valueText(item))}</div>
        <div class="telemetry-meta">0x${Number(item.address).toString(16).padStart(4, "0")} · ${escapeHTML(item.group)} · ${formatUpdatedAt(item.updatedAt)}</div>
        ${control}
      </article>
    `;
  }).join("");
}

function renderWriteControl(item) {
  if (item.writeOnly && item.options?.length === 1) {
    const confirmMessage = confirmationMessageForControl(item);
    const confirmAttr = confirmMessage ? ` data-confirm-message="${escapeAttribute(confirmMessage)}"` : "";
    return `
      <div class="telemetry-actions">
        <button class="action-button" data-write-button="${item.id}" data-write-value="${item.options[0].raw}"${confirmAttr} type="button">${escapeHTML(item.options[0].label)}</button>
      </div>
    `;
  }

  const serverValue = item.options?.length ? item.raw : item.rendered;
  const draftValue = controlDrafts.has(item.id) ? controlDrafts.get(item.id) : serverValue;
  if (item.options?.length) {
    const options = item.options.map((option) => {
      const selected = String(option.raw) === String(draftValue) ? "selected" : "";
      return `<option value="${option.raw}" ${selected}>${escapeHTML(option.label)}</option>`;
    }).join("");
    return `
      <div class="telemetry-actions">
        <select data-write-id="${item.id}" data-server-value="${escapeAttribute(serverValue)}">${options}</select>
        <button class="action-button" data-write-button="${item.id}" type="button">Apply</button>
      </div>
    `;
  }

  const step = item.writeStep || 1;
  const min = Number.isFinite(item.writeMin) ? `min="${item.writeMin}"` : "";
  const max = Number.isFinite(item.writeMax) ? `max="${item.writeMax}"` : "";
  return `
    <div class="telemetry-actions">
      <input data-write-id="${item.id}" data-server-value="${escapeAttribute(serverValue)}" type="number" step="${step}" ${min} ${max} value="${escapeAttribute(draftValue)}">
      <button class="action-button" data-write-button="${item.id}" type="button">Apply</button>
    </div>
  `;
}

function attachWriteHandlers() {
  document.querySelectorAll("[data-write-id]").forEach((input) => {
    const rememberDraft = () => {
      const id = input.getAttribute("data-write-id");
      if (String(input.value) === String(input.getAttribute("data-server-value"))) {
        controlDrafts.delete(id);
        return;
      }
      controlDrafts.set(id, input.value);
    };
    input.addEventListener("input", rememberDraft);
    input.addEventListener("change", rememberDraft);
  });

  document.querySelectorAll("[data-write-button]").forEach((button) => {
    button.addEventListener("click", async () => {
      const id = button.getAttribute("data-write-button");
      const input = document.querySelector(`[data-write-id="${id}"]`);
      const forcedValue = button.getAttribute("data-write-value");
      const value = forcedValue ?? input?.value;
      if (value == null) {
        return;
      }

      const confirmMessage = button.getAttribute("data-confirm-message")
        || confirmationMessageForWrite(id, value, input);
      if (confirmMessage && !window.confirm(confirmMessage)) {
        els.writeResult.textContent = `Write cancelled for ${id}.`;
        return;
      }

      els.writeResult.textContent = `Writing ${id}...`;
      try {
        await fetchJSON(`/api/v1/registers/${id}/write`, {
          method: "POST",
          body: JSON.stringify({ value }),
        });
        controlDrafts.delete(id);
        input?.blur();
        els.writeResult.textContent = `Register ${id} written.`;
        await loadStatus({ forceControls: true });
      } catch (error) {
        els.writeResult.textContent = error.message;
      }
    });
  });
}

function reconcileControlDrafts(items) {
  for (const item of items) {
    if (!controlDrafts.has(item.id)) {
      continue;
    }
    const serverValue = item.options?.length ? item.raw : item.rendered;
    if (String(controlDrafts.get(item.id)) === String(serverValue)) {
      controlDrafts.delete(item.id);
    }
  }
}

function isControlInteractionActive() {
  const active = document.activeElement;
  return Boolean(
    active
    && (els.controlsGrid.contains(active) || els.maintenanceGrid.contains(active))
  );
}

function renderSettings(status) {
  setDefinitionList("settings-runtime", [
    ["Version", status.build?.version ?? "-"],
    ["Commit", status.build?.commit ?? "-"],
    ["Build date", status.build?.buildDate ?? "-"],
    ["Config path", status.runtime?.configPath ?? "-"],
    ["Uptime", status.runtime?.uptime ?? "-"],
    ["Device", status.device?.name ?? "-"],
    ["Port", status.device?.port ?? "-"],
    ["Network protocol", status.device?.networkProtocol ?? "-"],
    ["Slave ID", status.device?.slaveId ?? "-"],
    ["Inverter type", status.device?.inverterType ?? "-"],
  ]);
}

function renderPorts(payload) {
  if (!payload.ports?.length) {
    els.serialPorts.innerHTML = "<li>No serial ports detected.</li>";
    return;
  }
  els.serialPorts.innerHTML = payload.ports.map((port) => `<li>${escapeHTML(port)}</li>`).join("");
}

function renderHAConfig(status) {
  if (!els.haYaml) {
    return;
  }
  els.haYaml.textContent = generateDashboardYAML(status, haYamlMode);
}

function generateDashboardYAML(status, mode) {
  const view = generateSectionsViewYAML(status);
  if (mode === "tab") {
    return `${view.join("\n")}\n`;
  }
  return `title: SRNE Inverter\nviews:\n${viewAsListItem(view)}\n`;
}

function generateSectionsViewYAML(status) {
  const telemetry = status.telemetry || [];
  const existing = new Set(telemetry.map((item) => item.id));
  const entity = (id) => `sensor.${sanitizeEntity(status.device?.name || "srne_main")}_${sanitizeEntity(id)}`;
  const namedEntity = (component, id) => `${component}.${sanitizeEntity(status.device?.name || "srne_main")}_${sanitizeEntity(id)}`;
  const sensorLine = (id, name = null) => [
    "      - type: tile",
    `        entity: ${entity(id)}`,
    ...(name ? [`        name: ${name}`] : []),
    "        vertical: false",
  ];
  const has = (id) => existing.has(id);

  const energyEntities = [
    "battery_charge_energy_total_estimate",
    "battery_discharge_energy_total_estimate",
    "total_production",
    "total_load_consumption",
    "total_energy_import",
    "system_energy_losses_total",
    "system_energy_efficiency_total",
  ].filter(has);
  const liveEntities = [
    "battery_soc",
    "battery_voltage",
    "battery_current",
    "pv_power",
    "pv1_power",
    "pv1_voltage",
    "pv1_current",
    "pv2_power",
    "pv2_voltage",
    "pv2_current",
    "load_power",
    "grid_power",
    "grid_voltage_phase_a",
    "grid_voltage_phase_b",
    "grid_voltage_phase_c",
    "grid_current_phase_a",
    "grid_current_phase_b",
    "grid_current_phase_c",
    "grid_frequency",
    "load_power_phase_a",
    "load_power_phase_b",
    "load_power_phase_c",
    "grid_power_phase_a",
    "grid_power_phase_b",
    "grid_power_phase_c",
    "machine_state",
  ].filter(has);
  const configEntities = [
    "battery_discharge_cutoff_soc",
    "charge_termination_current",
    "battery_charge_cutoff_soc",
    "battery_low_soc_alarm",
    "battery_discharge_stop",
    "battery_discharge_start",
    "bms_charge_limit_mode",
    "bms_communication_enable",
    "bms_protocol",
    "grid_operating_mode",
    "on_grid_max_power",
    "zero_export_power",
    "output_source_priority",
    "charger_source_priority",
    "power_saving_mode",
    "overload_auto_restart",
    "overtemperature_auto_restart",
    "buzzer_alarm",
    "source_change_alert",
    "overload_bypass",
    "pv_charge_current_setup",
    "maximum_charge_current",
    "mains_charge_current_limit",
  ].filter(has);

  const lines = [
    "type: sections",
    "title: SRNE Inverter",
    "path: srne-inverter",
    "icon: mdi:solar-power-variant",
    "max_columns: 4",
    "sections:",
    "  - type: grid",
    "    cards:",
    "      - type: heading",
    "        heading: Live power",
    ...["battery_soc", "pv_power", "load_power", "grid_power"].filter(has).flatMap((id) => sensorLine(id)),
    ...["pv1_power", "pv1_voltage", "pv1_current", "pv2_power", "pv2_voltage", "pv2_current"].filter(has).flatMap((id) => sensorLine(id)),
    ...["load_power_phase_a", "load_power_phase_b", "load_power_phase_c", "grid_power_phase_a", "grid_power_phase_b", "grid_power_phase_c", "grid_voltage_phase_a", "grid_voltage_phase_b", "grid_voltage_phase_c", "grid_current_phase_a", "grid_current_phase_b", "grid_current_phase_c", "grid_frequency"].filter(has).flatMap((id) => sensorLine(id)),
    "      - type: history-graph",
    "        title: Power and SOC",
    "        hours_to_show: 24",
    "        entities:",
    ...["battery_soc", "pv_power", "load_power", "grid_power"].filter(has).map((id) => `          - entity: ${entity(id)}`),
    "  - type: grid",
    "    cards:",
    "      - type: heading",
    "        heading: Battery system",
  ];

  if (!energyEntities.length) {
    lines.push(
      "      - type: markdown",
      "        content: >",
      "          No energy sensors were visible when this YAML was generated."
    );
  } else {
    lines.push(...energyEntities.flatMap((id) => sensorLine(id)));
    lines.push(
      "      - type: history-graph",
      "        title: Battery energy counters",
      "        hours_to_show: 168",
      "        entities:",
      ...energyEntities.slice(0, 4).map((id) => `          - entity: ${entity(id)}`)
    );
  }

  lines.push(
    "  - type: grid",
    "    cards:",
    "      - type: heading",
    "        heading: Diagnostics"
  );
  lines.push(...liveEntities.flatMap((id) => sensorLine(id)));

  if (status.device?.inverterType === "spi_h3p") {
    lines.push(
      "  - type: grid",
      "    cards:",
      "      - type: heading",
      "        heading: Storm charge",
      "      - type: tile",
      `        entity: ${namedEntity("switch", "storm_charge")}`,
      "      - type: tile",
      `        entity: ${namedEntity("number", "storm_charge_target_soc")}`,
      "      - type: tile",
      `        entity: ${namedEntity("number", "storm_charge_max_current")}`,
      "      - type: tile",
      `        entity: ${namedEntity("number", "storm_charge_timeout_minutes")}`,
      "      - type: tile",
      `        entity: ${namedEntity("sensor", "storm_charge_status")}`,
      "      - type: tile",
      `        entity: ${namedEntity("sensor", "storm_charge_estimated_power")}`
    );
  }

  lines.push(
    "  - type: grid",
    "    cards:",
    "      - type: heading",
    "        heading: Configuration"
  );
  if (!configEntities.length) {
    lines.push(
      "      - type: markdown",
      "        content: >",
      "          No configuration entities were visible when this YAML was generated."
    );
  } else {
    lines.push(...configEntities.flatMap((id) => sensorLine(id)));
  }

  return lines;
}

function fillConfigForm(cfg) {
  latestConfig = cfg;
  els.deviceName.value = cfg.device.name;
  els.deviceSlaveID.value = cfg.device.slaveId;
  const inverterType = cfg.device.inverterType || "single_phase";
  els.deviceInverterType.value = ["three_phase", "three-phase", "three", "3p"].includes(inverterType)
    ? "spi_h3p"
    : inverterType;
  els.serialPort.value = cfg.serial.port;
  els.serialNetworkProtocol.value = cfg.serial.networkProtocol || "rtu";
  els.serialBaudRate.value = cfg.serial.baudRate;
  els.serialParity.value = cfg.serial.parity;
  els.serialTimeout.value = cfg.serial.timeout;
  els.mqttBroker.value = cfg.mqtt.broker;
  els.mqttUsername.value = cfg.mqtt.username;
  els.mqttPassword.value = cfg.mqtt.password;
  els.mqttClientID.value = cfg.mqtt.clientId;
  els.mqttTopicPrefix.value = cfg.mqtt.topicPrefix;
  els.mqttDiscoveryPrefix.value = cfg.mqtt.discoveryPrefix;
  els.httpListen.value = cfg.http.listen;
  els.loggingLevel.value = cfg.logging.level;
  els.mqttRetain.checked = cfg.mqtt.retain;
}

function collectConfigForm() {
  return {
    device: {
      name: els.deviceName.value.trim(),
      slaveId: Number(els.deviceSlaveID.value),
      inverterType: els.deviceInverterType.value,
    },
    serial: {
      port: els.serialPort.value.trim(),
      networkProtocol: els.serialNetworkProtocol.value,
      baudRate: Number(els.serialBaudRate.value),
      dataBits: latestConfig?.serial?.dataBits || 8,
      parity: els.serialParity.value,
      stopBits: latestConfig?.serial?.stopBits || 1,
      timeout: els.serialTimeout.value.trim(),
    },
    polling: latestConfig?.polling || {
      fastInterval: "15s",
      slowInterval: "1m",
      reconnectDelay: "5s",
    },
    mqtt: {
      broker: els.mqttBroker.value.trim(),
      username: els.mqttUsername.value,
      password: els.mqttPassword.value,
      clientId: els.mqttClientID.value.trim(),
      topicPrefix: els.mqttTopicPrefix.value.trim(),
      discoveryPrefix: els.mqttDiscoveryPrefix.value.trim(),
      retain: els.mqttRetain.checked,
    },
    http: {
      listen: els.httpListen.value.trim(),
    },
    logging: {
      level: els.loggingLevel.value,
    },
    stormCharge: latestConfig?.stormCharge || {
      targetSoc: 95,
      maxCurrentA: 50,
      timeout: "12h",
    },
  };
}

async function saveConfig(event) {
  event.preventDefault();
  els.saveResult.textContent = "Saving...";
  try {
    const cfg = collectConfigForm();
    await fetchJSON("/api/v1/config", {
      method: "PUT",
      body: JSON.stringify(cfg),
    });
    els.saveResult.textContent = "Configuration saved.";
    await Promise.all([loadConfig(), loadStatus(), loadPorts()]);
  } catch (error) {
    els.saveResult.textContent = error.message;
  }
}

function initNavigation() {
  const expanded = localStorage.getItem("srneRailExpanded") === "true";
  document.body.classList.toggle("rail-expanded", expanded);
  els.rail.classList.toggle("expanded", expanded);

  els.railToggle.addEventListener("click", () => {
    const next = !document.body.classList.contains("rail-expanded");
    document.body.classList.toggle("rail-expanded", next);
    els.rail.classList.toggle("expanded", next);
    localStorage.setItem("srneRailExpanded", String(next));
  });

  document.querySelectorAll("[data-tab]").forEach((button) => {
    button.addEventListener("click", () => activateTab(button.dataset.tab));
  });

  document.querySelectorAll("[data-copy-target]").forEach((button) => {
    button.addEventListener("click", async () => {
      const target = document.getElementById(button.dataset.copyTarget);
      await copyText(target.textContent);
      const previous = button.textContent;
      button.textContent = "Copied";
      window.setTimeout(() => {
        button.textContent = previous;
      }, 1200);
    });
  });

  document.querySelectorAll('input[name="ha-yaml-mode"]').forEach((input) => {
    input.checked = input.value === haYamlMode;
    input.addEventListener("change", () => {
      haYamlMode = input.value;
      localStorage.setItem("srneHaYamlMode", haYamlMode);
      if (latestStatus) {
        renderHAConfig(latestStatus);
      }
    });
  });
}

function activateTab(tab) {
  document.querySelectorAll("[data-tab]").forEach((button) => {
    button.classList.toggle("active", button.dataset.tab === tab);
  });
  document.querySelectorAll(".tab-view").forEach((view) => {
    view.classList.toggle("active", view.id === `view-${tab}`);
  });
  if (latestStatus) {
    renderHAConfig(latestStatus);
  }
}

function startAutoRefresh() {
  if (refreshTimer) {
    window.clearInterval(refreshTimer);
  }
  refreshTimer = window.setInterval(() => {
    loadStatus().catch((error) => {
      els.saveResult.textContent = error.message;
    });
  }, REFRESH_INTERVAL_MS);
}

async function copyText(text) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fall back to a temporary textarea for non-secure HTTP contexts.
    }
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  document.execCommand("copy");
  textarea.remove();
}

function valueText(item) {
  if (!item) {
    return "-";
  }
  return `${item.rendered}${item.unit ? ` ${item.unit}` : ""}`;
}

function setDefinitionList(id, rows) {
  const el = document.getElementById(id);
  if (!el) {
    return;
  }
  el.innerHTML = rows.map(([label, value]) => `
    <dt>${escapeHTML(label)}</dt>
    <dd>${escapeHTML(String(value))}</dd>
  `).join("");
}

function serviceSummary(service) {
  if (!service) {
    return "unknown";
  }
  if (service.lastError) {
    return `${service.status} (${service.lastError})`;
  }
  return service.status;
}

function confirmationMessageForControl(item) {
  if (item.id === "reset_machine") {
    return "Are you sure you want to reset the inverter controller?";
  }
  return "";
}

function confirmationMessageForWrite(id, value, input) {
  if (id !== "grid_operating_mode") {
    return "";
  }

  const selectedLabel = input?.options?.[input.selectedIndex]?.textContent || value;
  if (String(value) === "1") {
    return "Enable ON-GRID export? Surplus PV energy may be fed into the utility grid. Confirm that the grid connection and operator requirements are satisfied.";
  }
  return `Change grid operating mode to "${selectedLabel}"? Power routing may change immediately.`;
}

function formatUpdatedAt(value) {
  if (!value) {
    return "n/a";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "n/a";
  }
  return date.toLocaleString();
}

function viewAsListItem(lines) {
  return lines.map((line, index) => {
    if (index === 0) {
      return `  - ${line}`;
    }
    return `    ${line}`;
  }).join("\n");
}

function sanitizeEntity(value) {
  return String(value)
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll("\"", "&quot;")
    .replaceAll("'", "&#39;");
}

function escapeAttribute(value) {
  return escapeHTML(value);
}

async function bootstrap() {
  initNavigation();
  els.refreshStatus.addEventListener("click", () => {
    loadStatus().catch((error) => {
      els.saveResult.textContent = error.message;
    });
  });
  els.refreshPorts.addEventListener("click", () => {
    loadPorts().catch((error) => {
      els.saveResult.textContent = error.message;
    });
  });
  els.reloadConfig.addEventListener("click", () => {
    loadConfig().catch((error) => {
      els.saveResult.textContent = error.message;
    });
  });
  els.configForm.addEventListener("submit", saveConfig);
  els.stormChargeForm?.addEventListener("submit", startStormCharge);
  els.stormChargeCancel?.addEventListener("click", cancelStormCharge);
  els.stormMaxCurrent?.addEventListener("input", updateStormPowerPreview);
  els.rawDataSearch?.addEventListener("input", () => renderRawData());
  els.rawDataClear?.addEventListener("click", () => {
    els.rawDataSearch.value = "";
    els.rawDataSearch.focus();
    renderRawData();
  });
  els.rawDataPause?.addEventListener("click", () => {
    rawDataPaused = !rawDataPaused;
    els.rawDataPause.textContent = rawDataPaused ? "Resume updates" : "Pause updates";
    els.rawDataPause.classList.toggle("paused", rawDataPaused);
    renderRawData(rawDataPaused ? undefined : latestStatus?.telemetry || []);
  });
  els.inverterClockForm?.addEventListener("submit", setInverterClock);
  els.refreshInverterClock?.addEventListener("click", loadInverterClock);
  els.useBrowserClock?.addEventListener("click", () => {
    els.inverterClockInput.value = localDateTimeInputValue();
  });
  startAutoRefresh();

  try {
    els.inverterClockInput.value = localDateTimeInputValue();
    await Promise.all([loadStatus(), loadPorts(), loadConfig(), loadInverterClock()]);
  } catch (error) {
    els.summary.textContent = error.message;
    els.saveResult.textContent = error.message;
  }
}

bootstrap();
