# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and the project uses Git tags for releases.

## [Unreleased]

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
