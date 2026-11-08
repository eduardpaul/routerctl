package units

import "testing"

func TestDataUnitFactors(t *testing.T) {
	cases := map[UnitOfData]float64{
		UnitOfDataByte: 8, UnitOfDataKilobit: 1e3, UnitOfDataMegabit: 1e6, UnitOfDataGigabit: 1e9,
		UnitOfDataTerabit: 1e12, UnitOfDataKibibit: 1 << 10, UnitOfDataMebibit: 1 << 20,
		UnitOfDataGibibit: 1 << 30, UnitOfDataTebibit: 1 << 40, UnitOfDataKilobyte: 8e3,
		UnitOfDataMegabyte: 8e6, UnitOfDataGigabyte: 8e9, UnitOfDataTerabyte: 8e12,
		UnitOfDataKibibyte: 1 << 13, UnitOfDataMebibyte: 1 << 23, UnitOfDataGibibyte: 1 << 33,
		UnitOfDataTebibyte: 1 << 43,
	}
	for unit, factor := range cases {
		if got, _ := DataUnitConverter.Convert(1.0, unit, UnitOfDataBit); got != factor {
			t.Errorf("Convert(%s) = %v", unit, got)
		}
		if got, _ := DataUnitConverter.ConvertToBase(1.0, unit); got != factor {
			t.Errorf("ConvertToBase(%s) = %v", unit, got)
		}
	}
}
