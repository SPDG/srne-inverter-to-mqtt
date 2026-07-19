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
	if len(ranges) != 4 {
		t.Fatalf("unexpected critical fast range count: got %d want 4", len(ranges))
	}

	expected := []struct {
		start uint16
		count uint16
	}{
		{start: 0x0100, count: 3},
		{start: 0x0107, count: 3},
		{start: 0x021B, count: 1},
		{start: 0xE20A, count: 1},
	}

	for i, want := range expected {
		if ranges[i].Start != want.start || ranges[i].Count != want.count {
			t.Fatalf("range %d = {start: 0x%04X, count: %d}, want {start: 0x%04X, count: %d}",
				i, ranges[i].Start, ranges[i].Count, want.start, want.count)
		}
	}
}

func TestThreePhaseCatalogIncludesPhaseAndPVStringSensors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id        string
		address   uint16
		synthetic bool
	}{
		{id: "pv_power", address: 0xFFFA, synthetic: true},
		{id: "pv1_voltage", address: 0x0107},
		{id: "pv1_current", address: 0x0108},
		{id: "pv1_power", address: 0x0109},
		{id: "pv2_voltage", address: 0x010F},
		{id: "pv2_current", address: 0x0110},
		{id: "pv2_power", address: 0x0111},
		{id: "load_power", address: 0xFFF8, synthetic: true},
		{id: "load_power_phase_a", address: 0x021B},
		{id: "load_power_phase_b", address: 0x0232},
		{id: "load_power_phase_c", address: 0x0233},
		{id: "grid_power", address: 0xFFF9, synthetic: true},
		{id: "grid_power_phase_a", address: 0x023A},
		{id: "grid_power_phase_b", address: 0x023B},
		{id: "grid_power_phase_c", address: 0x023C},
	}

	for _, tc := range cases {
		reg, ok := FindByIDForInverterType(tc.id, InverterTypeThreePhase)
		if !ok {
			t.Fatalf("%s not found", tc.id)
		}
		if reg.Address != tc.address {
			t.Fatalf("%s address = 0x%04X, want 0x%04X", tc.id, reg.Address, tc.address)
		}
		if reg.Synthetic != tc.synthetic {
			t.Fatalf("%s synthetic = %t, want %t", tc.id, reg.Synthetic, tc.synthetic)
		}
	}
}

func TestThreePhaseCatalogReplacesSinglePhasePowerRegisters(t *testing.T) {
	t.Parallel()

	catalog := CatalogForInverterType(InverterTypeThreePhase)
	for _, reg := range catalog {
		if reg.ID == "load_power" && !reg.Synthetic {
			t.Fatalf("three-phase load_power should be synthetic, got physical address 0x%04X", reg.Address)
		}
		if reg.ID == "grid_power" && !reg.Synthetic {
			t.Fatalf("three-phase grid_power should be synthetic, got physical address 0x%04X", reg.Address)
		}
		if reg.ID == "pv_power" && !reg.Synthetic {
			t.Fatalf("three-phase pv_power should be synthetic, got physical address 0x%04X", reg.Address)
		}
	}
}

func TestGridOperatingModeIsWritableOnlyForThreePhaseProfile(t *testing.T) {
	t.Parallel()

	if _, ok := FindByIDForInverterType("grid_operating_mode", InverterTypeSinglePhase); ok {
		t.Fatal("grid_operating_mode should not be exposed for the single-phase profile")
	}

	reg, ok := FindByIDForInverterType("grid_operating_mode", InverterTypeThreePhase)
	if !ok {
		t.Fatal("grid_operating_mode not found for the three-phase profile")
	}
	if !reg.Writable {
		t.Fatal("grid_operating_mode should be writable")
	}
	if reg.Address != 0xE037 {
		t.Fatalf("grid_operating_mode address = 0x%04X, want 0xE037", reg.Address)
	}

	expected := map[int64]string{
		0: "Disabled (DIS)",
		1: "On-grid Export (ON GRD)",
		2: "Zero Export (AC Output)",
		3: "Zero Export (AC Input)",
	}
	for raw, label := range expected {
		if got := reg.Enum[raw]; got != label {
			t.Fatalf("grid_operating_mode enum[%d] = %q, want %q", raw, got, label)
		}
	}
}

func TestThreePhaseCatalogIncludesGridPowerControls(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id        string
		address   uint16
		writeMax  float64
		writeStep float64
	}{
		{id: "on_grid_max_power", address: 0xE400, writeMax: 12000, writeStep: 100},
		{id: "zero_export_power", address: 0xE42C, writeMax: 500, writeStep: 1},
	}

	for _, tc := range cases {
		if _, ok := FindByIDForInverterType(tc.id, InverterTypeSinglePhase); ok {
			t.Fatalf("%s should not be exposed for the single-phase profile", tc.id)
		}

		reg, ok := FindByIDForInverterType(tc.id, InverterTypeThreePhase)
		if !ok {
			t.Fatalf("%s not found for the three-phase profile", tc.id)
		}
		if !reg.Writable {
			t.Fatalf("%s should be writable", tc.id)
		}
		if reg.Address != tc.address {
			t.Fatalf("%s address = 0x%04X, want 0x%04X", tc.id, reg.Address, tc.address)
		}
		if reg.WriteMin != 0 || reg.WriteMax != tc.writeMax || reg.WriteStep != tc.writeStep {
			t.Fatalf("%s write bounds = [%v,%v] step %v, want [0,%v] step %v",
				tc.id, reg.WriteMin, reg.WriteMax, reg.WriteStep, tc.writeMax, tc.writeStep)
		}
	}
}

func TestThreePhaseCatalogUsesSPI12KChargeCurrentBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id             string
		singlePhaseMax float64
		threePhaseMax  float64
	}{
		{id: "pv_charge_current_setup", singlePhaseMax: 100, threePhaseMax: 260},
		{id: "maximum_charge_current", singlePhaseMax: 150, threePhaseMax: 260},
		{id: "mains_charge_current_limit", singlePhaseMax: 100, threePhaseMax: 120},
	}

	for _, tc := range cases {
		singlePhase, ok := FindByIDForInverterType(tc.id, InverterTypeSinglePhase)
		if !ok {
			t.Fatalf("%s not found for single-phase profile", tc.id)
		}
		threePhase, ok := FindByIDForInverterType(tc.id, InverterTypeThreePhase)
		if !ok {
			t.Fatalf("%s not found for three-phase profile", tc.id)
		}
		if singlePhase.WriteMax != tc.singlePhaseMax {
			t.Fatalf("%s single-phase maximum = %v, want %v", tc.id, singlePhase.WriteMax, tc.singlePhaseMax)
		}
		if threePhase.WriteMax != tc.threePhaseMax {
			t.Fatalf("%s three-phase maximum = %v, want %v", tc.id, threePhase.WriteMax, tc.threePhaseMax)
		}
	}
}

func TestThreePhaseCriticalFastReadPlanIncludesPhaseAndPVStringRanges(t *testing.T) {
	t.Parallel()

	ranges := BuildCriticalFastReadPlanForInverterType(InverterTypeThreePhase)
	expected := []struct {
		start uint16
		count uint16
	}{
		{start: 0x0100, count: 3},
		{start: 0x0107, count: 3},
		{start: 0x010F, count: 3},
		{start: 0x0219, count: 1},
		{start: 0x021B, count: 1},
		{start: 0x0230, count: 4},
		{start: 0x023A, count: 3},
		{start: 0xE20A, count: 1},
	}

	if len(ranges) != len(expected) {
		t.Fatalf("unexpected critical fast range count: got %d want %d (%#v)", len(ranges), len(expected), ranges)
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

func TestCatalogIncludesMaximumChargeCurrent(t *testing.T) {
	t.Parallel()

	reg, ok := FindByID("maximum_charge_current")
	if !ok {
		t.Fatal("maximum_charge_current not found")
	}
	if !reg.Writable {
		t.Fatal("maximum_charge_current should be writable")
	}
	if reg.Address != 0xE20A {
		t.Fatalf("unexpected address: got 0x%04X want 0xE20A", reg.Address)
	}
	if reg.Scale != 0.1 || reg.WriteMin != 0 || reg.WriteMax != 150 || reg.WriteStep != 0.1 {
		t.Fatalf("unexpected write metadata: scale=%v bounds=[%v,%v] step=%v",
			reg.Scale, reg.WriteMin, reg.WriteMax, reg.WriteStep)
	}
	if reg.Group != GroupFast {
		t.Fatalf("maximum_charge_current group = %s, want %s", reg.Group, GroupFast)
	}
}

func TestCatalogIncludesBMSChargeLimitMode(t *testing.T) {
	t.Parallel()

	reg, ok := FindByID("bms_charge_limit_mode")
	if !ok {
		t.Fatal("bms_charge_limit_mode not found")
	}
	if !reg.Writable {
		t.Fatal("bms_charge_limit_mode should be writable")
	}
	if reg.Address != 0xE025 {
		t.Fatalf("unexpected address: got 0x%04X want 0xE025", reg.Address)
	}
	if got := reg.Enum[1]; got != "BMS" {
		t.Fatalf("unexpected enum value for raw 1: got %q want BMS", got)
	}
}

func TestCatalogIncludesBMSCommunicationDiagnostics(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id      string
		address uint16
	}{
		{id: "bms_communication_enable", address: 0xE215},
		{id: "bms_protocol", address: 0xE21B},
	}

	for _, tc := range cases {
		reg, ok := FindByID(tc.id)
		if !ok {
			t.Fatalf("%s not found", tc.id)
		}
		if reg.Address != tc.address {
			t.Fatalf("%s address = 0x%04X, want 0x%04X", tc.id, reg.Address, tc.address)
		}
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
		{id: "battery_discharge_cutoff_soc", address: 0xE00F, count: 1},
		{id: "charge_termination_current", address: 0xE01C, count: 1},
		{id: "battery_charge_cutoff_soc", address: 0xE01D, count: 1},
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

	for _, id := range []string{
		"battery_discharge_cutoff_soc",
		"battery_charge_cutoff_soc",
		"battery_low_soc_alarm",
		"battery_discharge_stop",
		"battery_discharge_start",
	} {
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

func TestCatalogIncludesChargeTerminationCurrent(t *testing.T) {
	t.Parallel()

	reg, ok := FindByID("charge_termination_current")
	if !ok {
		t.Fatal("charge_termination_current not found")
	}
	if !reg.Writable {
		t.Fatal("charge_termination_current should be writable")
	}
	if reg.Address != 0xE01C || reg.Scale != 0.1 {
		t.Fatalf("charge_termination_current address/scale = 0x%04X/%v, want 0xE01C/0.1", reg.Address, reg.Scale)
	}
	if reg.WriteMin != 0 || reg.WriteMax != 10 || reg.WriteStep != 0.1 {
		t.Fatalf("charge_termination_current write bounds = [%v,%v] step %v, want [0,10] step 0.1",
			reg.WriteMin, reg.WriteMax, reg.WriteStep)
	}
}

func TestCatalogIncludesWritableOperationalSwitches(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id      string
		address uint16
	}{
		{id: "power_saving_mode", address: 0xE20C},
		{id: "overload_auto_restart", address: 0xE20D},
		{id: "overtemperature_auto_restart", address: 0xE20E},
		{id: "buzzer_alarm", address: 0xE210},
		{id: "source_change_alert", address: 0xE211},
		{id: "overload_bypass", address: 0xE212},
	}

	for _, tc := range cases {
		reg, ok := FindByID(tc.id)
		if !ok {
			t.Fatalf("%s not found", tc.id)
		}
		if !reg.Writable {
			t.Fatalf("%s should be writable", tc.id)
		}
		if reg.Address != tc.address {
			t.Fatalf("%s address = 0x%04X, want 0x%04X", tc.id, reg.Address, tc.address)
		}
		if reg.WriteMin != 0 || reg.WriteMax != 1 || reg.WriteStep != 1 {
			t.Fatalf("%s write bounds = [%v,%v] step %v, want [0,1] step 1",
				tc.id, reg.WriteMin, reg.WriteMax, reg.WriteStep)
		}
		if reg.Enum[0] != "Disabled" || reg.Enum[1] != "Enabled" {
			t.Fatalf("%s should expose Disabled/Enabled options", tc.id)
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

func TestCatalogIncludesLastSourceSwitchSyntheticSensors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id          string
		address     uint16
		deviceClass string
	}{
		{id: "last_switch_to_grid_at", address: 0xFFF2, deviceClass: "timestamp"},
		{id: "last_switch_to_grid_soc", address: 0xFFF3, deviceClass: "battery"},
		{id: "last_switch_to_battery_at", address: 0xFFF4, deviceClass: "timestamp"},
		{id: "last_switch_to_battery_soc", address: 0xFFF5, deviceClass: "battery"},
	}

	for _, tc := range cases {
		reg, ok := FindByID(tc.id)
		if !ok {
			t.Fatalf("%s not found", tc.id)
		}
		if !reg.Synthetic {
			t.Fatalf("%s should be synthetic", tc.id)
		}
		if reg.Address != tc.address {
			t.Fatalf("%s address = 0x%04X, want 0x%04X", tc.id, reg.Address, tc.address)
		}
		if reg.DeviceClass != tc.deviceClass {
			t.Fatalf("%s device class = %q, want %q", tc.id, reg.DeviceClass, tc.deviceClass)
		}
	}
}

func TestCatalogIncludesBatteryEnergyEstimateSyntheticSensors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id      string
		address uint16
	}{
		{id: "battery_charge_energy_total_estimate", address: 0xFFF6},
		{id: "battery_discharge_energy_total_estimate", address: 0xFFF7},
	}

	for _, tc := range cases {
		reg, ok := FindByID(tc.id)
		if !ok {
			t.Fatalf("%s not found", tc.id)
		}
		if !reg.Synthetic {
			t.Fatalf("%s should be synthetic", tc.id)
		}
		if reg.Address != tc.address {
			t.Fatalf("%s address = 0x%04X, want 0x%04X", tc.id, reg.Address, tc.address)
		}
		if reg.DeviceClass != "energy" || reg.StateClass != "total_increasing" || reg.Unit != "kWh" {
			t.Fatalf("%s metadata = {deviceClass:%q stateClass:%q unit:%q}, want energy total_increasing kWh",
				tc.id, reg.DeviceClass, reg.StateClass, reg.Unit)
		}
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
