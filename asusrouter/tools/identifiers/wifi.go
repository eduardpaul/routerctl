// WiFi interface tools (wifi.py).

package identifiers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

var wifiSplit = regexp.MustCompile(`[. ]+`)

const wifiParts = 2

// WiFiInterface mirrors `WiFiInterface`: a wireless interface, a radio
// `unit` and its virtual `subunit`. It is comparable.
type WiFiInterface struct {
	Unit    int `json:"unit"`
	Subunit int `json:"subunit"`
}

// WiFiInterfaceFromValue mirrors `WiFiInterface.from_value`.
func WiFiInterfaceFromValue(value any) (WiFiInterface, error) {
	switch v := value.(type) {
	case WiFiInterface:
		return v, nil
	case *WiFiInterface:
		if v != nil {
			return *v, nil
		}
	}

	invalid := fmt.Errorf("Invalid WiFi interface: %s", converters.PyRepr(value))
	text := strings.TrimPrefix(converters.PyStrip(converters.PyStr(value)), "wl")
	parts := wifiSplit.Split(text, -1)
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		n, ok := pyInt10(part)
		if !ok {
			return WiFiInterface{}, invalid
		}
		numbers = append(numbers, n)
	}

	if len(numbers) == 0 || len(numbers) > wifiParts {
		return WiFiInterface{}, invalid
	}

	subunit := 0
	if len(numbers) == wifiParts {
		subunit = numbers[1]
	}
	return WiFiInterface{Unit: numbers[0], Subunit: subunit}, nil
}

// WiFiInterfaceFromValueSafe mirrors `WiFiInterface.from_value_safe`: nil
// when missing or unparseable.
func WiFiInterfaceFromValueSafe(value any) *WiFiInterface {
	if isNone(value) {
		return nil
	}
	iface, err := WiFiInterfaceFromValue(value)
	if err != nil {
		return nil
	}
	return &iface
}

// String returns the standard `unit.subunit` form.
func (w WiFiInterface) String() string {
	return fmt.Sprintf("%d.%d", w.Unit, w.Subunit)
}

// GoString returns the Python dataclass repr.
func (w WiFiInterface) GoString() string {
	return fmt.Sprintf("WiFiInterface(unit=%d, subunit=%d)", w.Unit, w.Subunit)
}

// pyInt10 mirrors Python `int(text)`: surrounding whitespace, a sign and
// single underscores between ASCII digits are accepted.
func pyInt10(text string) (int, bool) {
	s := converters.PyStrip(text)
	digits := strings.TrimLeft(s, "+-")
	if len(s)-len(digits) > 1 || digits == "" || digits[0] == '_' ||
		digits[len(digits)-1] == '_' || strings.Contains(digits, "__") {
		return 0, false
	}
	for _, c := range digits {
		if (c < '0' || c > '9') && c != '_' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(strings.ReplaceAll(s, "_", ""))
	return n, err == nil
}
