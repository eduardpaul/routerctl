// Data units (data.py).

package units

// UnitOfData mirrors `UnitOfData` (StrEnum).
// Units of data.
type UnitOfData string

const (
	// Base unit
	UnitOfDataBit  UnitOfData = "b"
	UnitOfDataByte UnitOfData = "B"
	// Base 10
	UnitOfDataKilobit  UnitOfData = "kb"
	UnitOfDataMegabit  UnitOfData = "Mb"
	UnitOfDataGigabit  UnitOfData = "Gb"
	UnitOfDataTerabit  UnitOfData = "Tb"
	UnitOfDataKilobyte UnitOfData = "KB"
	UnitOfDataMegabyte UnitOfData = "MB"
	UnitOfDataGigabyte UnitOfData = "GB"
	UnitOfDataTerabyte UnitOfData = "TB"
	// Base 2 (native Asus Type)
	UnitOfDataKibibit  UnitOfData = "Kib"
	UnitOfDataMebibit  UnitOfData = "Mib"
	UnitOfDataGibibit  UnitOfData = "Gib"
	UnitOfDataTebibit  UnitOfData = "Tib"
	UnitOfDataKibibyte UnitOfData = "KiB"
	UnitOfDataMebibyte UnitOfData = "MiB"
	UnitOfDataGibibyte UnitOfData = "GiB"
	UnitOfDataTebibyte UnitOfData = "TiB"
)

// UnitOfDataMembers lists the members in definition order.
var UnitOfDataMembers = []UnitOfData{
	UnitOfDataBit,
	UnitOfDataByte,
	UnitOfDataKilobit,
	UnitOfDataMegabit,
	UnitOfDataGigabit,
	UnitOfDataTerabit,
	UnitOfDataKilobyte,
	UnitOfDataMegabyte,
	UnitOfDataGigabyte,
	UnitOfDataTerabyte,
	UnitOfDataKibibit,
	UnitOfDataMebibit,
	UnitOfDataGibibit,
	UnitOfDataTebibit,
	UnitOfDataKibibyte,
	UnitOfDataMebibyte,
	UnitOfDataGibibyte,
	UnitOfDataTebibyte,
}

var unitOfDataNames = map[UnitOfData]string{
	UnitOfDataBit:      "BIT",
	UnitOfDataByte:     "BYTE",
	UnitOfDataKilobit:  "KILOBIT",
	UnitOfDataMegabit:  "MEGABIT",
	UnitOfDataGigabit:  "GIGABIT",
	UnitOfDataTerabit:  "TERABIT",
	UnitOfDataKilobyte: "KILOBYTE",
	UnitOfDataMegabyte: "MEGABYTE",
	UnitOfDataGigabyte: "GIGABYTE",
	UnitOfDataTerabyte: "TERABYTE",
	UnitOfDataKibibit:  "KIBIBIT",
	UnitOfDataMebibit:  "MEBIBIT",
	UnitOfDataGibibit:  "GIBIBIT",
	UnitOfDataTebibit:  "TEBIBIT",
	UnitOfDataKibibyte: "KIBIBYTE",
	UnitOfDataMebibyte: "MEBIBYTE",
	UnitOfDataGibibyte: "GIBIBYTE",
	UnitOfDataTebibyte: "TEBIBYTE",
}

// Name returns the Python member name.
func (e UnitOfData) Name() string { return unitOfDataNames[e] }

// Valid reports whether e is a defined member.
func (e UnitOfData) Valid() bool { _, ok := unitOfDataNames[e]; return ok }

// DataUnitConverter mirrors `DataUnitConverter`: Data Unit Converter.
var DataUnitConverter = &UnitConverterBase[UnitOfData]{
	UnitClass: "data",
	UnitRatio: map[UnitOfData]float64{
		UnitOfDataBit:      1,
		UnitOfDataByte:     8,
		UnitOfDataKilobit:  1000.0,
		UnitOfDataMegabit:  1000000.0,
		UnitOfDataGigabit:  1000000000.0,
		UnitOfDataTerabit:  1000000000000.0,
		UnitOfDataKibibit:  1024,
		UnitOfDataMebibit:  1048576,
		UnitOfDataGibibit:  1073741824,
		UnitOfDataTebibit:  1099511627776,
		UnitOfDataKilobyte: 8000.0,
		UnitOfDataMegabyte: 8000000.0,
		UnitOfDataGigabyte: 8000000000.0,
		UnitOfDataTerabyte: 8000000000000.0,
		UnitOfDataKibibyte: 8192,
		UnitOfDataMebibyte: 8388608,
		UnitOfDataGibibyte: 8589934592,
		UnitOfDataTebibyte: 8796093022208,
	},
}
