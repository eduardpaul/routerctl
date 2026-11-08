package units

import "testing"

func TestDataRateUnitFactors(t *testing.T) {
	cases := map[UnitOfDataRate]float64{
		UnitOfDataRateBytePerSecond: 8, UnitOfDataRateKilobitPerSecond: 1e3,
		UnitOfDataRateMegabitPerSecond: 1e6, UnitOfDataRateGigabitPerSecond: 1e9,
		UnitOfDataRateTerabitPerSecond: 1e12, UnitOfDataRateKibibitPerSecond: 1 << 10,
		UnitOfDataRateMebibitPerSecond: 1 << 20, UnitOfDataRateGibibitPerSecond: 1 << 30,
		UnitOfDataRateTebibitPerSecond: 1 << 40, UnitOfDataRateKilobytePerSecond: 8e3,
		UnitOfDataRateMegabytePerSecond: 8e6, UnitOfDataRateGigabytePerSecond: 8e9,
		UnitOfDataRateTerabytePerSecond: 8e12, UnitOfDataRateKibibytePerSecond: 1 << 13,
		UnitOfDataRateMebibytePerSecond: 1 << 23, UnitOfDataRateGibibytePerSecond: 1 << 33,
		UnitOfDataRateTebibytePerSecond: 1 << 43,
	}
	for unit, factor := range cases {
		if got, _ := DataRateUnitConverter.Convert(1.0, unit, UnitOfDataRateBitPerSecond); got != factor {
			t.Errorf("Convert(%s) = %v", unit, got)
		}
		if got, _ := DataRateUnitConverter.ConvertToBase(1.0, unit); got != factor {
			t.Errorf("ConvertToBase(%s) = %v", unit, got)
		}
	}
}

func TestReadDataRate(t *testing.T) {
	reader := ReadDataRate(UnitOfDataRateMebibitPerSecond)
	if got := reader(42.0); got != 42.0*(1<<20) {
		t.Fatal(got)
	}
	// Negative values fall back to 0
	if got := reader("-1"); got != 0.0 {
		t.Fatal(got)
	}
}
