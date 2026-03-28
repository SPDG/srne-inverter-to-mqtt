const THEME_KEY = "srne-theme";
const REFRESH_INTERVAL_MS = 15_000;

const state = {
  config: null,
  refreshTimer: null,
  theme: null,
};

const els = {
  buildVersion: document.getElementById("build-version"),
  runtimeUptime: document.getElementById("runtime-uptime"),
  serviceStatus: document.getElementById("service-status"),
  statusDetails: document.getElementById("status-details"),
  controlsGrid: document.getElementById("controls-grid"),
  telemetryGrid: document.getElementById("telemetry-grid"),
  serialPorts: document.getElementById("serial-ports"),
  saveResult: document.getElementById("save-result"),
  refreshStatus: document.getElementById("refresh-status"),
  refreshPorts: document.getElementById("refresh-ports"),
  reloadConfig: document.getElementById("reload-config"),
  configForm: document.getElementById("config-form"),
  themeToggle: document.getElementById("theme-toggle"),
  deviceName: document.getElementById("device-name"),
  deviceSlaveID: document.getElementById("device-slave-id"),
  serialPort: document.getElementById("serial-port"),
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
    headers: {
      "Content-Type": "application/json",
    },
    ...options,
  });

  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload.error || `Request failed: ${response.status}`);
  }

  return payload;
}

function renderStatus(status) {
  els.buildVersion.textContent = status.build.version || "dev";
  els.runtimeUptime.textContent = status.runtime.uptime;

  const serviceLine = ["web", "modbus", "mqtt"]
    .map((name) => {
      const entry = status.service?.[name];
      if (!entry) {
        return `${name} unknown`;
      }
      return `${name} ${entry.status}`;
    })
    .join(" · ");

  els.serviceStatus.textContent = serviceLine;
  els.serviceStatus.dataset.tone = overallTone(status.service);

  const items = [
    ["Device name", status.device.name],
    ["Slave ID", String(status.device.slaveId)],
    ["Serial port", status.device.port || "Not set"],
    ["Modbus", serviceSummary(status.service?.modbus)],
    ["MQTT", serviceSummary(status.service?.mqtt)],
    ["Config path", status.runtime.configPath],
    ["Started at", new Date(status.runtime.startedAt).toLocaleString()],
    ["Last status update", newestUpdate(status.service)],
  ];

  els.statusDetails.innerHTML = items
    .map(([label, value]) => `<div><dt>${label}</dt><dd>${value}</dd></div>`)
    .join("");

  renderTelemetry(status.telemetry || []);
}

function renderPorts(payload) {
  if (!payload.ports.length) {
    els.serialPorts.innerHTML = "<li>No serial ports detected.</li>";
    return;
  }

  els.serialPorts.innerHTML = payload.ports.map((port) => `<li>${port}</li>`).join("");
}

function renderTelemetry(items) {
  const controls = items.filter((item) => item.writable);
  const sensors = items.filter((item) => !item.writable);

  renderTelemetryGroup(els.controlsGrid, controls, "No writable settings are exposed yet.", true);
  renderTelemetryGroup(els.telemetryGrid, sensors, "Waiting for Modbus data...", false);

  attachWriteHandlers();
}

function renderTelemetryGroup(target, items, emptyMessage, controlsOnly) {
  if (!items.length) {
    target.innerHTML = `<div class="telemetry-empty">${emptyMessage}</div>`;
    return;
  }

  target.innerHTML = items
    .map((item) => {
      const unit = item.unit ? ` ${item.unit}` : "";
      const control = item.writable ? renderWriteControl(item) : "";
      const cardClass = item.writable ? "telemetry-card is-control" : "telemetry-card";
      const meta = controlsOnly
        ? `0x${Number(item.address).toString(16).padStart(4, "0")} · ${item.group} · updated ${formatUpdatedAt(item.updatedAt)}`
        : `0x${Number(item.address).toString(16).padStart(4, "0")} · ${item.group} · ${formatUpdatedAt(item.updatedAt)}`;

      return `
        <article class="${cardClass}">
          <div class="telemetry-label">${item.name}</div>
          <div class="telemetry-value">${item.rendered}${unit}</div>
          <div class="telemetry-meta">${meta}</div>
          ${control}
        </article>
      `;
    })
    .join("");
}

function renderWriteControl(item) {
  if (item.options?.length) {
    const options = item.options
      .map((option) => {
        const selected = option.label === item.rendered ? "selected" : "";
        return `<option value="${option.raw}" ${selected}>${option.label}</option>`;
      })
      .join("");

    return `
      <div class="telemetry-actions">
        <select data-write-id="${item.id}">${options}</select>
        <button class="button secondary" data-write-button="${item.id}" type="button">Apply</button>
      </div>
    `;
  }

  const step = item.writeStep || 1;
  const min = Number.isFinite(item.writeMin) ? `min="${item.writeMin}"` : "";
  const max = Number.isFinite(item.writeMax) ? `max="${item.writeMax}"` : "";

  return `
    <div class="telemetry-actions">
      <input
        data-write-id="${item.id}"
        type="number"
        step="${step}"
        ${min}
        ${max}
        value="${item.rendered}"
      >
      <button class="button secondary" data-write-button="${item.id}" type="button">Apply</button>
    </div>
  `;
}

function attachWriteHandlers() {
  document.querySelectorAll("[data-write-button]").forEach((button) => {
    button.addEventListener("click", async () => {
      const id = button.getAttribute("data-write-button");
      const input = document.querySelector(`[data-write-id="${id}"]`);
      if (!input) {
        return;
      }

      els.saveResult.textContent = `Writing ${id}...`;
      try {
        await fetchJSON(`/api/v1/registers/${id}/write`, {
          method: "POST",
          body: JSON.stringify({ value: input.value }),
        });
        els.saveResult.textContent = `Register ${id} written.`;
        await loadStatus();
      } catch (error) {
        els.saveResult.textContent = error.message;
      }
    });
  });
}

function fillConfigForm(cfg) {
  state.config = cfg;
  els.deviceName.value = cfg.device.name;
  els.deviceSlaveID.value = cfg.device.slaveId;
  els.serialPort.value = cfg.serial.port;
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
    },
    serial: {
      port: els.serialPort.value.trim(),
      baudRate: Number(els.serialBaudRate.value),
      dataBits: state.config?.serial?.dataBits || 8,
      parity: els.serialParity.value,
      stopBits: state.config?.serial?.stopBits || 1,
      timeout: els.serialTimeout.value.trim(),
    },
    polling: state.config?.polling || {
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
  };
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

function newestUpdate(services) {
  const timestamps = Object.values(services || {})
    .map((service) => service.updatedAt)
    .filter(Boolean)
    .map((value) => new Date(value).getTime())
    .filter(Number.isFinite);

  if (!timestamps.length) {
    return "n/a";
  }

  return new Date(Math.max(...timestamps)).toLocaleString();
}

function overallTone(services) {
  const entries = Object.values(services || {});
  if (!entries.length) {
    return "warning";
  }

  if (entries.some((service) => service.status === "error")) {
    return "error";
  }

  if (entries.some((service) => service.status === "degraded" || !service.connected)) {
    return "warning";
  }

  return "healthy";
}

function formatUpdatedAt(value) {
  if (!value) {
    return "n/a";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "n/a";
  }

  return date.toLocaleTimeString();
}

function resolveTheme() {
  const requested = new URLSearchParams(window.location.search).get("theme");
  if (requested === "light" || requested === "dark") {
    localStorage.setItem(THEME_KEY, requested);
    return requested;
  }

  const stored = localStorage.getItem(THEME_KEY);
  if (stored === "light" || stored === "dark") {
    return stored;
  }

  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function applyTheme(theme) {
  state.theme = theme;
  document.documentElement.dataset.theme = theme;
  els.themeToggle.textContent = theme === "dark" ? "Light mode" : "Dark mode";
}

function toggleTheme() {
  const nextTheme = state.theme === "dark" ? "light" : "dark";
  localStorage.setItem(THEME_KEY, nextTheme);
  applyTheme(nextTheme);
}

async function loadStatus() {
  const status = await fetchJSON("/api/v1/status");
  renderStatus(status);
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

function startAutoRefresh() {
  if (state.refreshTimer) {
    window.clearInterval(state.refreshTimer);
  }

  state.refreshTimer = window.setInterval(() => {
    loadStatus().catch((error) => {
      els.saveResult.textContent = error.message;
    });
  }, REFRESH_INTERVAL_MS);
}

async function bootstrap() {
  applyTheme(resolveTheme());

  els.themeToggle.addEventListener("click", toggleTheme);
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
  startAutoRefresh();

  try {
    await Promise.all([loadStatus(), loadPorts(), loadConfig()]);
  } catch (error) {
    els.saveResult.textContent = error.message;
  }
}

bootstrap();
