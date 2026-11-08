package firmware

import "testing"

func TestReadFirmwareNoteStripsHeadersAndBom(t *testing.T) {
	content := bom + "Firmware version 3.0.0.4\nRelease Note\n- Fix A\n- Fix B\n"
	got := ReadFirmwareNote(content)
	if got == nil || *got != "- Fix A\n- Fix B" {
		t.Fatalf("got %v", got)
	}
}

func TestReadFirmwareNoteEmptyReturnsNone(t *testing.T) {
	for _, content := range []string{
		"",
		"\n\n",
		"   \n  \n",
		"Firmware version 1\nRelease Note\n",
	} {
		if got := ReadFirmwareNote(content); got != nil {
			t.Errorf("%q: got %q", content, *got)
		}
	}
}
