package registers

import (
	"testing"
	"time"
)

func TestBuildReadPlanMergesContiguousRanges(t *testing.T) {
	t.Parallel()

	ranges := BuildReadPlan(GroupFast)
	if len(ranges) == 0 {
		t.Fatal("expected fast read ranges")
	}

	for _, rng := range ranges {
		if rng.Count == 0 {
			t.Fatal("expected non-empty range")
		}
	}
}

func TestBuildCriticalFastReadPlanUsesEssentialRanges(t *testing.T) {
	t.Parallel()

	ranges := BuildCriticalFastReadPlan()
	if len(ranges) != 3 {
		t.Fatalf("unexpected critical fast range count: got %d want 3", len(ranges))
	}

	expected := []struct {
		start uint16
		count uint16
	}{
		{start: 0x0100, count: 3},
		{start: 0x0107, count: 3},
		{start: 0x021B, count: 1},
	}

	for i, want := range expected {
		if ranges[i].Start != want.start || ranges[i].Count != want.count {
			t.Fatalf("range %d = {start: 0x%04X, count: %d}, want {start: 0x%04X, count: %d}",
				i, ranges[i].Start, ranges[i].Count, want.start, want.count)
		}
	}
}

func TestDecodeScaledUint16(t *testing.T) {
	t.Parallel()

	reg := Register{
		ID:        "battery_voltage",
		Name:      "Battery Voltage",
		Address:   0x0101,
		Count:     1,
		Type:      TypeUint16,
		Scale:     0.1,
		Precision: 1,
		Group:     GroupFast,
	}

	value, err := reg.Decode([]uint16{523}, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got, ok := value.Value.(float64); !ok || got != 52.3 {
		t.Fatalf("unexpected decoded value: %#v", value.Value)
	}
}

func TestDecodeEnum(t *testing.T) {
	t.Parallel()

	reg := Register{
		ID:      "machine_state",
		Name:    "Machine State",
		Address: 0x0210,
		Count:   1,
		Type:    TypeUint16,
		Group:   GroupSlow,
		Enum: map[int64]string{
			4: "AC Power Operation",
		},
	}

	value, err := reg.Decode([]uint16{4}, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got, ok := value.Value.(string); !ok || got != "AC Power Operation" {
		t.Fatalf("unexpected enum value: %#v", value.Value)
	}
}

func TestEncodeWriteNumeric(t *testing.T) {
	t.Parallel()

	reg := Register{
		ID:        "pv_charge_current_setup",
		Count:     1,
		Type:      TypeUint16,
		Scale:     0.1,
		Writable:  true,
		WriteMin:  0,
		WriteMax:  100,
		WriteStep: 0.1,
	}

	raw, err := reg.EncodeWrite("60.0")
	if err != nil {
		t.Fatalf("EncodeWrite() error = %v", err)
	}

	if raw != 600 {
		t.Fatalf("unexpected raw value: got %d want 600", raw)
	}
}

func TestCatalogDisablesBatteryTypeWrite(t *testing.T) {
	t.Parallel()

	reg, ok := FindByID("battery_type")
	if !ok {
		t.Fatal("battery_type not found")
	}
	if reg.Writable {
		t.Fatal("battery_type should not be writable")
	}
}

func TestCatalogIncludesMainsChargeCurrentLimit(t *testing.T) {
	t.Parallel()

	reg, ok := FindByID("mains_charge_current_limit")
	if !ok {
		t.Fatal("mains_charge_current_limit not found")
	}
	if !reg.Writable {
		t.Fatal("mains_charge_current_limit should be writable")
	}
	if reg.Address != 0xE205 {
		t.Fatalf("unexpected address: got 0x%04X want 0xE205", reg.Address)
	}
}

func TestEncodeWriteEnum(t *testing.T) {
	t.Parallel()

	reg := Register{
		ID:       "output_source_priority",
		Count:    1,
		Type:     TypeUint16,
		Writable: true,
		Enum: map[int64]string{
			0: "Solar",
			1: "Utility",
		},
	}

	raw, err := reg.EncodeWrite("Utility")
	if err != nil {
		t.Fatalf("EncodeWrite() error = %v", err)
	}

	if raw != 1 {
		t.Fatalf("unexpected raw value: got %d want 1", raw)
	}
}
