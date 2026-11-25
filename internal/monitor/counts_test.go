package monitor

import "testing"

// TestParseTempC reads the sensors: thermal zones report millidegrees and
// hwmon degrees, and the hottest of them is the one shown.
func TestParseTempC(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want float64
	}{
		{name: "no sensor", in: "\n\n", want: 0},
		{name: "millidegrees", in: "42000\n38500\n", want: 42},
		{name: "degrees", in: "42\n51\n", want: 51},
		{name: "the hottest wins", in: "42000\n61000\n", want: 61},
		{name: "a stray line is skipped", in: "n/a\n42000\n", want: 42},
		{name: "nothing sensible", in: "0\n0\n", want: 0},
		{name: "above boiling is no reading", in: "999000\n", want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseTempC(tc.in); got != tc.want {
				t.Errorf("temp = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestParseCount reads what wc -l prints.
func TestParseCount(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{in: "186\n", want: 186},
		{in: "  5  ", want: 5},
		{in: "", want: 0},
		{in: "n/a\n", want: 0},
		{in: "-1\n", want: 0},
	}
	for _, tc := range tests {
		if got := parseCount(tc.in); got != tc.want {
			t.Errorf("count(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
