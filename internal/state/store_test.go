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
