# SPI H3P Operation Notes

This document records the hardware and Modbus behavior validated for the
`SPI-12K-H3P` installation named `srne-002`.

## Installation Topology

- Model: `SPI-12K-H3P`
- Battery system: 48 V with BMS communication
- Utility connection: three phases connected to AC IN
- Protected loads: connected only to AC OUT
- External CT: not installed and not supported by the packaged OEM system
- Modbus transport: Modbus TCP gateway

This topology does not need an external CT. The inverter only has to measure
power that passes through its own AC IN and AC OUT terminals.

## No-export Behavior

SPI H3P is an off-grid storage inverter. Its AC input is used for utility bypass
and battery charging. The SPI H3P manual describes an `AC output reverse`
protection that prevents inverter output from backfeeding the bypass AC input.

The following live values were read from `srne-002` on 2026-07-26:

| Register | Address | Raw value | Interpretation |
| --- | ---: | ---: | --- |
| Grid operating mode | `0xE037` | `0` | Grid-connected operation disabled |
| Grid-connected active power | `0xE400` | `0` | No configured export |
| Today energy export | `0xF02C` | `0` | No recorded export |
| Total energy export | `0xF032` | `0` | No recorded export |

The same live check returned `0.0 V` for grid phases A, B, and C and `0.00 Hz`
for the grid frequency. This means the inverter did not detect utility voltage
at AC IN. Grid-connected export mode must not be enabled to work around this.
Check the upstream supply, the OEM contactor, the internal AC IN breaker, and
the three-phase wiring before changing software settings.

The application exposes these controls only for the `hesp_sh3` profile. They
are intentionally hidden for `spi_h3p`, even when the shared firmware responds
at the same register addresses.

## Utility Bypass and Recovery Charging

Utility bypass is separate from grid-connected export. The desired operating
sequence is:

1. PV powers AC OUT and charges the battery when energy is available.
2. The battery supplements PV while its SOC is above the utility-switch point.
3. At the configured low-SOC threshold, AC OUT switches to utility bypass.
4. Utility may charge the battery according to the selected charging mode.
5. The inverter returns to PV/battery output after reaching the configured
   restart SOC.

For this behavior, use:

- AC output source priority: `SBU` (PV, battery, then utility)
- Battery charging mode: `SNU` / Hybrid when utility should supplement weak PV
- Switch to utility SOC: high enough to switch before the BMS low-capacity stop
- Switch to inverter SOC: above the utility-switch threshold to provide
  hysteresis
- Grid charge current: a value supported by both the inverter and BMS

`PV Only` / `OSO` explicitly prevents utility battery charging. It does not
enable or disable utility bypass.

## Validated Low-SOC Incident

During the observed failure to recover from a low battery, the relevant values
were:

| Setting or state | Value |
| --- | ---: |
| Battery SOC | 11% |
| Switch to utility SOC | 11% |
| Switch to inverter SOC | 25% |
| Battery discharge cut-off SOC | 10% |
| Battery low SOC alarm | 18% |
| Charger source priority | PV Only |
| Machine state | Standby |
| Fault code 1 | 30 (`BatCapacityLow1`) |
| Fault code 2 | 32 (`BatCapacityLowStop`) |

The utility-switch threshold was too close to the low-capacity shutdown point.
The BMS/inverter could enter `BatCapacityLowStop` before completing the bypass
transition. A safer starting point is:

| Setting | Suggested starting value |
| --- | ---: |
| Switch to utility SOC | 20% |
| Switch to inverter SOC | 30% |
| Battery low SOC alarm | 15-18% |
| Battery discharge cut-off SOC | 10% |
| Charger source priority | Hybrid |

These values are operational starting points, not universal battery limits.
The BMS charge and discharge limits remain authoritative.

## Model Identification

The general SRNE protocol defines:

- model text at `0x000C-0x0013`,
- software and hardware versions at `0x0014-0x0017`,
- serial number at `0x0018-0x0019`.

On this SPI H3P firmware, the model and serial-number ranges return zeros.
Software/hardware version registers return data, but their encoding does not
match the older public protocol examples. Select `spi_h3p` explicitly in the
configuration instead of relying on automatic detection.

## References

- [SPI H3P User Manual](reference/SRNE_SPI_H3P_User_Manual_V1.0.pdf)
- [HESP SH3 User Manual](reference/SRNE_HESP_SH3_User_Manual_V1.4.pdf)
- [SRNE Modbus Protocol V1.92](reference/SRNE_Modbus_Protocol_V1.92.pdf)
- [Reference document provenance and checksums](reference/README.md)
