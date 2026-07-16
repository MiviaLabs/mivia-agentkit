// Package version exposes the mivia-agent build version metadata.
// Plan: WS0, WS8. PRD: §1, §4, §9, NFR-1.
package version

// Version is the mivia-agent version and can be overridden with -ldflags.
var Version = "v0.3.1-dev"

// Commit is the source revision embedded at link time for release builds.
var Commit = "unknown"

// Date is the build timestamp embedded at link time for release builds.
var Date = "unknown"
