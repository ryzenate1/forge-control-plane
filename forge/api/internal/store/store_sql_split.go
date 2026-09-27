package store

import "strings"

// splitSQLStatements splits a SQL string into individual statements,
// respecting dollar-quoted blocks ($$, $tag$), single-quoted literals, and
// stripping single-line comments.
func splitSQLStatements(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// First pass: strip single-line comments outside quoted text.
	cleaned := strings.TrimSpace(stripSQLComments(input))
	if cleaned == "" {
		return nil
	}

	// Second pass: split on semicolons outside quoted text. Dollar tags are
	// tracked on a stack so a nested body (for example $f$ inside DO $$) only
	// closes when its own tag is seen again.
	var statements []string
	runes := []rune(cleaned)
	pos := 0
	start := 0
	var openTags []string

	for pos < len(runes) {
		ch := runes[pos]

		if ch == '$' {
			if tag := scanDollarTag(runes, pos); tag != "" {
				if len(openTags) > 0 && openTags[len(openTags)-1] == tag {
					openTags = openTags[:len(openTags)-1]
				} else {
					openTags = append(openTags, tag)
				}
				pos += len(tag)
				continue
			}
		}

		// A semicolon inside a string literal does not end a statement.
		if ch == '\'' && len(openTags) == 0 {
			pos = skipSingleQuoted(runes, pos)
			continue
		}

		if ch == ';' && len(openTags) == 0 {
			candidate := string(runes[start:pos])
			if isTriggerStart(candidate) && !triggerTerminated(candidate) {
				// Inside a SQLite BEGIN ... END trigger body: the semicolon
				// separates body statements, it does not end the migration
				// statement. The terminator is the closing END; line.
				pos++
				continue
			}
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
// leaving dollar-quoted blocks and string literals untouched.
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

		// String literals may contain "--" that is not a comment.
		if ch == '\'' {
			end := skipSingleQuoted(runes, pos)
			result = append(result, runes[pos:end]...)
			pos = end
			continue
		}

		// Handle single-line comments outside quoted text
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

// skipSingleQuoted returns the index just past the single-quoted literal that
// starts at pos, treating ” as an escaped quote.
func skipSingleQuoted(runes []rune, pos int) int {
	pos++
	for pos < len(runes) {
		if runes[pos] == '\'' {
			if pos+1 < len(runes) && runes[pos+1] == '\'' {
				pos += 2
				continue
			}
			return pos + 1
		}
		pos++
	}
	return pos
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

// isTriggerStart reports whether a partial statement is the head of a
// CREATE TRIGGER definition. Only the SQLite form (plain BEGIN ... END body,
// no dollar quoting) reaches this check; PostgreSQL trigger functions are
// dollar-quoted and protected by the openTags tracking above.
func isTriggerStart(candidate string) bool {
	upper := strings.ToUpper(strings.TrimSpace(candidate))
	rest := strings.TrimPrefix(upper, "CREATE ")
	rest = strings.TrimPrefix(rest, "TEMP ")
	rest = strings.TrimPrefix(rest, "TEMPORARY ")
	return strings.HasPrefix(rest, "TRIGGER ")
}

// triggerTerminated reports whether a CREATE TRIGGER head already contains
// its closing END; line. The check requires BEGIN to be present as a whole
// word so a trigger name ending in "end" cannot false-positive. Limitation:
// an "\nEND;" sequence inside a single-quoted trigger literal would
// terminate early; migration triggers keep literals single-line to avoid it.
func triggerTerminated(candidate string) bool {
	upper := strings.ToUpper(candidate)
	hasBegin := false
	for _, field := range strings.Fields(upper) {
		if field == "BEGIN" {
			hasBegin = true
			break
		}
	}
	if !hasBegin {
		// No body yet (e.g. DROP TRIGGER, or the head before BEGIN): a bare
		// semicolon still ends the statement.
		return true
	}
	// The splitter hands us the candidate WITHOUT the semicolon under the
	// cursor, so the closing "END;" appears as a trailing whole-word END.
	// (An "\nEND;" already inside would mean we missed the terminator.)
	// Limitation: a trigger body statement that itself ends with the keyword
	// END (e.g. a CASE ... END; assignment inside the body) terminates early;
	// migration triggers avoid that shape.
	trimmed := strings.TrimRight(upper, " \t\r\n")
	if fields := strings.Fields(trimmed); len(fields) > 0 && fields[len(fields)-1] == "END" {
		return true
	}
	return strings.Contains(upper, "\nEND;") || strings.Contains(upper, "\nEND ;")
}
