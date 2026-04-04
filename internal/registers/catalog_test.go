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

func TestDecodeSignedGridPowerUsesPositiveImport(t *testing.T) {
	t.Parallel()

	reg := Register{
		ID:        "grid_power",
		Name:      "Grid Power",
		Address:   0x023A,
		Count:     1,
		Type:      TypeInt16,
		Scale:     -1,
		Precision: 0,
		Group:     GroupSlow,
	}

	value, err := reg.Decode([]uint16{0xFE0C}, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got, ok := value.Value.(int64); !ok || got != 500 {
		t.Fatalf("unexpected decoded value: %#v", value.Value)
	}
}

func TestDecodeScaledUint32LowWordFirst(t *testing.T) {
	t.Parallel()

	reg := Register{
		ID:        "total_load_consumption",
		Name:      "Total Load Consumption",
		Address:   0xF03A,
		Count:     2,
		Type:      TypeUint32,
		WordOrder: WordOrderLowHigh,
		Scale:     0.1,
		Precision: 1,
		Group:     GroupSlow,
	}

	value, err := reg.Decode([]uint16{0x7642, 0x0000}, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got, ok := value.Value.(float64); !ok || got != 3027.4 {
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

func TestCatalogIncludesWriteOnlyResetMachine(t *testing.T) {
	t.Parallel()

	reg, ok := FindByID("reset_machine")
	if !ok {
		t.Fatal("reset_machine not found")
	}
	if !reg.Writable || !reg.WriteOnly {
		t.Fatal("reset_machine should be writable and write-only")
	}
	if reg.Address != 0xDF01 {
		t.Fatalf("unexpected address: got 0x%04X want 0xDF01", reg.Address)
	}
}

func TestMergeWriteOnlyControlsIncludesResetMachine(t *testing.T) {
	t.Parallel()

	values := MergeWriteOnlyControls(nil, time.Unix(0, 0))
	for _, value := range values {
		if value.ID == "reset_machine" {
			if !value.Writable || !value.WriteOnly {
				t.Fatal("reset_machine control should be writable and write-only")
			}
			if value.Rendered != "Reset" {
				t.Fatalf("unexpected rendered value: got %q want %q", value.Rendered, "Reset")
			}
			return
		}
	}

	t.Fatal("reset_machine control not found")
}

func TestCatalogIncludesOnePhaseLiveAndHistoryRegisters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id      string
		address uint16
		count   uint16
	}{
		{id: "battery_temperature", address: 0x0103, count: 1},
		{id: "pv_total_power", address: 0x010A, count: 1},
		{id: "load_current", address: 0x0219, count: 1},
		{id: "grid_power", address: 0x023A, count: 1},
		{id: "today_production", address: 0xF02F, count: 1},
		{id: "total_production", address: 0xF038, count: 2},
		{id: "total_energy_import", address: 0xF048, count: 2},
		{id: "battery_discharge_stop", address: 0xE01F, count: 1},
		{id: "battery_discharge_start", address: 0xE020, count: 1},
	}

	for _, tc := range cases {
		reg, ok := FindByID(tc.id)
		if !ok {
			t.Fatalf("%s not found", tc.id)
		}
		if reg.Address != tc.address || reg.Count != tc.count {
			t.Fatalf("%s = {address: 0x%04X, count: %d}, want {address: 0x%04X, count: %d}",
				tc.id, reg.Address, reg.Count, tc.address, tc.count)
		}
	}
}

func TestCatalogIncludesWritableBatteryDischargeThresholds(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"battery_discharge_stop", "battery_discharge_start"} {
		reg, ok := FindByID(id)
		if !ok {
			t.Fatalf("%s not found", id)
		}
		if !reg.Writable {
			t.Fatalf("%s should be writable", id)
		}
		if reg.WriteMin != 0 || reg.WriteMax != 100 || reg.WriteStep != 1 {
			t.Fatalf("%s write bounds = [%v,%v] step %v, want [0,100] step 1", id, reg.WriteMin, reg.WriteMax, reg.WriteStep)
		}
	}
}

func TestCatalogHistoryTotalsUseLowWordFirst(t *testing.T) {
	t.Parallel()

	ids := []string{
		"total_energy_export",
		"total_battery_charge_ah",
		"total_battery_discharge_ah",
		"total_production",
		"total_load_consumption",
		"total_battery_grid_charge_ah",
		"total_energy_import",
	}

	for _, id := range ids {
		reg, ok := FindByID(id)
		if !ok {
			t.Fatalf("%s not found", id)
		}
		if reg.WordOrder != WordOrderLowHigh {
			t.Fatalf("%s word order = %q, want %q", id, reg.WordOrder, WordOrderLowHigh)
		}
	}
}

func TestCatalogGridPowerUsesSignedEncoding(t *testing.T) {
	t.Parallel()

	reg, ok := FindByID("grid_power")
	if !ok {
		t.Fatal("grid_power not found")
	}
	if reg.Type != TypeInt16 {
		t.Fatalf("grid_power type = %q, want %q", reg.Type, TypeInt16)
	}
	if reg.Scale != -1 {
		t.Fatalf("grid_power scale = %v, want -1", reg.Scale)
	}
}

func TestCatalogResetMachineUsesRestartButtonClass(t *testing.T) {
	t.Parallel()

	reg, ok := FindByID("reset_machine")
	if !ok {
		t.Fatal("reset_machine not found")
	}
	if reg.ButtonClass != "restart" {
		t.Fatalf("reset_machine button class = %q, want %q", reg.ButtonClass, "restart")
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
