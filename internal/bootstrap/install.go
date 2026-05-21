package bootstrap

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Installer sets roadie up as a systemd service. Its paths and command runner
// are fields so tests can target a temp directory and a fake runner.
type Installer struct {
	BinDir        string // e.g. /usr/local/bin
	ConfigDir     string // e.g. /etc/roadie
	StateDir      string // e.g. /var/lib/roadie
	UnitDir       string // e.g. /etc/systemd/system
	DefaultConfig string // collections.toml contents, written when none exists
	UnitFile      string // systemd unit file contents
	Run           func(name string, args ...string) error
	In            io.Reader // confirmation-prompt input
	Out           io.Writer // prompt and report output
}

// DefaultInstaller returns an Installer targeting the real system paths.
func DefaultInstaller(defaultConfig, unitFile string) *Installer {
	return &Installer{
		BinDir:        "/usr/local/bin",
		ConfigDir:     "/etc/roadie",
		StateDir:      "/var/lib/roadie",
		UnitDir:       "/etc/systemd/system",
		DefaultConfig: defaultConfig,
		UnitFile:      unitFile,
		Run:           func(name string, args ...string) error { return exec.Command(name, args...).Run() },
		In:            os.Stdin,
		Out:           os.Stdout,
	}
}

// Install copies the executable at exePath into place, writes the config (only
// if absent), state directory and systemd unit, and enables the service —
// after printing the plan and getting a y/N confirmation.
func (in *Installer) Install(exePath string) error {
	binPath := filepath.Join(in.BinDir, "roadie")
	configPath := filepath.Join(in.ConfigDir, "collections.toml")
	unitPath := filepath.Join(in.UnitDir, "roadie.service")

	fmt.Fprintln(in.Out, "roadie install will:")
	fmt.Fprintf(in.Out, "  - install the binary to %s\n", binPath)
	fmt.Fprintf(in.Out, "  - create %s and write %s (if absent)\n", in.ConfigDir, configPath)
	fmt.Fprintf(in.Out, "  - create %s\n", in.StateDir)
	fmt.Fprintf(in.Out, "  - write %s and enable the service\n", unitPath)
	fmt.Fprint(in.Out, "Proceed? [y/N] ")
	if !confirm(in.In) {
		fmt.Fprintln(in.Out, "aborted.")
		return nil
	}

	for _, dir := range []string{in.BinDir, in.ConfigDir, in.StateDir, in.UnitDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	if err := copyFile(exePath, binPath, 0o755); err != nil {
		return fmt.Errorf("installing binary: %w", err)
	}
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(configPath, []byte(in.DefaultConfig), 0o644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("checking config: %w", err)
	}
	if err := os.WriteFile(unitPath, []byte(in.UnitFile), 0o644); err != nil {
		return fmt.Errorf("writing unit: %w", err)
	}
	if err := in.Run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if err := in.Run("systemctl", "enable", "--now", "roadie"); err != nil {
		return fmt.Errorf("systemctl enable: %w", err)
	}
	fmt.Fprintln(in.Out, "roadie installed and started.")
	return nil
}

// confirm reads one line and reports whether it is an affirmative y / yes.
func confirm(r io.Reader) bool {
	line, _ := bufio.NewReader(r).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// copyFile copies src to dst with the given mode via a temp file + rename.
// The explicit Chmod makes the mode umask-proof, so the installed binary is
// reliably executable whatever umask the installer runs under.
func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
