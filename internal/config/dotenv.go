package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func readEnvFile(path string) (map[string]string, error) {
	values := make(map[string]string)
	if strings.TrimSpace(path) == "" {
		return values, nil
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return values, nil
		}
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("read env file %s: line %d must be KEY=VALUE", path, lineNo)
		}

		key = strings.TrimSpace(key)
		if !validEnvKey(key) {
			return nil, fmt.Errorf("read env file %s: line %d has invalid key", path, lineNo)
		}

		values[key] = parseEnvValue(value)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}

	return values, nil
}

func overlayLookup(fileValues map[string]string, lookup LookupFunc) LookupFunc {
	return func(key string) (string, bool) {
		if value, ok := lookup(key); ok {
			return value, true
		}
		value, ok := fileValues[key]
		return value, ok
	}
}

func parseEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}

	if index := strings.Index(value, " #"); index >= 0 {
		value = value[:index]
	}

	return strings.TrimSpace(value)
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}

	for i, r := range key {
		if r == '_' || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') || (i > 0 && '0' <= r && r <= '9') {
			continue
		}
		return false
	}

	return true
}
