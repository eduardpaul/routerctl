package units

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
)

type mockUnits string

const (
	mockUnitsBase     mockUnits = "base"
	mockUnitsMegabase mockUnits = "megabase"
	mockUnitsTerabase mockUnits = "terabase"
)

var mockConverter = &UnitConverterBase[mockUnits]{
	UnitClass: "mock_units",
	UnitRatio: map[mockUnits]float64{mockUnitsBase: 1, mockUnitsMegabase: 1e6, mockUnitsTerabase: 1e12},
}

func approx(a, b float64) bool { return math.Abs(a-b) <= 1e-6*math.Max(1, math.Abs(b)) }

func isUnknownUnit(err error) bool {
	return errors.Is(err, arerror.ErrAsusRouter) && strings.Contains(err.Error(), "Unknown unit")
}

func TestConvert(t *testing.T) {
	if got, _ := mockConverter.Convert(2.0, mockUnitsBase, mockUnitsMegabase); !approx(got, 2e-6) {
		t.Fatal(got)
	}
	// Identity when units are equal
	if got, _ := mockConverter.Convert(42.0, mockUnitsBase, mockUnitsBase); got != 42.0 {
		t.Fatal(got)
	}
}

func TestConvertToBase(t *testing.T) {
	cases := []struct {
		value    float64
		unit     mockUnits
		expected float64
	}{
		{1.0, mockUnitsBase, 1.0}, {1.0, mockUnitsMegabase, 1e6}, {2.5, mockUnitsTerabase, 2.5e12},
	}
	for _, c := range cases {
		if got, err := mockConverter.ConvertToBase(c.value, c.unit); err != nil || !approx(got, c.expected) {
			t.Errorf("ConvertToBase(%v, %s) = %v, %v", c.value, c.unit, got, err)
		}
	}
	if _, err := mockConverter.ConvertToBase(1.0, "not_a_unit"); !isUnknownUnit(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestConverterFactory(t *testing.T) {
	fn, err := mockConverter.ConverterFactory(mockUnitsBase, mockUnitsMegabase)
	if err != nil || !approx(fn(3.0), 3.0/1e6) {
		t.Fatal("factory")
	}
	fn, _ = mockConverter.ConverterFactory(mockUnitsBase, mockUnitsBase)
	if fn(1.234) != 1.234 {
		t.Fatal("identity")
	}
	if _, err := mockConverter.ConverterFactory("not_a_unit", mockUnitsMegabase); !isUnknownUnit(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestGetUnitRatio(t *testing.T) {
	if r, _ := mockConverter.GetUnitRatio(mockUnitsBase, mockUnitsMegabase); !approx(r, 1e-6) {
		t.Fatal(r)
	}
	if _, err := mockConverter.GetUnitRatio("not_a_unit", mockUnitsMegabase); !isUnknownUnit(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestIsNonNegative(t *testing.T) {
	cases := []struct {
		value  any
		result bool
	}{
		{1.0, true}, {"0", true}, {123132, true},
		{-1.0, false}, {"-1", false},
		{nil, true}, {struct{}{}, true}, {map[string]any{}, true},
	}
	for _, c := range cases {
		if got := IsNonNegative(c.value); got != c.result {
			t.Errorf("IsNonNegative(%#v) = %v", c.value, got)
		}
	}
}

func TestReadAsBaseChecks(t *testing.T) {
	isPositive := func(v float64) bool { return v > 0 }
	lessThan1 := func(v float64) bool { return v < 1 }
	for _, checks := range [][]func(float64) bool{nil, {isPositive}, {isPositive, lessThan1}} {
		reader := ReadAsBase(mockConverter, mockUnitsMegabase, checks, 0.0)
		if got := reader(0.5); got != 0.5e6 {
			t.Errorf("checks pass: %v", got)
		}
	}
	reader := ReadAsBase(mockConverter, mockUnitsMegabase, []func(float64) bool{isPositive, lessThan1}, 0.0)
	if got := reader(1.5); got != 0.0 {
		t.Errorf("checks fail: %v", got)
	}
	if got := reader("not a number"); got != 0.0 {
		t.Errorf("unparseable: %v", got)
	}
}
