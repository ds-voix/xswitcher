// Package about holds build-time metadata injected via -ldflags.
// These variables are set at compile time and should never be hardcoded.
package about

var (
	// Version is the git tag or describe output (e.g. "v1.0.0-rc1").
	Version string = "dev"

	// Commit is the short git commit hash.
	Commit string = "unknown"

	// ChangesBy names who produced the changes carried by this tree.
	ChangesBy string = "BaHooo (bahooo22)"

	// UpstreamURL is the GitHub URL of the original project.
	UpstreamURL string = "https://github.com/ds-voix/xswitcher"

	// License is the project license identifier.
	License string = "AGPL-3.0"
)
