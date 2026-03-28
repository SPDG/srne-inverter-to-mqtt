# Contributing

## Scope

This project focuses on a local Linux-hosted bridge between SRNE Modbus RTU devices and Home Assistant over MQTT.

Good contributions include:

- additional verified SRNE register coverage,
- safer write handling,
- MQTT and Home Assistant interoperability improvements,
- Linux deployment improvements,
- UI and documentation improvements backed by real-world usage.

## Development workflow

1. Fork the repository and create a focused branch.
2. Keep changes small and explain the user-facing reason in the PR.
3. Run the local checks before opening a PR:

```bash
gofmt -w .
go vet ./...
go test ./...
```

4. If you touch the web UI, attach updated screenshots when the visual change matters.
5. If you change register mappings or writable settings, note the exact hardware model and how the change was validated.

## Expectations

- Keep all repository artifacts in English.
- Prefer stable Linux serial paths such as `/dev/serial/by-path/...`.
- Do not submit speculative register writes without validation notes.
- Avoid large refactors mixed with protocol changes.

## Release notes

User-visible changes should be reflected in [`CHANGELOG.md`](CHANGELOG.md).
