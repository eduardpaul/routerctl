package ddns

// Non-enum content of Python `ddns/enums.py` (the enums themselves are
// generated in enums_gen.go).

// ActiveStatuses are the statuses the device treats as an active DDNS
// registration.
var ActiveStatuses = map[ARDdnsStatus]bool{
	ARDdnsStatusSuccess:            true,
	ARDdnsStatusRegisteredNew:      true,
	ARDdnsStatusRegisteredOriginal: true,
}
