# Deployment

## Linux host

The intended deployment model is a small Linux machine physically attached to the inverter over USB serial.

Recommended:

- local Modbus RTU over `/dev/serial/by-path/...`,
- local systemd service,
- MQTT broker reachable over LAN or VPN,
- Home Assistant consuming MQTT Discovery remotely.

## Files

- binary: `/opt/srne-inverter-to-mqtt/srne-inverter-to-mqtt`
- config: `/etc/srne-inverter-to-mqtt/config.yaml`
- systemd unit: `/etc/systemd/system/srne-inverter-to-mqtt.service`

A ready-to-use unit file is included at [`deploy/systemd/srne-inverter-to-mqtt.service`](../deploy/systemd/srne-inverter-to-mqtt.service).

## Install

1. Create directories:

```bash
sudo mkdir -p /opt/srne-inverter-to-mqtt /etc/srne-inverter-to-mqtt
```

2. Copy the binary and example config:

```bash
sudo install -m 0755 srne-inverter-to-mqtt /opt/srne-inverter-to-mqtt/srne-inverter-to-mqtt
sudo install -m 0644 configs/config.example.yaml /etc/srne-inverter-to-mqtt/config.yaml
```

3. Edit the config:

- set the real serial port,
- set MQTT broker credentials,
- set the listen address for the embedded panel.

4. Install the systemd unit:

```bash
sudo install -m 0644 deploy/systemd/srne-inverter-to-mqtt.service /etc/systemd/system/srne-inverter-to-mqtt.service
sudo systemctl daemon-reload
sudo systemctl enable --now srne-inverter-to-mqtt.service
```

5. Verify:

```bash
systemctl status srne-inverter-to-mqtt.service
curl http://127.0.0.1:8080/healthz
```

## Notes

- Prefer `/dev/serial/by-path/...` or `/dev/serial/by-id/...` over `/dev/ttyUSB*`.
- If port `8080` is already occupied, change `http.listen` in the YAML config.
- The service publishes MQTT availability and Home Assistant Discovery automatically after connect.
