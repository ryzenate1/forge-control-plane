package system

import (
	"regexp"
	"strings"
)

var stripAnsiRegex = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")

type LineMatcher struct {
	patterns []linePattern
}

type linePattern struct {
	raw  string
	comp *regexp.Regexp
}

func NewLineMatcher(patterns []string) *LineMatcher {
	lm := &LineMatcher{}
	for _, p := range patterns {
		pat := linePattern{raw: p}
		if strings.HasPrefix(p, "regex:") && len(p) > 6 {
			if r, err := regexp.Compile(strings.TrimPrefix(p, "regex:")); err == nil {
				pat.comp = r
			}
		}
		lm.patterns = append(lm.patterns, pat)
	}
	return lm
}

func (lm *LineMatcher) Match(line string) bool {
	for _, p := range lm.patterns {
		if p.comp != nil {
			if p.comp.MatchString(line) {
				return true
			}
		} else if strings.Contains(line, p.raw) {
			return true
		}
	}
	return false
}

func (lm *LineMatcher) Reset() {}

func StripANSI(line string) string {
	return stripAnsiRegex.ReplaceAllString(line, "")
}
