package bootstrap

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempInstaller builds an Installer rooted entirely in a temp directory, with a
// fake command runner that records calls. confirmInput is fed to the prompt.
func tempInstaller(t *testing.T, confirmInput string) (*Installer, *[][]string) {
	t.Helper()
	root := t.TempDir()
	var calls [][]string
	in := &Installer{
		BinDir:        filepath.Join(root, "bin"),
		ConfigDir:     filepath.Join(root, "etc"),
		StateDir:      filepath.Join(root, "state"),
		UnitDir:       filepath.Join(root, "units"),
		DefaultConfig: "# default config\n",
		UnitFile:      "[Unit]\n",
		Run: func(name string, args ...string) error {
			calls = append(calls, append([]string{name}, args...))
			return nil
		},
		In:  strings.NewReader(confirmInput),
		Out: &bytes.Buffer{},
	}
	return in, &calls
}

func fakeExe(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "roadie")
	if err := os.WriteFile(p, []byte("ELF-ish binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInstallCreatesEverything(t *testing.T) {
	in, calls := tempInstaller(t, "y\n")
	if err := in.Install(fakeExe(t)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(in.BinDir, "roadie")); err != nil || string(b) != "ELF-ish binary" {
		t.Errorf("binary not installed: %v", err)
	}
	info, err := os.Stat(filepath.Join(in.BinDir, "roadie"))
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("installed binary mode = %v, want 0755", info.Mode().Perm())
	}
	if b, err := os.ReadFile(filepath.Join(in.ConfigDir, "collections.toml")); err != nil || string(b) != "# default config\n" {
		t.Errorf("config not written: %v", err)
	}
	if _, err := os.Stat(in.StateDir); err != nil {
		t.Errorf("state dir not created: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(in.UnitDir, "roadie.service")); err != nil || string(b) != "[Unit]\n" {
		t.Errorf("unit not written: %v", err)
	}
	if len(*calls) != 2 || (*calls)[0][1] != "daemon-reload" || (*calls)[1][2] != "--now" {
		t.Errorf("systemctl calls = %v", *calls)
	}
}

func TestInstallDeclined(t *testing.T) {
	in, calls := tempInstaller(t, "n\n")
	if err := in.Install(fakeExe(t)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(in.BinDir, "roadie")); !os.IsNotExist(err) {
		t.Error("declining should install nothing")
	}
	if len(*calls) != 0 {
		t.Errorf("declining should run no commands, got %v", *calls)
	}
}

func TestInstallReportsRunFailure(t *testing.T) {
	in, _ := tempInstaller(t, "y\n")
	in.Run = func(name string, args ...string) error { return fmt.Errorf("systemctl boom") }
	if err := in.Install(fakeExe(t)); err == nil {
		t.Fatal("Install should return an error when a systemctl command fails")
	}
}

func TestInstallKeepsExistingConfig(t *testing.T) {
	in, _ := tempInstaller(t, "y\n")
	if err := os.MkdirAll(in.ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(in.ConfigDir, "collections.toml")
	if err := os.WriteFile(existing, []byte("# my edited config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := in.Install(fakeExe(t)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if b, _ := os.ReadFile(existing); string(b) != "# my edited config\n" {
		t.Error("Install must not clobber an existing config")
	}
}
