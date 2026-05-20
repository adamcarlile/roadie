// Package syncer runs rsync transfers driven by the manifest.
package syncer

import (
	"regexp"
	"strconv"
)

// Progress is a parsed rsync --info=progress2 status line.
type Progress struct {
	Percent int    `json:"percent"`
	Rate    string `json:"rate"`
	ETA     string `json:"eta"`
}

// progress2Re matches an rsync --info=progress2 line, e.g.
// "  1,234,567  45%  12.34MB/s  0:01:23". It is anchored to the leading byte
// count so a filename containing a "%" cannot be misread as progress, and the
// ETA group also accepts rsync's "--:--:--" stalled form.
var progress2Re = regexp.MustCompile(`^\s*[\d,]+\s+(\d+)%\s+(\S+)\s+(\d+:\d+:\d+|--:--:--)`)

// ParseProgress extracts a Progress from an rsync progress2 line. The second
// return is false if the line is not a progress line.
func ParseProgress(line string) (Progress, bool) {
	m := progress2Re.FindStringSubmatch(line)
	if m == nil {
		return Progress{}, false
	}
	pct, err := strconv.Atoi(m[1])
	if err != nil {
		return Progress{}, false
	}
	return Progress{Percent: pct, Rate: m[2], ETA: m[3]}, true
}
