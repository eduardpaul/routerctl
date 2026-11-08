// Data rate units (data_rate.py).

package units

// UnitOfDataRate mirrors `UnitOfDataRate` (StrEnum).
// Units of data rate.
type UnitOfDataRate string

const (
	// Base unit
	UnitOfDataRateBitPerSecond  UnitOfDataRate = "bps"
	UnitOfDataRateBytePerSecond UnitOfDataRate = "Bps"
	// Base 10
	UnitOfDataRateKilobitPerSecond  UnitOfDataRate = "kbps"
	UnitOfDataRateMegabitPerSecond  UnitOfDataRate = "Mbps"
	UnitOfDataRateGigabitPerSecond  UnitOfDataRate = "Gbps"
	UnitOfDataRateTerabitPerSecond  UnitOfDataRate = "Tbps"
	UnitOfDataRateKilobytePerSecond UnitOfDataRate = "KBps"
	UnitOfDataRateMegabytePerSecond UnitOfDataRate = "MBps"
	UnitOfDataRateGigabytePerSecond UnitOfDataRate = "GBps"
	UnitOfDataRateTerabytePerSecond UnitOfDataRate = "TBps"
	// Base 2 (native Asus Type)
	UnitOfDataRateKibibitPerSecond  UnitOfDataRate = "Kibps"
	UnitOfDataRateMebibitPerSecond  UnitOfDataRate = "Mibps"
	UnitOfDataRateGibibitPerSecond  UnitOfDataRate = "Gibps"
	UnitOfDataRateTebibitPerSecond  UnitOfDataRate = "Tibps"
	UnitOfDataRateKibibytePerSecond UnitOfDataRate = "KiBps"
	UnitOfDataRateMebibytePerSecond UnitOfDataRate = "MiBps"
	UnitOfDataRateGibibytePerSecond UnitOfDataRate = "GiBps"
	UnitOfDataRateTebibytePerSecond UnitOfDataRate = "TiBps"
)

// UnitOfDataRateMembers lists the members in definition order.
var UnitOfDataRateMembers = []UnitOfDataRate{
	UnitOfDataRateBitPerSecond,
	UnitOfDataRateBytePerSecond,
	UnitOfDataRateKilobitPerSecond,
	UnitOfDataRateMegabitPerSecond,
	UnitOfDataRateGigabitPerSecond,
	UnitOfDataRateTerabitPerSecond,
	UnitOfDataRateKilobytePerSecond,
	UnitOfDataRateMegabytePerSecond,
	UnitOfDataRateGigabytePerSecond,
	UnitOfDataRateTerabytePerSecond,
	UnitOfDataRateKibibitPerSecond,
	UnitOfDataRateMebibitPerSecond,
	UnitOfDataRateGibibitPerSecond,
	UnitOfDataRateTebibitPerSecond,
	UnitOfDataRateKibibytePerSecond,
	UnitOfDataRateMebibytePerSecond,
	UnitOfDataRateGibibytePerSecond,
	UnitOfDataRateTebibytePerSecond,
}

var unitOfDataRateNames = map[UnitOfDataRate]string{
	UnitOfDataRateBitPerSecond:      "BIT_PER_SECOND",
	UnitOfDataRateBytePerSecond:     "BYTE_PER_SECOND",
	UnitOfDataRateKilobitPerSecond:  "KILOBIT_PER_SECOND",
	UnitOfDataRateMegabitPerSecond:  "MEGABIT_PER_SECOND",
	UnitOfDataRateGigabitPerSecond:  "GIGABIT_PER_SECOND",
	UnitOfDataRateTerabitPerSecond:  "TERABIT_PER_SECOND",
	UnitOfDataRateKilobytePerSecond: "KILOBYTE_PER_SECOND",
	UnitOfDataRateMegabytePerSecond: "MEGABYTE_PER_SECOND",
	UnitOfDataRateGigabytePerSecond: "GIGABYTE_PER_SECOND",
	UnitOfDataRateTerabytePerSecond: "TERABYTE_PER_SECOND",
	UnitOfDataRateKibibitPerSecond:  "KIBIBIT_PER_SECOND",
	UnitOfDataRateMebibitPerSecond:  "MEBIBIT_PER_SECOND",
	UnitOfDataRateGibibitPerSecond:  "GIBIBIT_PER_SECOND",
	UnitOfDataRateTebibitPerSecond:  "TEBIBIT_PER_SECOND",
	UnitOfDataRateKibibytePerSecond: "KIBIBYTE_PER_SECOND",
	UnitOfDataRateMebibytePerSecond: "MEBIBYTE_PER_SECOND",
	UnitOfDataRateGibibytePerSecond: "GIBIBYTE_PER_SECOND",
	UnitOfDataRateTebibytePerSecond: "TEBIBYTE_PER_SECOND",
}

// Name returns the Python member name.
func (e UnitOfDataRate) Name() string { return unitOfDataRateNames[e] }

// Valid reports whether e is a defined member.
func (e UnitOfDataRate) Valid() bool { _, ok := unitOfDataRateNames[e]; return ok }

// DataRateUnitConverter mirrors `DataRateUnitConverter`: Data Rate Unit Converter.
var DataRateUnitConverter = &UnitConverterBase[UnitOfDataRate]{
	UnitClass: "data_rate",
	UnitRatio: map[UnitOfDataRate]float64{
		UnitOfDataRateBitPerSecond:      1,
		UnitOfDataRateBytePerSecond:     8,
		UnitOfDataRateKilobitPerSecond:  1000.0,
		UnitOfDataRateMegabitPerSecond:  1000000.0,
		UnitOfDataRateGigabitPerSecond:  1000000000.0,
		UnitOfDataRateTerabitPerSecond:  1000000000000.0,
		UnitOfDataRateKibibitPerSecond:  1024,
		UnitOfDataRateMebibitPerSecond:  1048576,
		UnitOfDataRateGibibitPerSecond:  1073741824,
		UnitOfDataRateTebibitPerSecond:  1099511627776,
		UnitOfDataRateKilobytePerSecond: 8000.0,
		UnitOfDataRateMegabytePerSecond: 8000000.0,
		UnitOfDataRateGigabytePerSecond: 8000000000.0,
		UnitOfDataRateTerabytePerSecond: 8000000000000.0,
		UnitOfDataRateKibibytePerSecond: 8192,
		UnitOfDataRateMebibytePerSecond: 8388608,
		UnitOfDataRateGibibytePerSecond: 8589934592,
		UnitOfDataRateTebibytePerSecond: 8796093022208,
	},
}

// ReadDataRate mirrors `read_data_rate`: read data rate values from the
// specified unit type.
func ReadDataRate(units UnitOfDataRate) func(any) float64 {
	return ReadAsBase(DataRateUnitConverter, units, []func(float64) bool{func(v float64) bool { return IsNonNegative(v) }}, 0.0)
}
