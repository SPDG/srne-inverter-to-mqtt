# SRNE Inverter to MQTT

## Goal

Replace the unstable `RS -> USB -> USB over IP -> Home Assistant Modbus` chain with a local service running on `hive-nuc` that:

- communicates with the SRNE inverter over a local USB serial port,
- polls Modbus RTU registers,
- publishes data over MQTT to Home Assistant,
- exposes a lightweight web panel for configuration and diagnostics,
- allows selected write operations to the inverter,
- is shipped as a single binary with the frontend embedded,
- stores runtime configuration in a local YAML file.

## Context and Problem

The current setup depends on remote USB/serial transport into Home Assistant. That introduces several failure points:

- USB over IP disconnects,
- poor recovery after transport failures,
- no local buffering or diagnostics near the physical device,
- configuration split across Home Assistant and the intermediary host.

The target service on `hive-nuc` should move the Modbus layer next to the actual USB device and send only MQTT upstream to Home Assistant.

## Input Material

This analysis is based on:

- the previously used Home Assistant Modbus configuration from the migration setup,
- example implementations from:
  - `cole8888/SRNE-Solar-Charge-Controller-Monitor`,
  - `SeByDocKy/myESPhome`,
- the `SRNE_MODBUS Protocol V3.9` manual.

Online references:

- https://community.home-assistant.io/t/integrating-srne-mppt-inverter-with-ha/490475/103?page=4
- https://github.com/cole8888/SRNE-Solar-Charge-Controller-Monitor
- https://github.com/SeByDocKy/myESPhome/blob/8dc25d9a12194c6a29289947f18026ab353aceae/code/srne.yaml
- https://github.com/SeByDocKy/myESPhome/blob/8dc25d9a12194c6a29289947f18026ab353aceae/code/SRNE_MODBUS%E5%8D%8F%E8%AE%AEV3.9-%E8%8B%B1%E6%96%87%E7%89%88.pdf

## Language Convention

To keep the project ready for public collaboration:

- repository-facing artifacts should be in English,
- documentation, issues, pull requests, commit messages, and UI text should be in English,
- direct conversation with the project owner can remain in Polish.

## Findings from the Current Home Assistant Config

The current HA config includes:

- 57 Modbus sensors,
- 2 switches,
- 3 selects,
- 4 number entities,
- 30 entities polled every 20 seconds,
- 35 entities polled every 60 seconds.

Key observations:

- the current HA scope is a strong candidate for the initial MVP because it reflects the data already in use,
- some HA entities are only numeric-to-text mappings and should move into the new service,
- several registers are used for both read and write flows,
- write operations must be treated carefully because support depends on inverter model and battery type.

Registers currently used for both read and write:

- `0xE004` battery type,
- `0xE008` boost charge voltage,
- `0xE009` float charge voltage,
- `0xE00A` boost return / rebulk voltage,
- `0xE012` boost charging time,
- `0xE204` output source priority.

## Findings from the Modbus References

Confirmed protocol assumptions:

- Modbus RTU,
- default serial settings are `9600 8N1`,
- one slave device, usually `1`,
- holding register reads are used,
- single-register and multi-register writes are supported,
- many configuration parameters live in the `0xE001-0xE02D` range,
- load/output control depends on operating mode and may require a precondition before write commands work.

Important limitations:

- not every SRNE model supports every documented write,
- some settings can be changed only for specific battery types,
- the examples mix different SRNE devices, so this project needs its own explicit register catalog and support policy.

## Product Assumptions

### MVP Scope

The MVP should provide:

- local serial connection to the inverter on a selected device path,
- configuration for serial port, slave id, and timeouts,
- MQTT configuration,
- telemetry publishing over MQTT,
- Home Assistant MQTT Discovery for the main entities,
- a simple web dashboard with:
  - connection status,
  - latest readings,
  - recent error log,
  - configuration form,
- safe writes for selected configuration registers,
- Linux-first deployment as a single binary.

### Post-MVP Scope

- historical charts in the UI,
- configuration export and backup,
- authentication or at least a basic password for the web panel,
- `systemd` service installation,
- release assets for multiple Linux architectures,
- wider register coverage beyond the current HA config.

## Proposed Architecture

### 1. Register Domain Layer

Maintain a central register catalog in code, not in user YAML.

Each register definition should include:

- logical identifier,
- Modbus address,
- data type,
- register count,
- scaling,
- unit,
- access mode `read` / `write`,
- Home Assistant entity metadata,
- default polling group,
- optional enum mapping,
- write validation.

This is critical because it:

- unifies Modbus, MQTT, UI, and validation logic,
- avoids duplicated scaling knowledge,
- provides an explicit allowlist for writes.

### 2. Modbus Transport

A dedicated component should handle:

- opening and maintaining the serial port,
- serialized access to the bus,
- retry and reconnect behavior,
- grouping reads into efficient batches,
- safe register writes with locking and validation.

Important rule: do not publish directly to MQTT from the Modbus transport layer. First update an internal state snapshot, then publish from that state.

### 3. Polling Engine

Registers should be divided into groups:

- `fast`: live telemetry, for example every `10-20 s`,
- `slow`: config and less volatile values, for example every `60 s`,
- `manual`: on-demand from UI or startup-only,
- `writeback-refresh`: immediate refresh after a write to confirm device state.

The polling engine should:

- merge adjacent registers into as few requests as practical,
- tolerate partial failures,
- track the timestamp of the last successful poll,
- keep the last known good state in memory.

### 4. Application State

In-memory application state should include:

- active configuration,
- serial connection status,
- MQTT connection status,
- latest register snapshot,
- recent error ring buffer,
- process metrics.

This will support:

- a useful dashboard,
- diagnostics endpoints,
- meaningful health checks.

### 5. MQTT Layer

Recommended behavior:

- publish state topics,
- publish availability topics,
- generate Home Assistant MQTT Discovery automatically,
- use a stable topic prefix such as `srne/<device_id>/...`,
- use retained messages for discovery and, if beneficial, for last known state.

Example topics:

- `srne/<device_id>/state/<entity>`
- `srne/<device_id>/command/<entity>`
- `srne/<device_id>/availability`
- `homeassistant/<component>/<device_id>/<entity>/config`

Architectural decision:

- the web UI and API should go through the same internal write service,
- MQTT command topics can be added after MVP, or in MVP only for the most important writable entities.

### 6. HTTP Server and Web Panel

The HTTP backend should live in the same binary.

Minimum views:

- `Dashboard`
  - Modbus status,
  - MQTT status,
  - uptime,
  - key values such as SOC, PV power, battery voltage, load power, fault codes.
- `Configuration`
  - serial device selection with auto-detection for `/dev/ttyUSB*` and `/dev/ttyACM*`,
  - Modbus settings,
  - MQTT settings,
  - device name and topic prefix,
  - save configuration and test connection actions.
- `Registers`
  - quick register view,
  - manual refresh,
  - write forms only for allowed registers.
- `Diagnostics`
  - recent errors,
  - latest request/response summary,
  - build version info.

The frontend should be built into static assets and embedded with `embed`.

### 7. Configuration

User configuration should live in YAML, for example:

```yaml
device:
  name: srne-main
  slave_id: 1

serial:
  port: /dev/ttyUSB0
  baud_rate: 9600
  data_bits: 8
  parity: N
  stop_bits: 1
  timeout: 3s

polling:
  fast_interval: 15s
  slow_interval: 60s
  reconnect_delay: 5s

mqtt:
  broker: tcp://192.168.1.10:1883
  username: ""
  password: ""
  client_id: srne-hive-nuc
  topic_prefix: srne/srne-main
  discovery_prefix: homeassistant
  retain: true

http:
  listen: 0.0.0.0:8080

logging:
  level: info
```

Assumptions:

- users configure connectivity and runtime behavior,
- users do not define the full register catalog,
- the application should save config atomically,
- a default config can be generated on first startup if no file exists.

## Proposed Go Project Structure

```text
cmd/srne-inverter-to-mqtt/
internal/app/
internal/config/
internal/modbus/
internal/registers/
internal/polling/
internal/mqtt/
internal/httpapi/
internal/web/
internal/state/
internal/buildinfo/
web/
  src/
  dist/
configs/
```

### Package Responsibilities

- `internal/config`: YAML schema, load/save, defaults, validation.
- `internal/registers`: register catalog, enum mappings, scaling, write validation.
- `internal/modbus`: RTU client, read/write, reconnect, timeouts, batching.
- `internal/polling`: poll scheduling and post-write refresh.
- `internal/state`: snapshots, health, and recent error buffers.
- `internal/mqtt`: connection handling, discovery generation, publish, optional command subscriptions.
- `internal/httpapi`: REST/JSON endpoints for the frontend.
- `internal/web`: serving embedded assets.
- `internal/app`: application wiring and lifecycle.

## Proposed Stack

Recommended technology choices:

- Go `1.24.x` or the current stable version chosen for the repo,
- HTTP router: standard `net/http` or a lightweight router such as `chi`,
- frontend:
  - option A: HTMX + server-rendered templates,
  - option B: a small SPA with React/Vite.

Initial recommendation:

- backend: `net/http`,
- frontend: either a small Vite app or a simple server-rendered UI,
- asset embedding: `//go:embed`.

Practical decision:

- if the priority is to reach a working single-binary service quickly, a simple frontend is the better default,
- if the UI is expected to grow into a more interactive register editor, a small Vite frontend is still acceptable because the final output remains embedded into the binary.

## Home Assistant Integration

The most practical starting point is MQTT Discovery, not a custom HA integration.

Reasons:

- faster time to value,
- no separate HA component to maintain,
- simpler deployment,
- easy migration from the current `modbus:` YAML config.

MVP HA support should include:

- telemetry sensors,
- a small set of writable select and number entities,
- availability reporting,
- device metadata,
- enum-to-text mappings handled inside the service instead of HA template sensors.

This should reduce the HA-side configuration close to zero.

## Minimum Register Scope for the First Iteration

### Critical Telemetry

- battery SOC `0x0100`
- battery voltage `0x0101`
- battery current `0x0102`
- PV voltage `0x0107`
- PV current `0x0108`
- PV power `0x0109`
- load power `0x021B`
- grid voltage/current/frequency `0x0213-0x0215`
- inverter voltage/frequency `0x0216-0x0218`
- fault codes `0x0204-0x0205`
- temperatures `0x0220-0x0222`

### Config and Potential Write Targets

- battery type `0xE004`
- charge current limit `0xE001`
- nominal battery capacity `0xE002`
- boost/float/rebulk voltages `0xE008-0xE00A`
- discharge thresholds `0xE00B-0xE010`
- boost charging time `0xE012`
- output source priority `0xE204`
- charger source priority `0xE20F`
- AC output state `0xDF00`
- reset machine `0xDF01`

### Historical Data

- daily generation/load `0xF02F-0xF030`
- total generation/load/grid charge `0xF038-0xF04B`

## Write Safety

This should not become a generic unrestricted Modbus write console.

Required safeguards:

- explicit write allowlist,
- range and step validation,
- UI descriptions for each writable setting,
- post-write refresh,
- serialized write execution,
- audit log of recent changes,
- optional confirmation step for critical settings.

A good MVP compromise:

- expose writes only for registers already used in the current HA setup,
- clearly label any extra writes as experimental.

## Resilience and Observability

The service should survive typical environment problems on its own:

- USB serial device disappearing,
- MQTT broker restart,
- Modbus timeout,
- transient CRC or empty-response failures.

Required mechanisms:

- reconnect loop for the serial port,
- reconnect loop for MQTT,
- bounded exponential backoff,
- HTTP health endpoint,
- error counters since startup,
- timestamp and age of the last successful poll,
- degraded status when reads fail for too long.

## Build, Release, and CI

Linux-first is enough for now.

### Build Targets

Priority targets:

- `linux/amd64`,
- `linux/arm64` if needed for current or future hosts.

### Release Artifacts

- a single self-contained binary,
- an example `config.example.yaml`,
- release checksums.

### Minimum CI

GitHub Actions or equivalent should run:

- `go test ./...`
- `go vet ./...`
- frontend build,
- embedded frontend + Linux binary build,
- pipeline artifact output.

### Later CI Improvements

- frontend linting,
- integration tests with a fake Modbus server,
- release workflow on tag.

## Test Strategy

### Unit Tests

- config parsing,
- register scaling,
- enum mapping,
- write validation,
- MQTT Discovery payload generation.

### Integration Tests

- fake Modbus RTU/TCP adapter for simulated replies,
- reconnect scenarios,
- write + refresh scenarios,
- verification of discovery and state topic publishing.

### Manual Device Tests

- serial port enumeration,
- first telemetry read,
- process restart with existing config,
- USB unplug and replug,
- MQTT broker restart,
- safe register writes from the UI.

## Risks

Main technical risks:

- differences between the real SRNE model and the online examples,
- ambiguous mapping for some registers in the current HA config,
- uncertainty about which documented writes are actually supported by the target inverter,
- the need to batch reads correctly without overloading the device,
- the need to serialize reads and writes safely on one bus.

Product risks:

- letting MVP scope grow too wide,
- trying to replicate every HA entity immediately,
- overbuilding the frontend too early.

## Recommended Delivery Plan

### Phase 0: Repository Bootstrap

- initialize the Go module,
- create the directory structure,
- add a minimal embedded frontend,
- add YAML config support,
- add a basic HTTP server,
- add Linux CI.

### Phase 1: Stable Modbus Read Path

- serial device enumeration,
- RTU connection,
- batched register reads,
- in-memory state snapshot,
- dashboard with core telemetry,
- error logging.

Completion criterion:

- stable local reads on `hive-nuc` without Home Assistant involvement.

### Phase 2: MQTT + Home Assistant Discovery

- telemetry publishing,
- availability publishing,
- device metadata,
- Discovery payloads for key entities.

Completion criterion:

- Home Assistant discovers and shows the entities without manual `modbus:` YAML configuration.

### Phase 3: UI Configuration Flow

- serial and MQTT settings forms,
- config persistence,
- reload or restart of subsystems without manual file editing,
- connection test actions.

### Phase 4: Register Write Support

- write allowlist,
- validation,
- write actions from the web panel,
- optional MQTT command topics.

### Phase 5: Hardening

- `systemd` service support,
- integration tests,
- release workflow,
- improved diagnostics,
- config import/export.

## Concrete MVP Recommendation

To reach a working system quickly, the first runnable version should be limited to:

- reading roughly 15-20 critical registers,
- a status dashboard,
- serial + MQTT configuration,
- MQTT Discovery for telemetry,
- no write support in the first deployable build.

Reasoning:

- the highest value is stable local Modbus polling and clean MQTT delivery into HA,
- the write path should come only after real-device validation of the register map,
- this reduces the risk of accidentally pushing unsafe inverter settings too early.

## Execution Plan for This Repo

1. Bootstrap the Go project and directory layout.
2. Add YAML config model and load/save logic.
3. Implement the MVP register catalog.
4. Build a stable Modbus RTU client with reconnect behavior.
5. Add application state snapshots and a basic HTTP dashboard.
6. Add MQTT publishing and HA Discovery.
7. Extend the UI with configuration and diagnostics.
8. Add the safe write path only after the read path is stable.

## Decisions to Lock In Now

Recommended immediate decisions:

- Go as the main runtime,
- one binary with an embedded frontend,
- YAML only for app configuration, not for full register definitions,
- MQTT Discovery as the HA integration path,
- Linux-first builds and CI,
- write support only after the read path is proven stable.

## Open Questions for the Next Iteration

- what is the exact SRNE inverter model, and are all registers from the current HA config valid for it,
- whether `hive-nuc` deployment should be managed by `systemd`,
- whether the web panel should be LAN-accessible or local-only,
- whether MQTT command topics belong in MVP or a later phase,
- whether the project should support one inverter only or be multi-device aware from the start.

## Next Practical Step

The next step should be Phase 0:

- scaffold the Go project,
- pick the minimal HTTP/UI stack,
- add an example config,
- produce the first Linux build,
- add CI,
- create a server stub with an embedded frontend.
