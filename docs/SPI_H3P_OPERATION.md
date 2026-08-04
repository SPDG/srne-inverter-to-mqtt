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

The 2026-07-26 live check returned `0.0 V` for grid phases A, B, and C and
`0.00 Hz` for the grid frequency because utility power was not reaching AC IN.
After the upstream connection was restored, the same validated Modbus registers
reported approximately 233-235 V per phase and 50 Hz. Grid-connected export
mode was not required and must not be enabled to work around a missing AC input.

The application exposes these controls only for the `hesp_sh3` profile. They
are intentionally hidden for `spi_h3p`, even when the shared firmware responds
at the same register addresses.

### Grid Power Direction

The signed phase power registers `0x023A-0x023C` use positive values for power
imported from utility and negative values for power flowing toward the grid.
This follows the SRNE Modbus protocol definition and was confirmed live on
`srne-002` during Storm Charge on 2026-08-04: positive raw values on all three
phases coincided with battery charging and utility consumption. The synthetic
`grid_power` sensor is the signed sum of these three phase values and uses the
same direction convention.

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
- Switch to utility SOC: high enough to switch before the BMS low-capacity stop
- Switch to inverter SOC: above the utility-switch threshold to provide
  hysteresis
- Grid charge current: a value supported by both the inverter and BMS

`PV Only` / `OSO` prevents utility charging during normal operation. It does
not enable or disable utility bypass. SRNE firmware may override the normal
charging priority during BMS low-capacity recovery or lithium-battery
activation, using AC IN to reach the configured restart SOC. This behavior was
observed on the previous single-phase SRNE inverter and must be validated on
the SPI H3P after AC IN is restored.

Keep `PV Only` when grid charging is wanted only as a protective low-SOC
recovery mechanism. Use `Hybrid` only when utility should also supplement weak
PV during otherwise normal operation.

## Managed Storm Charge

The application provides a managed `Storm Charge` operation for charging from
utility before an expected outage. It is intentionally an application-level
workflow rather than a single Modbus register.

On start, the service persists and then temporarily changes:

1. battery charge cut-off SOC to the selected target,
2. mains charge current limit to the selected maximum,
3. all three timed utility-charge slots to `00:01-23:59`,
4. timed utility charging to enabled.

The entire seven-word timed-charge schedule is captured before the first
change. If any write is rejected, startup fails and all captured settings are
restored.

The manual states that enabling timed utility charging changes the power-supply
mode during the configured period. This is the documented mechanism intended
to move AC OUT to utility bypass and permit utility battery charging together.

The previous values are restored after the target SOC is reached, on manual
cancellation, after the configured timeout, after grid loss, or when a fault is
reported. An active session survives process restarts through the automatic
`<config>.storm-charge-state.yaml` recovery file. Manual writes to the target
SOC and mains-current registers and service configuration changes are rejected
while the session is active.

The selected current is a battery-side limit. The UI estimates power as current
multiplied by the live battery voltage; actual power remains subject to PV/load
conditions, inverter derating, and the BMS charge-current limit. SPI-12K-H3P
utility charging is capped at 120 A.

Live validation on `srne-002` on 2026-07-31 established two firmware-specific
limitations:

- `Utility Priority` (`CUb`, raw value 1) was rejected with Modbus exception 3
  even after utility bypass had been stable for 25 seconds.
- `Hybrid` (`SNU`, raw value 2) was accepted, but it only supplemented AC OUT.
  With 463 W PV, 477 W grid input, and 999 W load, the battery was still
  discharging instead of charging toward the selected target.

Modbus protocol V1.96 identifies the alternative function as parameters 40-46:

| Register | Name | Encoding |
| --- | --- | --- |
| `0xE026` / `0xE027` | Utility charge slot 1 start / end | hour in high byte, minute in low byte |
| `0xE028` / `0xE029` | Utility charge slot 2 start / end | hour in high byte, minute in low byte |
| `0xE02A` / `0xE02B` | Utility charge slot 3 start / end | hour in high byte, minute in low byte |
| `0xE02C` | Timed utility charging | `0` disabled, `1` enabled |

Live reads on `srne-002` confirmed that this seven-register block is present;
all values were initially zero. A live write also established that the firmware
normalizes a `00:00` start to `00:01` and validates all three slot pairs when
the function is enabled. Storm Charge now uses this function instead of
changing output or charger source priority.

The completed live test on 2026-07-31 used an 80% target, 10 A utility limit,
and five-minute timeout. Before activation, battery current was `+14.4 A`
(discharging). After the timed function was enabled, it changed to `-16.4 A`
(charging), with approximately 1.49 kW imported from utility, 227 W from PV,
and 984 W consumed by the load. Manual cancellation restored the original
seven zero words; a direct Modbus read after cancellation confirmed
`0xE026-0xE02C = [0, 0, 0, 0, 0, 0, 0]`.

## Inverter Clock

The inverter RTC is stored as three contiguous read/write registers:

| Register | High byte | Low byte |
| --- | --- | --- |
| `0x020C` | year since 2000 | month |
| `0x020D` | day | hour |
| `0x020E` | minute | second |

The Controls view reads these registers on demand and can write a local wall
clock supplied by the browser. The service does not synchronize it
automatically, because the host timezone may differ from the inverter's local
timezone. The original live value was invalid (`2022-13-14 22:25:52`), which
confirmed that the factory RTC had not been configured correctly.

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
| Charger source priority | PV Only; use Hybrid only for normal grid-assisted charging |

These values are operational starting points, not universal battery limits.
The BMS charge and discharge limits remain authoritative.

### Validated SOC Threshold Constraints

The generic SRNE Modbus V1.92 register table documents these absolute ranges:

| Register | Setting | Documented range |
| --- | --- | ---: |
| `0xE00F` | Battery discharge cut-off SOC | 0-100% |
| `0xE01E` | Battery low SOC alarm | 0-100% |
| `0xE01F` | Switch to utility SOC | 0-100% |
| `0xE020` | Switch to inverter SOC | 1-100% |

The table does not document cross-register constraints. Live tests on the SPI
H3P established that its firmware additionally requires a five-percentage-point
gap in this sequence:

```text
Battery discharge cut-off SOC
    + 5 percentage points <= Battery low SOC alarm
    + 5 percentage points <= Switch to inverter SOC
```

With cut-off SOC at 10%, lowering the alarm from 15% to 14% returned Modbus
exception 3 (`illegal data value`). With the alarm at 16%, an inverter-switch
threshold of 21% was accepted and 20% was rejected. The settings were restored
to 10% cut-off, 15% alarm, and 20% inverter-switch SOC after the test.

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
