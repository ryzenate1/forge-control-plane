package http

import (
	"fmt"
	"strconv"
	"strings"
)

func parsePortRanges(input string) ([]int, error) {
	parts := strings.FieldsFunc(input, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' })
	ports := []int{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			sides := strings.Split(part, "-")
			if len(sides) != 2 {
				return nil, fmt.Errorf("invalid port range %q", part)
			}
			start, err1 := strconv.Atoi(strings.TrimSpace(sides[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(sides[1]))
			if err1 != nil || err2 != nil || start <= 0 || end <= 0 || start > 65535 || end > 65535 || end < start {
				return nil, fmt.Errorf("invalid port range %q", part)
			}
			for p := start; p <= end; p++ {
				ports = append(ports, p)
				if len(ports) > 2000 {
					return nil, fmt.Errorf("too many ports in one request")
				}
			}
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil || value <= 0 || value > 65535 {
			return nil, fmt.Errorf("invalid port %q", part)
		}
		ports = append(ports, value)
		if len(ports) > 2000 {
			return nil, fmt.Errorf("too many ports in one request")
		}
	}
	return ports, nil
}

func uniquePorts(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 || value > 65535 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
