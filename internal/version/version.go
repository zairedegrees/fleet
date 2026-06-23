// Package version is the single source of truth for the fleet version string,
// shared by the CLI (`fleet --version`) and the embedded coord (MCP serverInfo).
// Release builds stamp Version via -ldflags
// "-X github.com/zairedegrees/fleet/internal/version.Version=<tag>"; the literal
// is the source-build fallback for `go build` / `go install`.
package version

var Version = "0.3.0"
