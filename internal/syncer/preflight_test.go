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

func TestDriveMounted(t *testing.T) {
	// "/" is the root filesystem itself — never a separately mounted drive.
	if DriveMounted("/") {
		t.Error("DriveMounted(/) = true, want false")
	}
	// A missing path under "/" resolves up to the root device.
	if DriveMounted("/no/such/path/roadie-test") {
		t.Error("DriveMounted of a missing root-fs path = true, want false")
	}
}
