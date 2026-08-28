package monitor

import "testing"

func TestParseInput(t *testing.T) {
	for value, want := range map[string]Input{"0x0f": 0x0f, "17": 17, " 0X11 ": 0x11, "08": 8} {
		got, err := ParseInput(value)
		if err != nil {
			t.Fatalf("ParseInput(%q) error = %v", value, err)
		}
		if got != want {
			t.Fatalf("ParseInput(%q) = %s, want %s", value, got, want)
		}
	}
	if _, err := ParseInput("not-a-number"); err == nil {
		t.Fatal("ParseInput(invalid) error = nil")
	}
}

func TestConnectorName(t *testing.T) {
	if got := Input(0x11).ConnectorName(); got != "HDMI 1" {
		t.Fatalf("ConnectorName() = %q", got)
	}
	if got := Input(0xff).ConnectorName(); got != "Unknown" {
		t.Fatalf("ConnectorName(unknown) = %q", got)
	}
}
