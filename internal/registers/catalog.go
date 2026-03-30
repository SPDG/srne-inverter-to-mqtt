package registers

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type PollGroup string

const (
	GroupFast PollGroup = "fast"
	GroupSlow PollGroup = "slow"
)

type ValueType string

const (
	TypeUint16 ValueType = "uint16"
	TypeInt16  ValueType = "int16"
	TypeUint32 ValueType = "uint32"
	TypeInt32  ValueType = "int32"
)

type WordOrder string

const (
	WordOrderHighLow WordOrder = "high-low"
	WordOrderLowHigh WordOrder = "low-high"
)

type Register struct {
	ID             string
	Name           string
	Address        uint16
	Count          uint16
	Type           ValueType
	WordOrder      WordOrder
	Synthetic      bool
	Scale          float64
	Precision      int
	Unit           string
	DeviceClass    string
	StateClass     string
	Icon           string
	Component      string
	Group          PollGroup
	Writable       bool
	WriteOnly      bool
	Entity         string
	EntityCategory string
	Enum           map[int64]string
	WriteMin       float64
	WriteMax       float64
	WriteStep      float64
}

type DecodedValue struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Address        uint16    `json:"address"`
	Group          PollGroup `json:"group"`
	Component      string    `json:"component"`
	Entity         string    `json:"entity"`
	EntityCategory string    `json:"entityCategory,omitempty"`
	Writable       bool      `json:"writable"`
	WriteOnly      bool      `json:"writeOnly,omitempty"`
	Unit           string    `json:"unit,omitempty"`
	DeviceClass    string    `json:"deviceClass,omitempty"`
	StateClass     string    `json:"stateClass,omitempty"`
	Icon           string    `json:"icon,omitempty"`
	Raw            int64     `json:"raw"`
	Value          any       `json:"value"`
	Rendered       string    `json:"rendered"`
	WriteMin       float64   `json:"writeMin,omitempty"`
	WriteMax       float64   `json:"writeMax,omitempty"`
	WriteStep      float64   `json:"writeStep,omitempty"`
	Options        []Option  `json:"options,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Option struct {
	Raw   int64  `json:"raw"`
	Label string `json:"label"`
}

type ReadRange struct {
	Start     uint16
	Count     uint16
	Registers []Register
}

func Catalog() []Register {
	return []Register{
		{
			ID:          "battery_soc",
			Name:        "Battery SOC",
			Address:     0x0100,
			Count:       1,
			Type:        TypeUint16,
			Scale:       1,
			Precision:   0,
			Unit:        "%",
			DeviceClass: "battery",
			StateClass:  "measurement",
			Icon:        "mdi:battery",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "battery_voltage",
			Name:        "Battery Voltage",
			Address:     0x0101,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "V",
			DeviceClass: "voltage",
			StateClass:  "measurement",
			Icon:        "mdi:car-battery",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "battery_current",
			Name:        "Battery Current",
			Address:     0x0102,
			Count:       1,
			Type:        TypeInt16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "A",
			DeviceClass: "current",
			StateClass:  "measurement",
			Icon:        "mdi:current-dc",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "battery_temperature",
			Name:        "Battery Temperature",
			Address:     0x0103,
			Count:       1,
			Type:        TypeInt16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "°C",
			DeviceClass: "temperature",
			StateClass:  "measurement",
			Icon:        "mdi:thermometer",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:          "pv_voltage",
			Name:        "PV Voltage",
			Address:     0x0107,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "V",
			DeviceClass: "voltage",
			StateClass:  "measurement",
			Icon:        "mdi:solar-power",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "pv_current",
			Name:        "PV Current",
			Address:     0x0108,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "A",
			DeviceClass: "current",
			StateClass:  "measurement",
			Icon:        "mdi:solar-power",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "pv_power",
			Name:        "PV Power",
			Address:     0x0109,
			Count:       1,
			Type:        TypeUint16,
			Scale:       1,
			Precision:   0,
			Unit:        "W",
			DeviceClass: "power",
			StateClass:  "measurement",
			Icon:        "mdi:white-balance-sunny",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "pv_total_power",
			Name:        "PV Total Power",
			Address:     0x010A,
			Count:       1,
			Type:        TypeUint16,
			Scale:       1,
			Precision:   0,
			Unit:        "W",
			DeviceClass: "power",
			StateClass:  "measurement",
			Icon:        "mdi:solar-power-variant",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:          "pv_ac_power",
			Name:        "PV+AC Power",
			Address:     0x010E,
			Count:       1,
			Type:        TypeUint16,
			Scale:       1,
			Precision:   0,
			Unit:        "W",
			DeviceClass: "power",
			StateClass:  "measurement",
			Icon:        "mdi:solar-power-variant",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:        "charge_state",
			Name:      "Charge State",
			Address:   0x010B,
			Count:     1,
			Type:      TypeUint16,
			Precision: 0,
			Icon:      "mdi:battery-charging",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "diagnostic",
			Enum: map[int64]string{
				0: "Charge Off",
				1: "Quick Charge",
				2: "Constant Voltage Charge",
				4: "Float Charge",
				5: "Reserved",
				6: "Li Battery Active",
				8: "Full",
			},
		},
		{
			ID:        "fault_code_1",
			Name:      "Fault Code 1",
			Address:   0x0204,
			Count:     1,
			Type:      TypeUint16,
			Precision: 0,
			Icon:      "mdi:alert-circle-outline",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "diagnostic",
		},
		{
			ID:        "fault_code_2",
			Name:      "Fault Code 2",
			Address:   0x0205,
			Count:     1,
			Type:      TypeUint16,
			Precision: 0,
			Icon:      "mdi:alert-circle-outline",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "diagnostic",
		},
		{
			ID:          "load_power",
			Name:        "Load Power",
			Address:     0x021B,
			Count:       1,
			Type:        TypeUint16,
			Scale:       1,
			Precision:   0,
			Unit:        "W",
			DeviceClass: "power",
			StateClass:  "measurement",
			Icon:        "mdi:home-lightning-bolt-outline",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "grid_voltage",
			Name:        "Grid Voltage",
			Address:     0x0213,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "V",
			DeviceClass: "voltage",
			StateClass:  "measurement",
			Icon:        "mdi:transmission-tower",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "grid_current",
			Name:        "Grid Current",
			Address:     0x0214,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "A",
			DeviceClass: "current",
			StateClass:  "measurement",
			Icon:        "mdi:transmission-tower",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "grid_frequency",
			Name:        "Grid Frequency",
			Address:     0x0215,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.01,
			Precision:   2,
			Unit:        "Hz",
			DeviceClass: "frequency",
			StateClass:  "measurement",
			Icon:        "mdi:sine-wave",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "inverter_voltage",
			Name:        "Inverter Voltage",
			Address:     0x0216,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "V",
			DeviceClass: "voltage",
			StateClass:  "measurement",
			Icon:        "mdi:flash",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "inverter_frequency",
			Name:        "Inverter Frequency",
			Address:     0x0218,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.01,
			Precision:   2,
			Unit:        "Hz",
			DeviceClass: "frequency",
			StateClass:  "measurement",
			Icon:        "mdi:sine-wave",
			Component:   "sensor",
			Group:       GroupFast,
			Entity:      "diagnostic",
		},
		{
			ID:          "load_current",
			Name:        "Load Current",
			Address:     0x0219,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "A",
			DeviceClass: "current",
			StateClass:  "measurement",
			Icon:        "mdi:current-ac",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:        "load_apparent_power",
			Name:      "Load Apparent Power",
			Address:   0x021C,
			Count:     1,
			Type:      TypeUint16,
			Scale:     1,
			Precision: 0,
			Unit:      "VA",
			Icon:      "mdi:flash-outline",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "diagnostic",
		},
		{
			ID:        "machine_state",
			Name:      "Machine State",
			Address:   0x0210,
			Count:     1,
			Type:      TypeUint16,
			Precision: 0,
			Icon:      "mdi:state-machine",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "diagnostic",
			Enum: map[int64]string{
				0:  "Power-on Delay",
				1:  "Standby",
				2:  "Initialization",
				3:  "Soft Start",
				4:  "AC Power Operation",
				5:  "Inverter Operation",
				6:  "Inverter to AC Power",
				7:  "AC Power to Inverter",
				8:  "Battery Activation",
				9:  "Manual Shutdown",
				10: "Fault",
			},
		},
		{
			ID:          "pv_charge_current_setup",
			Name:        "PV Charge Current Setup",
			Address:     0xE001,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "A",
			DeviceClass: "current",
			Icon:        "mdi:solar-power",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "config",
			Writable:    true,
			WriteMin:    0,
			WriteMax:    100,
			WriteStep:   0.1,
		},
		{
			ID:        "battery_type",
			Name:      "Battery Type",
			Address:   0xE004,
			Count:     1,
			Type:      TypeUint16,
			Precision: 0,
			Icon:      "mdi:battery-heart-variant",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "config",
			Enum: map[int64]string{
				0:  "User Defined",
				1:  "SLD",
				2:  "FLD",
				3:  "GEL",
				4:  "LFP x14",
				5:  "LFP x15",
				6:  "LFP x16",
				7:  "LFP x7",
				8:  "LFP x8",
				9:  "LFP x9",
				10: "Ternary Li x7",
				11: "Ternary Li x8",
				12: "Ternary Li x13",
				13: "Ternary Li x14",
			},
		},
		{
			ID:          "boost_charge_voltage",
			Name:        "Boost Charge Voltage",
			Address:     0xE008,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.2,
			Precision:   2,
			Unit:        "V",
			DeviceClass: "voltage",
			Icon:        "mdi:car-battery",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "config",
		},
		{
			ID:          "float_charge_voltage",
			Name:        "Float Charge Voltage",
			Address:     0xE009,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.2,
			Precision:   2,
			Unit:        "V",
			DeviceClass: "voltage",
			Icon:        "mdi:car-battery",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "config",
		},
		{
			ID:          "boost_return_voltage",
			Name:        "Boost Return Voltage",
			Address:     0xE00A,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.2,
			Precision:   2,
			Unit:        "V",
			DeviceClass: "voltage",
			Icon:        "mdi:car-battery",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "config",
		},
		{
			ID:        "boost_charging_time",
			Name:      "Boost Charging Time",
			Address:   0xE012,
			Count:     1,
			Type:      TypeUint16,
			Scale:     1,
			Precision: 0,
			Unit:      "min",
			Icon:      "mdi:timer-outline",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "config",
		},
		{
			ID:        "output_source_priority",
			Name:      "Output Source Priority",
			Address:   0xE204,
			Count:     1,
			Type:      TypeUint16,
			Precision: 0,
			Icon:      "mdi:home-switch-outline",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "config",
			Writable:  true,
			WriteMin:  0,
			WriteMax:  2,
			WriteStep: 1,
			Enum: map[int64]string{
				0: "Solar",
				1: "Utility",
				2: "Solar, Battery, Utility",
			},
		},
		{
			ID:          "mains_charge_current_limit",
			Name:        "Mains Charge Current Limit",
			Address:     0xE205,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "A",
			DeviceClass: "current",
			Icon:        "mdi:transmission-tower",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "config",
			Writable:    true,
			WriteMin:    0,
			WriteMax:    100,
			WriteStep:   0.1,
		},
		{
			ID:        "reset_machine",
			Name:      "Reset Machine",
			Address:   0xDF01,
			Count:     1,
			Type:      TypeUint16,
			Precision: 0,
			Icon:      "mdi:restart-alert",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "config",
			Writable:  true,
			WriteOnly: true,
			WriteMin:  1,
			WriteMax:  1,
			WriteStep: 1,
			Enum: map[int64]string{
				1: "Reset",
			},
		},
		{
			ID:        "charger_source_priority",
			Name:      "Charger Source Priority",
			Address:   0xE20F,
			Count:     1,
			Type:      TypeUint16,
			Precision: 0,
			Icon:      "mdi:battery-charging-outline",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "config",
			Writable:  true,
			WriteMin:  0,
			WriteMax:  3,
			WriteStep: 1,
			Enum: map[int64]string{
				0: "PV Priority",
				1: "Utility Priority",
				2: "Hybrid",
				3: "PV Only",
			},
		},
		{
			ID:        "power_saving_mode",
			Name:      "Power Saving Mode",
			Address:   0xE20C,
			Count:     1,
			Type:      TypeUint16,
			Precision: 0,
			Icon:      "mdi:leaf",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "config",
			Enum: map[int64]string{
				0: "Disabled",
				1: "Enabled",
			},
		},
		{
			ID:          "dc_to_dc_temperature",
			Name:        "DC to DC Temperature",
			Address:     0x0220,
			Count:       1,
			Type:        TypeInt16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "°C",
			DeviceClass: "temperature",
			StateClass:  "measurement",
			Icon:        "mdi:thermometer",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:          "dc_to_ac_temperature",
			Name:        "DC to AC Temperature",
			Address:     0x0221,
			Count:       1,
			Type:        TypeInt16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "°C",
			DeviceClass: "temperature",
			StateClass:  "measurement",
			Icon:        "mdi:thermometer",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:          "transformer_temperature",
			Name:        "Transformer Temperature",
			Address:     0x0222,
			Count:       1,
			Type:        TypeInt16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "°C",
			DeviceClass: "temperature",
			StateClass:  "measurement",
			Icon:        "mdi:thermometer",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:          "grid_power",
			Name:        "Grid Power",
			Address:     0x023A,
			Count:       1,
			Type:        TypeUint16,
			Scale:       1,
			Precision:   0,
			Unit:        "W",
			DeviceClass: "power",
			StateClass:  "measurement",
			Icon:        "mdi:transmission-tower",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:          "today_energy_export",
			Name:        "Today Energy Export",
			Address:     0xF02C,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "kWh",
			DeviceClass: "energy",
			StateClass:  "total_increasing",
			Icon:        "mdi:transmission-tower-import",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:         "today_battery_charge_ah",
			Name:       "Today Battery Charge",
			Address:    0xF02D,
			Count:      1,
			Type:       TypeUint16,
			Scale:      1,
			Precision:  0,
			Unit:       "Ah",
			StateClass: "total_increasing",
			Icon:       "mdi:battery-plus",
			Component:  "sensor",
			Group:      GroupSlow,
			Entity:     "diagnostic",
		},
		{
			ID:         "today_battery_discharge_ah",
			Name:       "Today Battery Discharge",
			Address:    0xF02E,
			Count:      1,
			Type:       TypeUint16,
			Scale:      1,
			Precision:  0,
			Unit:       "Ah",
			StateClass: "total_increasing",
			Icon:       "mdi:battery-minus",
			Component:  "sensor",
			Group:      GroupSlow,
			Entity:     "diagnostic",
		},
		{
			ID:          "today_production",
			Name:        "Today Production",
			Address:     0xF02F,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "kWh",
			DeviceClass: "energy",
			StateClass:  "total_increasing",
			Icon:        "mdi:solar-power",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:          "today_load_consumption",
			Name:        "Today Load Consumption",
			Address:     0xF030,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "kWh",
			DeviceClass: "energy",
			StateClass:  "total_increasing",
			Icon:        "mdi:home-lightning-bolt-outline",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:          "total_energy_export",
			Name:        "Total Energy Export",
			Address:     0xF032,
			Count:       2,
			Type:        TypeUint32,
			WordOrder:   WordOrderLowHigh,
			Scale:       0.1,
			Precision:   1,
			Unit:        "kWh",
			DeviceClass: "energy",
			StateClass:  "total_increasing",
			Icon:        "mdi:transmission-tower-import",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:         "total_battery_charge_ah",
			Name:       "Total Battery Charge",
			Address:    0xF034,
			Count:      2,
			Type:       TypeUint32,
			WordOrder:  WordOrderLowHigh,
			Scale:      1,
			Precision:  0,
			Unit:       "Ah",
			StateClass: "total_increasing",
			Icon:       "mdi:battery-plus",
			Component:  "sensor",
			Group:      GroupSlow,
			Entity:     "diagnostic",
		},
		{
			ID:         "total_battery_discharge_ah",
			Name:       "Total Battery Discharge",
			Address:    0xF036,
			Count:      2,
			Type:       TypeUint32,
			WordOrder:  WordOrderLowHigh,
			Scale:      1,
			Precision:  0,
			Unit:       "Ah",
			StateClass: "total_increasing",
			Icon:       "mdi:battery-minus",
			Component:  "sensor",
			Group:      GroupSlow,
			Entity:     "diagnostic",
		},
		{
			ID:          "total_production",
			Name:        "Total Production",
			Address:     0xF038,
			Count:       2,
			Type:        TypeUint32,
			WordOrder:   WordOrderLowHigh,
			Scale:       0.1,
			Precision:   1,
			Unit:        "kWh",
			DeviceClass: "energy",
			StateClass:  "total_increasing",
			Icon:        "mdi:solar-power",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:          "total_load_consumption",
			Name:        "Total Load Consumption",
			Address:     0xF03A,
			Count:       2,
			Type:        TypeUint32,
			WordOrder:   WordOrderLowHigh,
			Scale:       0.1,
			Precision:   1,
			Unit:        "kWh",
			DeviceClass: "energy",
			StateClass:  "total_increasing",
			Icon:        "mdi:home-lightning-bolt-outline",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:         "today_battery_grid_charge_ah",
			Name:       "Today Battery Grid Charge",
			Address:    0xF03C,
			Count:      1,
			Type:       TypeUint16,
			Scale:      1,
			Precision:  0,
			Unit:       "Ah",
			StateClass: "total_increasing",
			Icon:       "mdi:battery-plus",
			Component:  "sensor",
			Group:      GroupSlow,
			Entity:     "diagnostic",
		},
		{
			ID:          "today_energy_import",
			Name:        "Today Energy Import",
			Address:     0xF03D,
			Count:       1,
			Type:        TypeUint16,
			Scale:       0.1,
			Precision:   1,
			Unit:        "kWh",
			DeviceClass: "energy",
			StateClass:  "total_increasing",
			Icon:        "mdi:transmission-tower-export",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:         "total_battery_grid_charge_ah",
			Name:       "Total Battery Grid Charge",
			Address:    0xF046,
			Count:      2,
			Type:       TypeUint32,
			WordOrder:  WordOrderLowHigh,
			Scale:      1,
			Precision:  0,
			Unit:       "Ah",
			StateClass: "total_increasing",
			Icon:       "mdi:battery-plus",
			Component:  "sensor",
			Group:      GroupSlow,
			Entity:     "diagnostic",
		},
		{
			ID:          "total_energy_import",
			Name:        "Total Energy Import",
			Address:     0xF048,
			Count:       2,
			Type:        TypeUint32,
			WordOrder:   WordOrderLowHigh,
			Scale:       0.1,
			Precision:   1,
			Unit:        "kWh",
			DeviceClass: "energy",
			StateClass:  "total_increasing",
			Icon:        "mdi:transmission-tower-export",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:          "system_energy_losses_total",
			Name:        "System Energy Losses Total",
			Address:     0xFFF0,
			Count:       0,
			Type:        TypeUint16,
			Synthetic:   true,
			Scale:       1,
			Precision:   1,
			Unit:        "kWh",
			DeviceClass: "energy",
			StateClass:  "total_increasing",
			Icon:        "mdi:transmission-tower-off",
			Component:   "sensor",
			Group:       GroupSlow,
			Entity:      "diagnostic",
		},
		{
			ID:        "system_energy_efficiency_total",
			Name:      "System Energy Efficiency Total",
			Address:   0xFFF1,
			Count:     0,
			Type:      TypeUint16,
			Synthetic: true,
			Scale:     1,
			Precision: 1,
			Unit:      "%",
			Icon:      "mdi:percent-outline",
			Component: "sensor",
			Group:     GroupSlow,
			Entity:    "diagnostic",
		},
	}
}

func ByGroup(group PollGroup) []Register {
	catalog := Catalog()
	filtered := make([]Register, 0, len(catalog))
	for _, reg := range catalog {
		if reg.Group == group && !reg.WriteOnly && !reg.Synthetic {
			filtered = append(filtered, reg)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Address < filtered[j].Address
	})

	return filtered
}

func BuildReadPlan(group PollGroup) []ReadRange {
	return BuildReadPlanForRegisters(ByGroup(group))
}

func BuildCriticalFastReadPlan() []ReadRange {
	critical := map[string]struct{}{
		"battery_soc":     {},
		"battery_voltage": {},
		"battery_current": {},
		"pv_voltage":      {},
		"pv_current":      {},
		"pv_power":        {},
		"load_power":      {},
	}

	filtered := make([]Register, 0, len(critical))
	for _, reg := range ByGroup(GroupFast) {
		if _, ok := critical[reg.ID]; ok {
			filtered = append(filtered, reg)
		}
	}

	return BuildReadPlanForRegisters(filtered)
}

func BuildReadPlanForRegisters(regs []Register) []ReadRange {
	if len(regs) == 0 {
		return nil
	}

	filtered := append([]Register(nil), regs...)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Address < filtered[j].Address
	})

	ranges := make([]ReadRange, 0)
	current := ReadRange{
		Start:     filtered[0].Address,
		Count:     filtered[0].Count,
		Registers: []Register{filtered[0]},
	}

	for _, reg := range filtered[1:] {
		currentEnd := current.Start + current.Count
		regEnd := reg.Address + reg.Count

		if reg.Address <= currentEnd {
			if regEnd > currentEnd {
				current.Count = regEnd - current.Start
			}
			current.Registers = append(current.Registers, reg)
			continue
		}

		ranges = append(ranges, current)
		current = ReadRange{
			Start:     reg.Address,
			Count:     reg.Count,
			Registers: []Register{reg},
		}
	}

	ranges = append(ranges, current)
	return ranges
}

func WordsFromBytes(data []byte, expectedWords uint16) ([]uint16, error) {
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("invalid modbus payload length %d", len(data))
	}

	words := make([]uint16, len(data)/2)
	for i := 0; i < len(words); i++ {
		words[i] = binary.BigEndian.Uint16(data[i*2 : i*2+2])
	}

	if len(words) < int(expectedWords) {
		return nil, fmt.Errorf("short payload: got %d words, expected %d", len(words), expectedWords)
	}

	return words, nil
}

func (r Register) Decode(words []uint16, now time.Time) (DecodedValue, error) {
	if len(words) < int(r.Count) {
		return DecodedValue{}, fmt.Errorf("register %s expected %d words, got %d", r.ID, r.Count, len(words))
	}

	raw, err := r.rawValue(words[:r.Count])
	if err != nil {
		return DecodedValue{}, err
	}

	var value any
	var rendered string

	if len(r.Enum) > 0 {
		value = enumValue(r.Enum, raw)
		rendered = value.(string)
	} else {
		scaled := float64(raw) * scaleOrOne(r.Scale)
		if r.Precision == 0 && math.Abs(scaled-math.Round(scaled)) < 0.000001 {
			rounded := int64(math.Round(scaled))
			value = rounded
			rendered = strconv.FormatInt(rounded, 10)
		} else {
			value = roundFloat(scaled, r.Precision)
			rendered = strconv.FormatFloat(value.(float64), 'f', r.Precision, 64)
		}
	}

	return DecodedValue{
		ID:             r.ID,
		Name:           r.Name,
		Address:        r.Address,
		Group:          r.Group,
		Component:      r.Component,
		Entity:         r.Entity,
		EntityCategory: r.EntityCategory,
		Writable:       r.Writable,
		WriteOnly:      r.WriteOnly,
		Unit:           r.Unit,
		DeviceClass:    r.DeviceClass,
		StateClass:     r.StateClass,
		Icon:           r.Icon,
		Raw:            raw,
		Value:          value,
		Rendered:       rendered,
		WriteMin:       r.WriteMin,
		WriteMax:       r.WriteMax,
		WriteStep:      r.WriteStep,
		Options:        writeOptions(r),
		UpdatedAt:      now,
	}, nil
}

func (r Register) rawValue(words []uint16) (int64, error) {
	switch r.Type {
	case TypeUint16:
		return int64(words[0]), nil
	case TypeInt16:
		return int64(int16(words[0])), nil
	case TypeUint32:
		if len(words) < 2 {
			return 0, fmt.Errorf("register %s needs 2 words", r.ID)
		}
		return int64(joinWords32(words[0], words[1], r.WordOrder)), nil
	case TypeInt32:
		if len(words) < 2 {
			return 0, fmt.Errorf("register %s needs 2 words", r.ID)
		}
		return int64(int32(joinWords32(words[0], words[1], r.WordOrder))), nil
	default:
		return 0, fmt.Errorf("unsupported value type %s", r.Type)
	}
}

func joinWords32(first uint16, second uint16, order WordOrder) uint32 {
	if order == WordOrderLowHigh {
		return uint32(second)<<16 | uint32(first)
	}

	return uint32(first)<<16 | uint32(second)
}

func (r Register) ControlValue(now time.Time) DecodedValue {
	rendered := ""
	var value any
	options := writeOptions(r)
	if r.WriteOnly && len(options) == 1 {
		rendered = options[0].Label
		value = options[0].Label
	}

	return DecodedValue{
		ID:             r.ID,
		Name:           r.Name,
		Address:        r.Address,
		Group:          r.Group,
		Component:      r.Component,
		Entity:         r.Entity,
		EntityCategory: r.EntityCategory,
		Writable:       r.Writable,
		WriteOnly:      r.WriteOnly,
		Unit:           r.Unit,
		DeviceClass:    r.DeviceClass,
		StateClass:     r.StateClass,
		Icon:           r.Icon,
		Value:          value,
		Rendered:       rendered,
		WriteMin:       r.WriteMin,
		WriteMax:       r.WriteMax,
		WriteStep:      r.WriteStep,
		Options:        options,
		UpdatedAt:      now,
	}
}

func MergeWriteOnlyControls(values []DecodedValue, now time.Time) []DecodedValue {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value.ID] = struct{}{}
	}

	merged := append([]DecodedValue(nil), values...)
	for _, reg := range Catalog() {
		if !reg.Writable || !reg.WriteOnly {
			continue
		}
		if _, ok := seen[reg.ID]; ok {
			continue
		}
		merged = append(merged, reg.ControlValue(now))
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Group == merged[j].Group {
			return merged[i].Address < merged[j].Address
		}
		return merged[i].Group < merged[j].Group
	})

	return merged
}

func roundFloat(value float64, precision int) float64 {
	pow := math.Pow10(precision)
	return math.Round(value*pow) / pow
}

func scaleOrOne(scale float64) float64 {
	if scale == 0 {
		return 1
	}
	return scale
}

func enumValue(mapping map[int64]string, raw int64) string {
	if value, ok := mapping[raw]; ok {
		return value
	}
	return fmt.Sprintf("Unknown (%d)", raw)
}

func enumOptions(mapping map[int64]string) []Option {
	if len(mapping) == 0 {
		return nil
	}

	keys := make([]int64, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	options := make([]Option, 0, len(keys))
	for _, key := range keys {
		options = append(options, Option{
			Raw:   key,
			Label: mapping[key],
		})
	}

	return options
}

func writeOptions(r Register) []Option {
	if !r.Writable {
		return nil
	}
	return enumOptions(r.Enum)
}

func FindByID(id string) (Register, bool) {
	for _, reg := range Catalog() {
		if reg.ID == id {
			return reg, true
		}
	}
	return Register{}, false
}

func (r Register) EncodeWrite(input any) (uint16, error) {
	if !r.Writable {
		return 0, fmt.Errorf("register %s is not writable", r.ID)
	}
	if r.Count != 1 {
		return 0, fmt.Errorf("register %s write encoding supports single-register writes only", r.ID)
	}

	if len(r.Enum) > 0 {
		raw, err := enumRawValue(r.Enum, input)
		if err != nil {
			return 0, err
		}
		if raw < 0 || raw > math.MaxUint16 {
			return 0, fmt.Errorf("encoded value out of uint16 range")
		}
		return uint16(raw), nil
	}

	number, err := numericInput(input)
	if err != nil {
		return 0, err
	}
	if err := validateWriteBounds(r, number); err != nil {
		return 0, err
	}

	scale := scaleOrOne(r.Scale)
	raw := math.Round(number / scale)
	if raw < 0 || raw > math.MaxUint16 {
		return 0, fmt.Errorf("encoded value out of uint16 range")
	}

	return uint16(raw), nil
}

func enumRawValue(mapping map[int64]string, input any) (int64, error) {
	switch value := input.(type) {
	case string:
		for raw, label := range mapping {
			if strings.EqualFold(label, value) {
				return raw, nil
			}
		}
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			if _, ok := mapping[parsed]; ok {
				return parsed, nil
			}
		}
		return 0, fmt.Errorf("unsupported enum value %q", value)
	case float64:
		raw := int64(value)
		if _, ok := mapping[raw]; !ok {
			return 0, fmt.Errorf("unsupported enum value %.0f", value)
		}
		return raw, nil
	case int64:
		if _, ok := mapping[value]; !ok {
			return 0, fmt.Errorf("unsupported enum value %d", value)
		}
		return value, nil
	case int:
		raw := int64(value)
		if _, ok := mapping[raw]; !ok {
			return 0, fmt.Errorf("unsupported enum value %d", value)
		}
		return raw, nil
	default:
		return 0, fmt.Errorf("unsupported enum input type %T", input)
	}
}

func numericInput(input any) (float64, error) {
	switch value := input.(type) {
	case float64:
		return value, nil
	case int64:
		return float64(value), nil
	case int:
		return float64(value), nil
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid numeric value %q", value)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported numeric input type %T", input)
	}
}

func validateWriteBounds(r Register, value float64) error {
	if r.WriteMin != 0 || r.WriteMax != 0 {
		if value < r.WriteMin || value > r.WriteMax {
			return fmt.Errorf("%s must be between %.3f and %.3f", r.ID, r.WriteMin, r.WriteMax)
		}
	}

	if r.WriteStep > 0 {
		base := r.WriteMin
		steps := math.Round((value - base) / r.WriteStep)
		expected := base + steps*r.WriteStep
		if math.Abs(expected-value) > 0.000001 {
			return fmt.Errorf("%s must follow step %.3f", r.ID, r.WriteStep)
		}
	}

	return nil
}
