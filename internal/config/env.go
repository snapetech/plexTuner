package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadEnvFile reads path and sets environment variables for each line "KEY=value".
// Skips empty lines and lines starting with #. Use for .env (keep .env out of git).
// Path is cleaned with filepath.Clean to avoid traversal if path is user-influenced.
func LoadEnvFile(path string) error {
	path = filepath.Clean(path)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		value = unquoteEnv(value)
		os.Setenv(key, value)
	}
	return sc.Err()
}

func unquoteEnv(s string) string {
	if len(s) < 2 {
		return s
	}
	if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}
