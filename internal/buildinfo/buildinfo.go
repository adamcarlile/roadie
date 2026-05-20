// Package buildinfo carries build-time version information.
package buildinfo

// Version is the roadie version. It is "dev" for local builds and is set to
// the release tag at build time via -ldflags "-X .../buildinfo.Version=...".
var Version = "dev"
