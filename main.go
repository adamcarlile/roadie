package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"roadie/internal/bootstrap"
	"roadie/internal/buildinfo"
	"roadie/internal/config"
	"roadie/internal/manifest"
	"roadie/internal/server"
)

//go:embed collections.toml.example
var defaultConfig string

//go:embed deploy/roadie.service
var unitFile string

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "version":
			fmt.Println(buildinfo.Version)
			return
		case "install":
			runInstall()
			return
		case "update":
			runUpdate()
			return
		case "serve":
			args = args[1:] // serve consumes the rest as flags
		default:
			// A leading flag (e.g. `roadie -addr ...`) is for serve; a bare
			// unknown word is a mistyped subcommand and should not start a server.
			if !strings.HasPrefix(args[0], "-") {
				fmt.Fprintf(os.Stderr, "roadie: unknown subcommand %q\n", args[0])
				fmt.Fprintln(os.Stderr, "usage: roadie [serve|install|update|version]")
				os.Exit(2)
			}
		}
	}
	serve(args)
}

// runInstall installs roadie as a service (requires root).
func runInstall() {
	if os.Geteuid() != 0 {
		log.Fatal("roadie install must be run as root (try: sudo roadie install)")
	}
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("locating executable: %v", err)
	}
	if err := bootstrap.DefaultInstaller(defaultConfig, unitFile).Install(exe); err != nil {
		log.Fatalf("install: %v", err)
	}
}

// runUpdate updates the installed roadie binary (requires root).
func runUpdate() {
	if os.Geteuid() != 0 {
		log.Fatal("roadie update must be run as root (try: sudo roadie update)")
	}
	if err := bootstrap.DefaultUpdater(buildinfo.Version).Update(); err != nil {
		log.Fatalf("update: %v", err)
	}
}

// serve runs the roadie HTTP server, parsing args as flags.
func serve(args []string) {
	fs := flag.NewFlagSet("roadie serve", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/roadie/collections.toml", "path to collections.toml")
	manPath := fs.String("manifest", "/var/lib/roadie/manifest.json", "path to manifest.json")
	addr := fs.String("addr", "0.0.0.0:8473", "listen address")
	fs.Parse(args) // flag.ExitOnError: a bad flag exits here, never returns an error

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	store, err := manifest.Open(*manPath)
	if err != nil {
		log.Fatalf("manifest: %v", err)
	}
	srv := server.New(cfg, store, server.WebHandler())
	log.Printf("roadie %s listening on %s", buildinfo.Version, *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.Handler()))
}
