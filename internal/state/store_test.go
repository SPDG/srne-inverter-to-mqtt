package state

import (
	"testing"
	"time"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/registers"
)

func TestSnapshotIncludesDerivedEnergyMetrics(t *testing.T) {
	t.Parallel()

	store := New()
	now := time.Unix(1711929600, 0).UTC()
	store.UpsertTelemetry([]registers.DecodedValue{
		{
			ID:        "total_energy_import",
			Address:   0xF048,
			Group:     registers.GroupSlow,
			Component: "sensor",
			Entity:    "diagnostic",
			Unit:      "kWh",
			Value:     3653.6,
			Rendered:  "3653.6",
			UpdatedAt: now,
		},
		{
			ID:        "total_load_consumption",
			Address:   0xF03A,
			Group:     registers.GroupSlow,
			Component: "sensor",
			Entity:    "diagnostic",
			Unit:      "kWh",
			Value:     3027.4,
			Rendered:  "3027.4",
			UpdatedAt: now,
		},
	})

	snapshot := store.Snapshot()

	values := make(map[string]registers.DecodedValue, len(snapshot.Telemetry))
	for _, value := range snapshot.Telemetry {
		values[value.ID] = value
	}

	losses, ok := values["system_energy_losses_total"]
	if !ok {
		t.Fatal("system_energy_losses_total not found")
	}
	if got, ok := losses.Value.(float64); !ok || got != 626.2 {
		t.Fatalf("unexpected losses value: %#v", losses.Value)
	}
	if losses.Rendered != "626.2" {
		t.Fatalf("unexpected losses rendered value: %q", losses.Rendered)
	}

	efficiency, ok := values["system_energy_efficiency_total"]
	if !ok {
		t.Fatal("system_energy_efficiency_total not found")
	}
	if got, ok := efficiency.Value.(float64); !ok || got != 82.9 {
		t.Fatalf("unexpected efficiency value: %#v", efficiency.Value)
	}
	if efficiency.Rendered != "82.9" {
		t.Fatalf("unexpected efficiency rendered value: %q", efficiency.Rendered)
	}
}

func TestSnapshotIncludesBatteryEnergyEstimates(t *testing.T) {
	t.Parallel()

	store := New()
	now := time.Unix(1711929600, 0).UTC()
	store.UpsertTelemetry([]registers.DecodedValue{
		{
			ID:        "battery_type",
			Address:   0xE004,
			Group:     registers.GroupSlow,
			Component: "sensor",
			Entity:    "config",
			Value:     "LFP x16",
			Rendered:  "LFP x16",
			UpdatedAt: now,
		},
		{
			ID:        "total_battery_charge_ah",
			Address:   0xF034,
			Group:     registers.GroupSlow,
			Component: "sensor",
			Entity:    "diagnostic",
			Unit:      "Ah",
			Value:     8758.0,
			Rendered:  "8758",
			UpdatedAt: now,
		},
		{
			ID:        "total_battery_discharge_ah",
			Address:   0xF036,
			Group:     registers.GroupSlow,
			Component: "sensor",
			Entity:    "diagnostic",
			Unit:      "Ah",
			Value:     3585.0,
			Rendered:  "3585",
			UpdatedAt: now,
		},
	})

	snapshot := store.Snapshot()
	values := make(map[string]registers.DecodedValue, len(snapshot.Telemetry))
	for _, value := range snapshot.Telemetry {
		values[value.ID] = value
	}

	charged, ok := values["battery_charge_energy_total_estimate"]
	if !ok {
		t.Fatal("battery_charge_energy_total_estimate not found")
	}
	if got, ok := charged.Value.(float64); !ok || got != 448.4 {
		t.Fatalf("unexpected charged value: %#v", charged.Value)
	}
	if charged.DeviceClass != "energy" || charged.StateClass != "total_increasing" || charged.Unit != "kWh" {
		t.Fatalf("unexpected charged metadata: %#v", charged)
	}

	discharged, ok := values["battery_discharge_energy_total_estimate"]
	if !ok {
		t.Fatal("battery_discharge_energy_total_estimate not found")
	}
	if got, ok := discharged.Value.(float64); !ok || got != 183.6 {
		t.Fatalf("unexpected discharged value: %#v", discharged.Value)
	}
	if discharged.DeviceClass != "energy" || discharged.StateClass != "total_increasing" || discharged.Unit != "kWh" {
		t.Fatalf("unexpected discharged metadata: %#v", discharged)
	}
}

func TestSnapshotSkipsBatteryEnergyEstimatesWithoutNominalVoltage(t *testing.T) {
	t.Parallel()

	store := New()
	store.UpsertTelemetry([]registers.DecodedValue{
		{
			ID:        "battery_type",
			Value:     "GEL",
			Rendered:  "GEL",
			UpdatedAt: time.Unix(1711929600, 0).UTC(),
		},
		{
			ID:        "total_battery_charge_ah",
			Value:     1000.0,
			Rendered:  "1000",
			UpdatedAt: time.Unix(1711929600, 0).UTC(),
		},
	})

	snapshot := store.Snapshot()
	for _, value := range snapshot.Telemetry {
		if value.ID == "battery_charge_energy_total_estimate" || value.ID == "battery_discharge_energy_total_estimate" {
			t.Fatalf("unexpected battery energy estimate %s", value.ID)
		}
	}
}

func TestSnapshotSkipsDerivedMetricsWithoutTotals(t *testing.T) {
	t.Parallel()

	store := New()
	store.UpsertTelemetry([]registers.DecodedValue{
		{
			ID:        "today_energy_import",
			Address:   0xF03D,
			Group:     registers.GroupSlow,
			Component: "sensor",
			Entity:    "diagnostic",
			Unit:      "kWh",
			Value:     0.9,
			Rendered:  "0.9",
			UpdatedAt: time.Unix(1711929600, 0).UTC(),
		},
	})

	snapshot := store.Snapshot()
	for _, value := range snapshot.Telemetry {
		if value.ID == "system_energy_losses_total" || value.ID == "system_energy_efficiency_total" {
			t.Fatalf("unexpected derived metric %s", value.ID)
		}
	}
}

func TestSnapshotIncludesLastSourceSwitchEvents(t *testing.T) {
	t.Parallel()

	store := New()
	batteryAt := time.Unix(1711929000, 0).UTC()
	gridAt := batteryAt.Add(15 * time.Minute)

	store.UpsertTelemetry([]registers.DecodedValue{
		{
			ID:        "battery_soc",
			Address:   0x0100,
			Group:     registers.GroupFast,
			Component: "sensor",
			Entity:    "diagnostic",
			Unit:      "%",
			Value:     36.0,
			Rendered:  "36",
			UpdatedAt: batteryAt.Add(-5 * time.Minute),
		},
		{
			ID:        "machine_state",
			Address:   0x0210,
			Group:     registers.GroupSlow,
			Component: "sensor",
			Entity:    "diagnostic",
			Value:     "AC Power Operation",
			Rendered:  "AC Power Operation",
			UpdatedAt: batteryAt.Add(-5 * time.Minute),
		},
	})

	store.UpsertTelemetry([]registers.DecodedValue{
		{
			ID:        "battery_soc",
			Address:   0x0100,
			Group:     registers.GroupFast,
			Component: "sensor",
			Entity:    "diagnostic",
			Unit:      "%",
			Value:     35.0,
			Rendered:  "35",
			UpdatedAt: batteryAt,
		},
		{
			ID:        "machine_state",
			Address:   0x0210,
			Group:     registers.GroupSlow,
			Component: "sensor",
			Entity:    "diagnostic",
			Value:     "Inverter Operation",
			Rendered:  "Inverter Operation",
			UpdatedAt: batteryAt,
		},
	})

	store.UpsertTelemetry([]registers.DecodedValue{
		{
			ID:        "battery_soc",
			Address:   0x0100,
			Group:     registers.GroupFast,
			Component: "sensor",
			Entity:    "diagnostic",
			Unit:      "%",
			Value:     25.0,
			Rendered:  "25",
			UpdatedAt: gridAt,
		},
		{
			ID:        "machine_state",
			Address:   0x0210,
			Group:     registers.GroupSlow,
			Component: "sensor",
			Entity:    "diagnostic",
			Value:     "AC Power Operation",
			Rendered:  "AC Power Operation",
			UpdatedAt: gridAt,
		},
	})

	snapshot := store.Snapshot()
	values := make(map[string]registers.DecodedValue, len(snapshot.Telemetry))
	for _, value := range snapshot.Telemetry {
		values[value.ID] = value
	}

	lastGridAt, ok := values["last_switch_to_grid_at"]
	if !ok {
		t.Fatal("last_switch_to_grid_at not found")
	}
	if got, ok := lastGridAt.Value.(string); !ok || got != gridAt.Format(time.RFC3339) {
		t.Fatalf("unexpected last_switch_to_grid_at value: %#v", lastGridAt.Value)
	}

	lastGridSOC, ok := values["last_switch_to_grid_soc"]
	if !ok {
		t.Fatal("last_switch_to_grid_soc not found")
	}
	if got, ok := lastGridSOC.Value.(float64); !ok || got != 25 {
		t.Fatalf("unexpected last_switch_to_grid_soc value: %#v", lastGridSOC.Value)
	}

	lastBatteryAt, ok := values["last_switch_to_battery_at"]
	if !ok {
		t.Fatal("last_switch_to_battery_at not found")
	}
	if got, ok := lastBatteryAt.Value.(string); !ok || got != batteryAt.Format(time.RFC3339) {
		t.Fatalf("unexpected last_switch_to_battery_at value: %#v", lastBatteryAt.Value)
	}

	lastBatterySOC, ok := values["last_switch_to_battery_soc"]
	if !ok {
		t.Fatal("last_switch_to_battery_soc not found")
	}
	if got, ok := lastBatterySOC.Value.(float64); !ok || got != 35 {
		t.Fatalf("unexpected last_switch_to_battery_soc value: %#v", lastBatterySOC.Value)
	}
}
