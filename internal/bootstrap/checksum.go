package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// VerifySHA256 checks that data's SHA-256 digest matches the entry for
// assetName in a GoReleaser checksums.txt body (lines of "<hex>  <filename>",
// matched by exact bare filename). The hex comparison is case-insensitive. It
// returns an error if assetName is not listed or the digest does not match.
func VerifySHA256(data []byte, checksumsTxt, assetName string) error {
	var want string
	found := false
	for _, line := range strings.Split(checksumsTxt, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			want = strings.ToLower(fields[0])
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no checksum listed for %q", assetName)
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %q", assetName)
	}
	return nil
}
