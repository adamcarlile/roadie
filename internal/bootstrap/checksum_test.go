package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestVerifySHA256(t *testing.T) {
	data := []byte("the roadie binary")
	sum := sha256.Sum256(data)
	checksums := hex.EncodeToString(sum[:]) + "  roadie_linux_amd64\n" +
		"0000000000000000000000000000000000000000000000000000000000000000  other\n"

	if err := VerifySHA256(data, checksums, "roadie_linux_amd64"); err != nil {
		t.Fatalf("VerifySHA256 on matching data: %v", err)
	}
	if err := VerifySHA256([]byte("tampered"), checksums, "roadie_linux_amd64"); err == nil {
		t.Error("expected a mismatch error for tampered data")
	}
	if err := VerifySHA256(data, checksums, "roadie_linux_arm64"); err == nil {
		t.Error("expected an error when the asset is absent from checksums.txt")
	}
}

func TestVerifySHA256KnownVector(t *testing.T) {
	// SHA-256("abc") is a published test vector — a hardcoded digest catches a
	// VerifySHA256 that mis-uses crypto/sha256 (a same-function fixture cannot).
	const abcSum = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if err := VerifySHA256([]byte("abc"), abcSum+"  roadie_linux_amd64\n", "roadie_linux_amd64"); err != nil {
		t.Fatalf("known SHA-256 vector should verify: %v", err)
	}
}

func TestVerifySHA256UppercaseHex(t *testing.T) {
	data := []byte("roadie")
	sum := sha256.Sum256(data)
	upper := strings.ToUpper(hex.EncodeToString(sum[:]))
	if err := VerifySHA256(data, upper+"  roadie_linux_amd64\n", "roadie_linux_amd64"); err != nil {
		t.Fatalf("an uppercase-hex checksum should still verify: %v", err)
	}
}
