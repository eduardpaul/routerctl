// Base unit converter (base.py).

package units

import (
	"fmt"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// UnitConverterBase mirrors `UnitConverterBase`: a unit converter over
// the unit enum U. Python subclasses become package instances
// (`DataUnitConverter`, ...).
type UnitConverterBase[U ~string] struct {
	UnitClass string
	UnitRatio map[U]float64
}

func (c *UnitConverterBase[U]) unknownUnit(unit U) error {
	return arerror.NewAsusRouterError(fmt.Sprintf(
		"Unknown unit `%s` encountered during conversion of `%s`",
		string(unit), c.UnitClass,
	))
}

// GetUnitRatio mirrors `get_unit_ratio`: get unit ratio between units of
// measurement.
func (c *UnitConverterBase[U]) GetUnitRatio(fromUnit, toUnit U) (float64, error) {
	from, ok := c.UnitRatio[fromUnit]
	if !ok {
		return 0, c.unknownUnit(fromUnit)
	}
	to, ok := c.UnitRatio[toUnit]
	if !ok {
		return 0, c.unknownUnit(toUnit)
	}
	return from / to, nil
}

// Convert mirrors `convert`: convert a value from one unit to another.
func (c *UnitConverterBase[U]) Convert(value float64, fromUnit, toUnit U) (float64, error) {
	if fromUnit == toUnit {
		return value, nil
	}
	ratio, err := c.GetUnitRatio(fromUnit, toUnit)
	if err != nil {
		return 0, err
	}
	return value * ratio, nil
}

// ConvertToBase mirrors `convert_to_base`: convert a value to the base
// unit.
func (c *UnitConverterBase[U]) ConvertToBase(value float64, fromUnit U) (float64, error) {
	ratio, ok := c.UnitRatio[fromUnit]
	if !ok {
		return 0, c.unknownUnit(fromUnit)
	}
	return value * ratio, nil
}

// ConverterFactory mirrors `converter_factory`: create a conversion
// function from one unit to another.
func (c *UnitConverterBase[U]) ConverterFactory(fromUnit, toUnit U) (func(float64) float64, error) {
	if fromUnit == toUnit {
		return func(value float64) float64 { return value }, nil
	}

	factor, err := c.GetUnitRatio(fromUnit, toUnit)
	if err != nil {
		return nil, err
	}
	return func(value float64) float64 { return value * factor }, nil
}

// IsNonNegative mirrors `is_non_negative`: check if the value is
// non-negative (unparseable values pass).
func IsNonNegative(value any) bool {
	fval := converters.RawToFloat(value)
	return fval == nil || *fval >= 0
}

// ReadAsBase mirrors `read_as_base`: create a reader from the units to
// the base.
//
// The reader returns fallbackValue unless the value parses as a float
// and passes every check. Like Python, an unknown unit only fails when
// the reader converts: it panics with the AsusRouterError.
func ReadAsBase[U ~string](
	converter *UnitConverterBase[U],
	units U,
	checkCalls []func(float64) bool,
	fallbackValue float64,
) func(any) float64 {
	if converter == nil {
		panic("Converter must be a subclass of UnitConverterBase")
	}

	return func(value any) float64 {
		fval := converters.RawToFloat(value)
		if fval == nil {
			return fallbackValue
		}
		// Convert to base if all the checks passed
		for _, call := range checkCalls {
			if !call(*fval) {
				return fallbackValue
			}
		}
		result, err := converter.ConvertToBase(*fval, units)
		if err != nil {
			panic(err)
		}
		return result
	}
}
