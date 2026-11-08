// Time units (time.py).

package units

// UnitOfTime mirrors `UnitOfTime` (StrEnum).
// Units of time.
type UnitOfTime string

const (
	// Base unit
	UnitOfTimeSecond      UnitOfTime = "s"
	UnitOfTimeNanosecond  UnitOfTime = "ns"
	UnitOfTimeMicrosecond UnitOfTime = "us"
	UnitOfTimeMillisecond UnitOfTime = "ms"
	UnitOfTimeMinute      UnitOfTime = "min"
	UnitOfTimeHour        UnitOfTime = "h"
	UnitOfTimeDay         UnitOfTime = "d"
	UnitOfTimeWeek        UnitOfTime = "w"
)

// UnitOfTimeMembers lists the members in definition order.
var UnitOfTimeMembers = []UnitOfTime{
	UnitOfTimeSecond,
	UnitOfTimeNanosecond,
	UnitOfTimeMicrosecond,
	UnitOfTimeMillisecond,
	UnitOfTimeMinute,
	UnitOfTimeHour,
	UnitOfTimeDay,
	UnitOfTimeWeek,
}

var unitOfTimeNames = map[UnitOfTime]string{
	UnitOfTimeSecond:      "SECOND",
	UnitOfTimeNanosecond:  "NANOSECOND",
	UnitOfTimeMicrosecond: "MICROSECOND",
	UnitOfTimeMillisecond: "MILLISECOND",
	UnitOfTimeMinute:      "MINUTE",
	UnitOfTimeHour:        "HOUR",
	UnitOfTimeDay:         "DAY",
	UnitOfTimeWeek:        "WEEK",
}

// Name returns the Python member name.
func (e UnitOfTime) Name() string { return unitOfTimeNames[e] }

// Valid reports whether e is a defined member.
func (e UnitOfTime) Valid() bool { _, ok := unitOfTimeNames[e]; return ok }

// TimeUnitConverter mirrors `TimeUnitConverter`: Time Unit Converter.
var TimeUnitConverter = &UnitConverterBase[UnitOfTime]{
	UnitClass: "time",
	UnitRatio: map[UnitOfTime]float64{
		UnitOfTimeSecond:      1,
		UnitOfTimeNanosecond:  1e-09,
		UnitOfTimeMicrosecond: 1e-06,
		UnitOfTimeMillisecond: 0.001,
		UnitOfTimeMinute:      60,
		UnitOfTimeHour:        3600,
		UnitOfTimeDay:         86400,
		UnitOfTimeWeek:        604800,
	},
}
