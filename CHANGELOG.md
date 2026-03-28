# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and the project uses Git tags for releases.

## [Unreleased]

### Added

- Release-ready embedded web panel with light and dark themes.
- Home Assistant MQTT Discovery for telemetry and writable controls.
- Linux CI and tagged release workflows.

## [0.1.0] - 2026-03-28

### Added

- Initial public release.
- Single-binary Go service for SRNE Modbus RTU polling over local serial.
- YAML configuration file management.
- Embedded web UI for runtime status, telemetry, safe writes, and service configuration.
- MQTT publishing with Home Assistant Discovery.
- Writable inverter settings exposed as MQTT `select` and `number` entities.
- Linux `amd64` and `arm64` release artifacts through GitHub Actions.
