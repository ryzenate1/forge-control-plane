package ignore

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// IgnoreList represents a list of patterns to ignore
type IgnoreList struct {
	patterns []string
}

// LoadIgnoreFile reads an ignore file (e.g., .pteroignore) and returns an IgnoreList
func LoadIgnoreFile(path string) (*IgnoreList, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &IgnoreList{}, nil
		}
		return nil, err
	}
	defer file.Close()
	return LoadIgnoreReader(file)
}

// LoadIgnoreReader parses ignore patterns from an already safely-opened file.
func LoadIgnoreReader(reader io.Reader) (*IgnoreList, error) {
	var patterns []string
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		patterns = append(patterns, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return &IgnoreList{patterns: patterns}, nil
}

// NewIgnoreList creates an IgnoreList from a slice of patterns
func NewIgnoreList(patterns []string) *IgnoreList {
	return &IgnoreList{patterns: patterns}
}

// IsIgnored checks if a path should be ignored
func (i *IgnoreList) IsIgnored(path string) bool {
	if len(i.patterns) == 0 {
		return false
	}

	// Normalize path separators
	normalizedPath := filepath.ToSlash(path)

	for _, pattern := range i.patterns {
		// Normalize pattern
		normalizedPattern := filepath.ToSlash(pattern)

		// Check if pattern matches the full path
		if matched, _ := filepath.Match(normalizedPattern, normalizedPath); matched {
			return true
		}

		// Check if pattern matches just the filename
		if matched, _ := filepath.Match(normalizedPattern, filepath.Base(path)); matched {
			return true
		}

		// Check if pattern matches any path component
		parts := strings.Split(normalizedPath, "/")
		for _, part := range parts {
			if matched, _ := filepath.Match(normalizedPattern, part); matched {
				return true
			}
		}

		// Handle directory patterns (ending with /)
		if strings.HasSuffix(normalizedPattern, "/") {
			dirPattern := strings.TrimSuffix(normalizedPattern, "/")
			for _, part := range parts {
				if matched, _ := filepath.Match(dirPattern, part); matched {
					return true
				}
			}
		}

		// Handle glob patterns with **
		if strings.Contains(normalizedPattern, "**") {
			// Convert ** to * for simple matching
			simplePattern := strings.ReplaceAll(normalizedPattern, "**", "*")
			if matched, _ := filepath.Match(simplePattern, normalizedPath); matched {
				return true
			}
		}
	}

	return false
}

// Patterns returns the list of ignore patterns
func (i *IgnoreList) Patterns() []string {
	return i.patterns
}

// LoadServerIgnore loads the .pteroignore file from the server root
func LoadServerIgnore(serverRoot string) (*IgnoreList, error) {
	ignorePath := filepath.Join(serverRoot, ".pteroignore")
	return LoadIgnoreFile(ignorePath)
}
