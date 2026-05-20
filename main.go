package main

import (
	"flag"
	"log"
	"net/http"

	"roadie/internal/config"
	"roadie/internal/manifest"
	"roadie/internal/server"
)

func main() {
	cfgPath := flag.String("config", "/etc/roadie/collections.toml", "path to collections.toml")
	manPath := flag.String("manifest", "/var/lib/roadie/manifest.json", "path to manifest.json")
	addr := flag.String("addr", "0.0.0.0:8473", "listen address")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	store, err := manifest.Open(*manPath)
	if err != nil {
		log.Fatalf("manifest: %v", err)
	}
	srv := server.New(cfg, store, server.WebHandler())
	log.Printf("roadie listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.Handler()))
}
