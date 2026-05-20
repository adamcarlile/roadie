package syncer

import "testing"

func TestFreeBytesReportsPositive(t *testing.T) {
	n, err := FreeBytes(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Fatalf("FreeBytes = %d, want > 0", n)
	}
}

func TestIsMountPointFalseForOrdinaryDir(t *testing.T) {
	if IsMountPoint(t.TempDir()) {
		t.Error("IsMountPoint should be false for an ordinary temp directory")
	}
}
