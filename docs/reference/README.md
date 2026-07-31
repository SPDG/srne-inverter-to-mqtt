# Reference Documents

This directory keeps the inverter manuals and protocol documents used to
validate register mappings. The documents remain the property of their
respective publishers and are not covered by this repository's Apache-2.0
license.

## Documents

| File | Applies to | Source | SHA-256 |
| --- | --- | --- | --- |
| [`SRNE_SPI_H3P_User_Manual_V1.0.pdf`](SRNE_SPI_H3P_User_Manual_V1.0.pdf) | SPI-8K-H3P, SPI-10K-H3P, and SPI-12K-H3P. This is the primary hardware manual for the validated `srne-002` installation. | [Original download](https://cdn-files.myshopline.com/file/store/1665806659085/SPI-8-12K-H3P-series-Three-phase-8-12kW-EU-Usermanual-V1-0.pdf) | `0ef9a356b49ced06f9886237eca2a5a0d27dff77f0fbe3cfc9080439eb25b7af` |
| [`SRNE_HESP_SH3_User_Manual_V1.4.pdf`](SRNE_HESP_SH3_User_Manual_V1.4.pdf) | HESP4880SH3, HESP48100SH3, and HESP48120SH3. This is a related hybrid/on-grid platform, not the SPI H3P hardware manual. | [SRNE download](https://www.srnesolar.com/userfiles/files/2026/06/10/SRNE_HESP_48V_8-12K%20SH3%20series_EU_Three-phase_Solar%20Hybrid%20Inverter_User%20Manual_V1.4%5B20250704%5D.pdf) | `648517b7c554f366ad200f8644e613d05606ed05f79bb7cebece2cec346cebc1` |
| [`SRNE_Modbus_Protocol_V1.92.pdf`](SRNE_Modbus_Protocol_V1.92.pdf) | General SRNE energy storage inverter register reference. Firmware-specific behavior must still be validated on hardware. | [Archived download](https://myhomethings.eu/wp-content/uploads/2024/04/SRNE_ModBus_Protokoll_V1.92.pdf) | `e1d9ab4fd75881e2870b75daea1ece8ef89d6aee5a9d97084ae9affc9d91e8d6` |

The newer [SRNE Modbus Protocol V1.96 archive](https://github.com/HotNoob/PythonProtocolGateway/blob/main/documentation/3rdparty/protocols/SRNE.Solar.Charge.Inverter.MODBUS.Protocol1.96.pdf)
was used to identify the SPI H3P RTC (`0x020C-0x020E`) and timed utility-charge
registers (`0xE026-0xE02C`). These addresses were subsequently checked against
the live `srne-002` inverter.

## Model Boundary

The SPI H3P and HESP SH3 families share telemetry and configuration concepts,
but they are not interchangeable:

- SPI H3P is an off-grid storage inverter with AC IN and AC OUT. It supports
  utility bypass and utility battery charging without exporting energy.
- HESP SH3 is a hybrid/on-grid inverter with external CT support, grid export,
  anti-backflow modes, and separate on-grid configuration.

Do not apply HESP grid-export settings to an SPI H3P unit merely because a
register responds. Shared firmware may expose values that are not valid for the
installed power stage or wiring topology.
