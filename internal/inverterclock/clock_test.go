package inverterclock

import (
	"reflect"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	value := DateTime{Year: 2026, Month: 7, Day: 31, Hour: 18, Minute: 42, Second: 7}
	words, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if want := []uint16{0x1A07, 0x1F12, 0x2A07}; !reflect.DeepEqual(words, want) {
		t.Fatalf("Encode() = %#v, want %#v", words, want)
	}
	decoded, err := Decode(words)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !decoded.Valid || decoded.Formatted != "2026-07-31 18:42:07" {
		t.Fatalf("Decode() = %#v", decoded)
	}
}

func TestDecodeFactoryTMYearAndInvalidMonth(t *testing.T) {
	decoded, err := Decode([]uint16{0x7A0D, 0x0E16, 0x1934})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded.Year != 2022 || decoded.Valid {
		t.Fatalf("Decode() = %#v, want invalid 2022 clock", decoded)
	}
}

func TestParseLocalRejectsInvalidDate(t *testing.T) {
	if _, err := ParseLocal("2026-02-31T12:00"); err == nil {
		t.Fatal("ParseLocal() expected invalid date error")
	}
}
