# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and the project uses Git tags for releases.

## [Unreleased]

### Added

- Dedicated `spi_h3p` and `hesp_sh3` hardware profiles.
- Per-phase grid voltage and current telemetry for three-phase inverters.
- SPI H3P, HESP SH3, and Modbus reference documents with validated operating notes.
- Modbus stall watchdog coverage on the current polling implementation.
- Managed SPI H3P storm charging with target SOC, current and timeout controls, persistent rollback, web controls, and Home Assistant MQTT Discovery entities.

### Changed

- Legacy `three_phase` configurations now resolve to the safer SPI H3P off-grid profile.
- HESP grid-connected active power is represented as a signed percentage according to the protocol reference.

### Fixed

- SPI H3P no longer exposes HESP-only grid export controls.
- Obsolete HESP grid controls are removed from Home Assistant Discovery when the SPI profile is active.
- SPI operating notes distinguish normal `PV Only` charging from low-SOC recovery charging.
- SOC threshold writes enforce the firmware's validated five-percentage-point safety gaps before sending Modbus commands.
- Storm charge now fails and rolls back when utility-priority charging is rejected instead of treating load-only Hybrid operation as battery charging.

## [0.2.0] - 2026-07-19

### Added

- Transparent RTU-over-TCP and Modbus TCP gateway transports.
- Configurable single-phase and three-phase inverter profiles.
- Three-phase load and grid telemetry, dual-PV string telemetry, and aggregate power sensors.
- SPI-12K-H3P battery, charging, source, bypass, and grid export controls.
- Home Assistant controls and generated dashboard YAML for the new telemetry and settings.

### Changed

- Critical power and BMS charge-limit values are polled more frequently.
- Writable register updates are read back and verified after Modbus writes.
- Charge-current bounds now follow the selected inverter profile.

### Fixed

- Control form edits are preserved while live telemetry refreshes.
- Numeric controls display engineering values instead of raw scaled register values.
- Three-phase load and grid power are calculated from their phase registers.

## [0.1.0] - 2026-03-28

### Added

- Initial public release.
- Single-binary Go service for SRNE Modbus RTU polling over local serial.
- YAML configuration file management.
- Embedded web UI for runtime status, telemetry, safe writes, and service configuration.
- MQTT publishing with Home Assistant Discovery.
- Writable inverter settings exposed as MQTT `select` and `number` entities.
- Linux `amd64` and `arm64` release artifacts through GitHub Actions.
