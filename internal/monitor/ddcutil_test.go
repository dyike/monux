package monitor

import "testing"

func TestParseCurrentInput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   Input
	}{
		{"terse simple non-continuous", "VCP 60 SNC x0f", 0x0f},
		{"terse simple non-continuous with max", "VCP 60 SNC x11 x12", 0x11},
		{"terse continuous decimal", "VCP 60 C 15 18", 0x0f},
		{"terse continuous hex", "VCP 60 C 0x000f 0x0012", 0x0f},
		{"short hex", "VCP code 0x60 (Input Source): DisplayPort-1 (sl=0x0f)", 0x0f},
		{"decimal", "VCP code 0x60 (Input Source): current value = 17, max value = 18", 0x11},
		{"hex current", "current value = 0x11", 0x11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCurrentInput(tt.output)
			if err != nil {
				t.Fatalf("parseCurrentInput() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseCurrentInput() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestDDCUtilCommands(t *testing.T) {
	var gotName string
	var gotArgs []string
	d := NewDDCUtil(15)
	d.run = func(name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil, nil
	}

	if err := d.SetInput(0x11); err != nil {
		t.Fatalf("SetInput() error = %v", err)
	}
	if gotName != "ddcutil" {
		t.Fatalf("command = %q, want ddcutil", gotName)
	}
	want := []string{"--bus", "15", "setvcp", "60", "0x11"}
	if len(gotArgs) != len(want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Fatalf("args = %v, want %v", gotArgs, want)
		}
	}
}

func TestDDCUtilCurrentInputCommand(t *testing.T) {
	var gotArgs []string
	d := NewDDCUtil(15)
	d.run = func(_ string, args ...string) ([]byte, error) {
		gotArgs = append([]string(nil), args...)
		return []byte("VCP 60 SNC x0f\n"), nil
	}

	got, err := d.CurrentInput()
	if err != nil {
		t.Fatalf("CurrentInput() error = %v", err)
	}
	if got != 0x0f {
		t.Fatalf("CurrentInput() = %s, want 0x0f", got)
	}
	want := []string{"--bus", "15", "getvcp", "60", "--terse"}
	if len(gotArgs) != len(want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Fatalf("args = %v, want %v", gotArgs, want)
		}
	}
}
