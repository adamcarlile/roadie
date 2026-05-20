package syncer

import "testing"

func TestParseProgressLine(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		percent int
		rate    string
		eta     string
	}{
		{"mid-transfer", "    1,234,567  45%   12.34MB/s    0:01:23", 45, "12.34MB/s", "0:01:23"},
		{"final summary", "  9,999,999 100%    0.00kB/s    0:00:00 (xfr#1, to-chk=0/1)", 100, "0.00kB/s", "0:00:00"},
		{"zero-byte start", "          0   0%    0.00kB/s    0:00:00", 0, "0.00kB/s", "0:00:00"},
		{"stalled eta", "    1,234,567  45%   12.34MB/s    --:--:--", 45, "12.34MB/s", "--:--:--"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, ok := ParseProgress(c.line)
			if !ok {
				t.Fatalf("expected %q to parse", c.line)
			}
			if p.Percent != c.percent || p.Rate != c.rate || p.ETA != c.eta {
				t.Fatalf("parsed %+v, want percent=%d rate=%q eta=%q", p, c.percent, c.rate, c.eta)
			}
		})
	}
}

func TestParseProgressIgnoresNonProgressLines(t *testing.T) {
	for _, line := range []string{"", "sending incremental file list", "Up (2009).mkv"} {
		if _, ok := ParseProgress(line); ok {
			t.Errorf("line %q should not parse as progress", line)
		}
	}
}
