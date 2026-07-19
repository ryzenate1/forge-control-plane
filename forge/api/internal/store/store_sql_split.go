package store

import "strings"

// splitSQLStatements splits a SQL string into individual statements,
// respecting dollar-quoted blocks ($$, $tag$) and stripping single-line comments.
func splitSQLStatements(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// First pass: strip single-line comments outside dollar quotes.
	cleaned := stripSQLComments(input)

	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return nil
	}

	// Second pass: split on semicolons outside dollar quotes.
	var statements []string
	runes := []rune(cleaned)
	pos := 0
	start := 0
	depth := 0

	for pos < len(runes) {
		ch := runes[pos]

		// Enter/exit dollar-quoted block
		if ch == '$' {
			if tag := scanDollarTag(runes, pos); tag != "" {
				if depth == 0 {
					depth++
				} else {
					// Check if this is the matching closing tag
					depth--
				}
				pos += len(tag)
				continue
			}
		}

		// Split on semicolon outside dollar quotes
		if ch == ';' && depth == 0 {
			stmt := strings.TrimSpace(string(runes[start:pos]))
			if stmt != "" {
				statements = append(statements, stmt)
			}
			pos++
			start = pos
			continue
		}

		pos++
	}

	// Remaining text after last semicolon
	remaining := strings.TrimSpace(string(runes[start:]))
	if remaining != "" {
		statements = append(statements, remaining)
	}

	if len(statements) == 0 {
		return nil
	}
	return statements
}

// stripSQLComments removes single-line comments (-- ... \n) from SQL text,
// respecting dollar-quoted blocks where comments should be preserved.
func stripSQLComments(input string) string {
	runes := []rune(input)
	var result []rune
	pos := 0

	for pos < len(runes) {
		ch := runes[pos]

		// Handle dollar-quoted blocks — pass through unchanged
		if ch == '$' {
			if tag := scanDollarTag(runes, pos); tag != "" {
				result = append(result, []rune(tag)...)
				pos += len(tag)
				// Scan for closing tag
				for pos < len(runes) {
					if t := scanDollarTag(runes, pos); t == tag {
						result = append(result, []rune(t)...)
						pos += len(tag)
						break
					}
					result = append(result, runes[pos])
					pos++
				}
				continue
			}
		}

		// Handle single-line comments outside dollar quotes
		if ch == '-' && pos+1 < len(runes) && runes[pos+1] == '-' {
			// Skip to end of line
			for pos < len(runes) && runes[pos] != '\n' {
				pos++
			}
			if pos < len(runes) {
				// Keep the newline (replaces comment with empty line)
				result = append(result, '\n')
				pos++
			}
			continue
		}

		result = append(result, ch)
		pos++
	}

	return string(result)
}

// scanDollarTag checks if there's a dollar-quote tag starting at pos.
// Returns the tag (including dollar signs) or empty string if not a valid tag.
func scanDollarTag(runes []rune, pos int) string {
	if pos >= len(runes) || runes[pos] != '$' {
		return ""
	}

	pos++
	tagStart := pos

	for pos < len(runes) && (isAlphaNumeric(runes[pos]) || runes[pos] == '_') {
		pos++
	}

	if pos >= len(runes) || runes[pos] != '$' {
		return ""
	}

	tag := string(runes[tagStart:pos])
	return "$" + tag + "$"
}

func isAlphaNumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
