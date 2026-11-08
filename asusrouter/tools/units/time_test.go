package units

import "testing"

func TestTimeUnitConverter(t *testing.T) {
	cases := []struct {
		value    float64
		from, to UnitOfTime
		expected float64
	}{
		{1000.0, UnitOfTimeMillisecond, UnitOfTimeSecond, 1.0},
		{1e6, UnitOfTimeMicrosecond, UnitOfTimeSecond, 1.0},
		{1e9, UnitOfTimeNanosecond, UnitOfTimeSecond, 1.0},
		{2.0, UnitOfTimeMinute, UnitOfTimeSecond, 120.0},
		{1.0, UnitOfTimeHour, UnitOfTimeMinute, 60.0},
		{1.0, UnitOfTimeDay, UnitOfTimeHour, 24.0},
		{1.0, UnitOfTimeWeek, UnitOfTimeDay, 7.0},
	}
	for _, c := range cases {
		if got, _ := TimeUnitConverter.Convert(c.value, c.from, c.to); !approx(got, c.expected) {
			t.Errorf("Convert(%v, %s, %s) = %v", c.value, c.from, c.to, got)
		}
	}
	if got, _ := TimeUnitConverter.ConvertToBase(500.0, UnitOfTimeMillisecond); !approx(got, 0.5) {
		t.Fatal(got)
	}
}
