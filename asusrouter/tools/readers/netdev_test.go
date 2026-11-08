package readers

import (
	"reflect"
	"testing"
)

func TestReadNetdevParsesInterfaces(t *testing.T) {
	// Each interface yields integer rx/tx from hex
	content := "netdev = {\n 'WIRED':{rx:0x0a,tx:0x14}\n,'WIRELESS0':{rx:0xff,tx:0x01}\n}"
	expected := map[string]map[string]int{
		"WIRED":     {"rx": 10, "tx": 20},
		"WIRELESS0": {"rx": 255, "tx": 1},
	}
	if got := ReadNetdev(content); !reflect.DeepEqual(got, expected) {
		t.Fatalf("got %v", got)
	}
}

func TestReadNetdevSumsDuplicateInternet(t *testing.T) {
	// Duplicate INTERNET entries (dual-WAN) are summed
	content := "netdev = {\n 'INTERNET':{rx:0x10,tx:0x20}\n,'INTERNET':{rx:0x01,tx:0x02}\n}"
	expected := map[string]map[string]int{"INTERNET": {"rx": 17, "tx": 34}}
	if got := ReadNetdev(content); !reflect.DeepEqual(got, expected) {
		t.Fatalf("got %v", got)
	}
}

func TestReadNetdevParsesFlatAppgetForm(t *testing.T) {
	// The flat appGet `IFACE_rx` form is parsed, duplicates summed
	content := `{"netdev":{"INTERNET_rx":"0x0a","INTERNET_tx":"0x14",` +
		`"INTERNET_rx":"0x01","INTERNET_tx":"0x02","WIRED_rx":"0x05"}}`
	expected := map[string]map[string]int{
		"INTERNET": {"rx": 11, "tx": 22},
		"WIRED":    {"rx": 5},
	}
	if got := ReadNetdev(content); !reflect.DeepEqual(got, expected) {
		t.Fatalf("got %v", got)
	}
}

func TestReadNetdevNoMatchYieldsEmpty(t *testing.T) {
	// Content without valid entries yields an empty dict
	for id, content := range map[string]string{
		"empty": "", "no_entries": "netdev = {}", "garbage": "garbage", "bad_hex": "'X':{rx:0xZZ,tx:0x1}",
	} {
		if got := ReadNetdev(content); len(got) != 0 {
			t.Errorf("%s: got %v", id, got)
		}
	}
}
