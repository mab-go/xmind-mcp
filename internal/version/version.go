// Package version holds build metadata injected via ldflags.
//
// Example:
//
//	go build -ldflags "-X github.com/mab-go/xmind-mcp/internal/version.Version=1.0.0 \
//	  -X github.com/mab-go/xmind-mcp/internal/version.Commit=$(git rev-parse HEAD) \
//	  -X github.com/mab-go/xmind-mcp/internal/version.Date=$(date -u +%Y-%m-%d)" ./cmd/xmind-mcp
package version

// Version is the release / semantic version string.
var Version = "0.0.0"

// Commit is the full VCS revision (e.g. git SHA).
var Commit = "0000000000000000000000000000000000000000"

// Date is the build date in UTC (conventionally YYYY-MM-DD).
var Date = "0000-00-00"

// ShortCommit returns up to 12 characters of Commit for compact display.
func ShortCommit() string {
	if len(Commit) >= 12 {
		return Commit[:12]
	}
	return Commit
}
