package hub

import (
	"path/filepath"
	"strings"

	"github.com/GoogleCloudPlatform/scion/pkg/util"
)

// normalizeCloneURLLabel preserves explicit clone URL overrides while still
// normalizing schemeless git remotes stored in grove labels.
func normalizeCloneURLLabel(cloneURL string) string {
	if cloneURL == "" {
		return ""
	}

	lower := strings.ToLower(cloneURL)
	for _, prefix := range []string{"http://", "https://", "ssh://", "git://"} {
		if strings.HasPrefix(lower, prefix) {
			return cloneURL
		}
	}
	if strings.HasPrefix(cloneURL, "git@") {
		return cloneURL
	}
	if filepath.IsAbs(cloneURL) || strings.HasPrefix(cloneURL, "./") || strings.HasPrefix(cloneURL, "../") {
		return cloneURL
	}

	return util.ToHTTPSCloneURL(cloneURL)
}

func resolveCloneURL(override, gitRemote string) string {
	override = normalizeCloneURLLabel(override)
	if override != "" {
		return override
	}

	return normalizeCloneURLLabel(gitRemote)
}
