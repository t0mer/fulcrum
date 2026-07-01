// Package version exposes the build-injected version string.
package version

// Version is set at build time via -ldflags "-X .../internal/version.Version=<v>".
var Version = "dev"
