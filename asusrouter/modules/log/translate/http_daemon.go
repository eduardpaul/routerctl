package translate

// HTTP daemon log events for AsusRouter.

import (
	"fmt"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/connection"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/internal/pyre"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const httpDaemonDateParts = 3

// Date is the Go form of a Python `date` (canonical: `isoformat()`).
type Date struct {
	Year  int
	Month int
	Day   int
}

// CanonicalValue renders the date like Python `date.isoformat()`.
func (d Date) CanonicalValue() any {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// newDate mirrors `date(year, month, day)` (false where it raises).
func newDate(year, month, day int) (Date, bool) {
	if year < 1 || year > 9999 || month < 1 || month > 12 || day < 1 {
		return Date{}, false
	}
	days := [13]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}[month]
	if month == 2 && year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		days = 29
	}
	if day > days {
		return Date{}, false
	}
	return Date{year, month, day}, true
}

// httpDaemonReadDate parses a `YYYY/M/D` certificate date, or None.
func httpDaemonReadDate(text string) any {
	parts := strings.Split(text, "/")
	if len(parts) != httpDaemonDateParts {
		return nil
	}
	var numbers [3]int
	for i, part := range parts {
		value, ok := pyre.Int(part, 10)
		if !ok {
			return nil
		}
		numbers[i] = value
	}
	d, ok := newDate(numbers[0], numbers[1], numbers[2])
	if !ok {
		return nil
	}
	return d
}

// httpDaemonReadStatus maps a firmware status word.
func httpDaemonReadStatus(word string) any {
	lowered := strings.ToLower(word)
	if strings.HasPrefix(lowered, "succe") {
		return common.ARStatusCodeSuccess
	}
	if strings.HasPrefix(lowered, "fail") {
		return common.ARStatusCodeFail
	}
	return common.ARStatusCodeUnknown
}

// HttpDaemonPatterns mirrors `http_daemon.PATTERNS`.
var HttpDaemonPatterns = NewARLogPatternSet(
	AREventHttpDaemonUnknown,
	// The SSL port trails a literal `...`
	MustARLogPattern(
		AREventHttpDaemonCertificateGenerate,
		`Generating SSL certificate\.\.\.(?P<port>\d+)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyPort: Int},
			Marker:  "Generating SSL certificate",
		},
	),
	// Subject and issuer name the device and its AiMesh root as
	// `<model>-<last 2 MAC octets>`, so the raw stays unclassified
	// TODO: decide whether part-MAC is sensitive
	MustARLogPattern(
		AREventHttpDaemonCertificateInfo,
		`S:(?P<subject>.+?) Server Certificate,\s*`+
			`I:(?P<issuer>.+?) Root Certificate\s+(?P<serial>\S+),\s*`+
			`(?P<valid_from>[\d/]+)\s*~\s*(?P<valid_to>[\d/]+)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyValidFrom: Plain(httpDaemonReadDate),
				log.AREventKeyValidTo:   Plain(httpDaemonReadDate),
			},
			Marker: "Server Certificate",
		},
	),
	MustARLogPattern(
		AREventHttpDaemonCertificateInit,
		`(?P<status>\w+) to init SSL certificate\.\.\.(?P<port>\d+)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyPort:   Int,
				log.AREventKeyStatus: Plain(httpDaemonReadStatus),
			},
			Marker: "to init SSL certificate",
		},
	),
	MustARLogPattern(
		AREventHttpDaemonCertificateReload,
		`reload cert and clean all files`,
		PatternOptions{Marker: "reload cert and clean all files"},
	),
	MustARLogPattern(
		AREventHttpDaemonCertificateRestore,
		`Restore saved SSL certificate\.\.\.(?P<port>\d+)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyPort: Int},
			Marker:  "Restore saved SSL certificate",
		},
	),
	MustARLogPattern(
		AREventHttpDaemonLogin,
		`\[LOGIN\]\[(?P<connection>[A-Za-z]+)\]`+
			`\[(?P<api_client>[A-Za-z]+)\]`+
			`\s+(?P<status>[A-Za-z]+)\s+\((?P<client_ip>[\d.]+)\)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyApiClient:  Plain(func(s string) any { return common.ARApiClientFromValue(s) }),
				log.AREventKeyClientIp:   FromValueSafe(identifiers.IpAddressFromValueSafe),
				log.AREventKeyConnection: Plain(func(s string) any { return connection.ARConnectionFromValue(s) }),
				log.AREventKeyStatus:     Plain(httpDaemonReadStatus),
			},
			Marker: "[LOGIN]",
		},
	),
)
