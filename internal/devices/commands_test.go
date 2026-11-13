package devices

import (
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

func f(v float64) *float64 { return &v }

func TestValidateValue(t *testing.T) {
	power := domain.Capability{Name: "power", Type: "switch"}
	for _, v := range []any{true, false, "on", "OFF", float64(1), 0} {
		if _, err := validateValue(power, v); err != nil {
			t.Fatalf("switch %v rejected: %v", v, err)
		}
	}
	if _, err := validateValue(power, "maybe"); err == nil {
		t.Fatal("invalid switch accepted")
	}

	brightness := domain.Capability{Name: "brightness", Type: "number", Min: f(0), Max: f(100)}
	if got, err := validateValue(brightness, "42"); err != nil || got.(float64) != 42 {
		t.Fatalf("numeric string: %v %v", got, err)
	}
	if _, err := validateValue(brightness, 101); err == nil {
		t.Fatal("above max accepted")
	}
	if _, err := validateValue(brightness, -1); err == nil {
		t.Fatal("below min accepted")
	}
	if _, err := validateValue(brightness, "abc"); err == nil {
		t.Fatal("non-numeric accepted")
	}

	mode := domain.Capability{Name: "mode", Type: "enum", Enum: []string{"cool", "heat"}}
	if got, err := validateValue(mode, "HEAT"); err != nil || got != "heat" {
		t.Fatalf("enum: %v %v", got, err)
	}
	if _, err := validateValue(mode, "dry"); err == nil {
		t.Fatal("enum outside set accepted")
	}
	if _, err := validateValue(mode, 3); err == nil {
		t.Fatal("non-string enum accepted")
	}

	label := domain.Capability{Name: "label", Type: "string"}
	if _, err := validateValue(label, nil); err == nil {
		t.Fatal("nil accepted for required value")
	}
	if got, err := validateValue(label, "hello"); err != nil || got != "hello" {
		t.Fatalf("string: %v %v", got, err)
	}
}
