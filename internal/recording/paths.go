package recording

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Directory resolves a caller override, an environment override, then the
// platform's user data directory. Resolving paths does not create directories.
func Directory(override string) (string, error) {
	root := strings.TrimSpace(override)
	if root == "" {
		root = strings.TrimSpace(os.Getenv("WATTSCOPE_RECORDINGS_DIR"))
	}
	if root != "" {
		if root == "~" || strings.HasPrefix(root, "~/") || strings.HasPrefix(root, "~\\") {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			if root == "~" {
				root = home
			} else {
				root = filepath.Join(home, root[2:])
			}
		}
		return filepath.Abs(root)
	}
	var err error
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		root, err = os.UserConfigDir()
	} else {
		root = os.Getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(root) {
			var home string
			home, err = os.UserHomeDir()
			root = filepath.Join(home, ".local", "share")
		}
	}
	if err != nil {
		return "", fmt.Errorf("resolve recordings directory (set --recordings-dir): %w", err)
	}
	return filepath.Abs(filepath.Join(root, "wattscope", "recordings"))
}

// createFile resolves the destination and exclusively creates a private file.
// Both recording formats use this path so neither can overwrite existing data.
func createFile(path string) (*os.File, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}

func NextPath(directory string) string {
	return filepath.Join(directory, "wattscope-"+time.Now().Format("20060102-150405.000000000")+".csv")
}

func NextCapturePath(directory string) string {
	return filepath.Join(directory, "wattscope-"+time.Now().Format("20060102-150405.000000000")+".pd.jsonl")
}
