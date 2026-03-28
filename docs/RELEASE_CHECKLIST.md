# Release Checklist

## Before tagging

- Confirm the working tree is clean.
- Run:

```bash
gofmt -w .
go vet ./...
go test ./...
```

- Verify the embedded web UI still loads and renders live telemetry.
- Verify at least one writable setting from the web UI.
- Verify Home Assistant MQTT Discovery still exposes telemetry and controls.
- Update [`CHANGELOG.md`](../CHANGELOG.md) if needed.
- Refresh screenshots in [`docs/screenshots`](screenshots) if the UI changed.

## Tagging

```bash
git tag v0.1.0
git push origin v0.1.0
```

## After the GitHub release job finishes

- Confirm release artifacts exist for Linux `amd64` and `arm64`.
- Confirm `checksums.txt` was attached.
- Smoke test one release archive on a Linux host.
- Check generated release notes and edit them if they need cleanup.

## Recommended manual checks

- Open the web panel in both light and dark mode.
- Verify `GET /api/v1/status`.
- Verify one MQTT command write and its readback.
- Verify Home Assistant entity naming still looks clean on the device page.
