package roles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ReadRole(s string) (string, error) {
	if !strings.Contains(s, "/") {
		s = filepath.Join("roles", s)
	}
	if s == "" {
		return "", nil
	}
	if !strings.HasSuffix(s, "*") && !strings.HasSuffix(s, ".md") {
		s += "*"
	}
	matches, err := filepath.Glob(s)
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("found %d matches", len(matches))
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		return "", err
	}
	s = string(b)

	// TODO: templateData

	return s, nil
}
