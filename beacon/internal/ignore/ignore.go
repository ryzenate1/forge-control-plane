package ignore

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type ignoreRule struct {
	pattern string
	negated bool
	matcher *regexp.Regexp
}

// IgnoreList is an ordered gitignore-style rule set. Later matching rules,
// including negations, override earlier rules.
type IgnoreList struct {
	patterns []string
	rules    []ignoreRule
}

func LoadIgnoreFile(filePath string) (*IgnoreList, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &IgnoreList{}, nil
		}
		return nil, err
	}
	defer file.Close()
	return LoadIgnoreReader(file)
}

func LoadIgnoreReader(reader io.Reader) (*IgnoreList, error) {
	var patterns []string
	scanner := bufio.NewScanner(io.LimitReader(reader, 1<<20))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return compileIgnoreList(patterns)
}

// NewIgnoreList creates a list from already trusted patterns. Invalid patterns
// are retained for display but never match.
func NewIgnoreList(patterns []string) *IgnoreList {
	list, err := compileIgnoreList(patterns)
	if err != nil {
		return &IgnoreList{patterns: append([]string(nil), patterns...)}
	}
	return list
}

func compileIgnoreList(patterns []string) (*IgnoreList, error) {
	list := &IgnoreList{patterns: append([]string(nil), patterns...)}
	for _, original := range patterns {
		rule, err := compileRule(original)
		if err != nil {
			return nil, fmt.Errorf("invalid ignore pattern %q: %w", original, err)
		}
		if rule.matcher != nil {
			list.rules = append(list.rules, rule)
		}
	}
	return list, nil
}

func compileRule(original string) (ignoreRule, error) {
	value := filepath.ToSlash(strings.TrimSpace(original))
	rule := ignoreRule{pattern: original}
	if value == "" || strings.HasPrefix(value, "#") {
		return rule, nil
	}
	if strings.HasPrefix(value, `\#`) || strings.HasPrefix(value, `\!`) {
		value = value[1:]
	} else if strings.HasPrefix(value, "!") {
		rule.negated = true
		value = strings.TrimPrefix(value, "!")
	}
	anchored := strings.HasPrefix(value, "/")
	value = strings.TrimPrefix(value, "/")
	directory := strings.HasSuffix(value, "/")
	value = strings.TrimSuffix(value, "/")
	if value == "" || strings.ContainsRune(value, '\x00') {
		return rule, fmt.Errorf("empty or unsafe pattern")
	}

	var expression strings.Builder
	if anchored || strings.Contains(value, "/") {
		expression.WriteString("^")
	} else {
		expression.WriteString(`(?:^|.*/)`)
	}
	for index := 0; index < len(value); {
		switch value[index] {
		case '*':
			if index+1 < len(value) && value[index+1] == '*' {
				index += 2
				if index < len(value) && value[index] == '/' {
					expression.WriteString(`(?:.*/)?`)
					index++
				} else {
					expression.WriteString(`.*`)
				}
				continue
			}
			expression.WriteString(`[^/]*`)
		case '?':
			expression.WriteString(`[^/]`)
		case '[':
			end := strings.IndexByte(value[index+1:], ']')
			if end < 0 {
				return rule, fmt.Errorf("unterminated character class")
			}
			end += index + 1
			class := value[index+1 : end]
			if strings.HasPrefix(class, "!") {
				class = "^" + regexp.QuoteMeta(class[1:])
			} else {
				class = regexp.QuoteMeta(class)
			}
			expression.WriteString("[" + class + "]")
			index = end
		default:
			expression.WriteString(regexp.QuoteMeta(string(value[index])))
		}
		index++
	}
	if directory || !strings.Contains(value, "/") {
		expression.WriteString(`(?:/.*)?$`)
	} else {
		expression.WriteString(`$`)
	}
	matcher, err := regexp.Compile(expression.String())
	if err != nil {
		return rule, err
	}
	rule.matcher = matcher
	return rule, nil
}

func (i *IgnoreList) IsIgnored(filePath string) bool {
	normalized := path.Clean(filepath.ToSlash(filePath))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return false
	}
	ignored := false
	for _, rule := range i.rules {
		if rule.matcher.MatchString(normalized) {
			ignored = !rule.negated
		}
	}
	return ignored
}

func (i *IgnoreList) Patterns() []string {
	return append([]string(nil), i.patterns...)
}

func LoadServerIgnore(serverRoot string) (*IgnoreList, error) {
	return LoadIgnoreFile(filepath.Join(serverRoot, ".pteroignore"))
}
