package serialdetect

import (
	"os"
	"path/filepath"
	"sort"
)

func List() ([]string, error) {
	patterns := []string{
		"/dev/ttyUSB*",
		"/dev/ttyACM*",
		"/dev/ttyS*",
		"/dev/ttyAMA*",
		"/dev/serial/by-id/*",
	}

	seen := make(map[string]struct{})
	ports := make([]string, 0)

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			if _, err := os.Stat(match); err != nil {
				continue
			}

			if _, ok := seen[match]; ok {
				continue
			}

			seen[match] = struct{}{}
			ports = append(ports, match)
		}
	}

	sort.Strings(ports)
	return ports, nil
}
