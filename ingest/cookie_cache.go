package ingest

import (
	"fmt"
	"path/filepath"
	"strings"
)

func cookiesCacheFilePath(p videoPlatform) (string, error) {
	if strings.TrimSpace(p.ID) == "" {
		return "", fmt.Errorf("platform id is empty")
	}
	base, err := appStateDir()
	if err != nil {
		return "", err
	}

	// Backward compatibility: keep the YouTube filename stable.
	if p.ID == "youtube" {
		return filepath.Join(base, "youtube-cookies.txt"), nil
	}

	return filepath.Join(base, p.ID+"-cookies.txt"), nil
}

