package trino

import (
	"strings"
)

// SubstitutePreparedLiterals reconstructs the parameterised SQL template by
// substituting each `?` placeholder with the matching positional literal from
// the EXECUTE statement's USING clause.
//
// Given:
//
//	executeSQL    = "EXECUTE _trino_go USING 'abc', 42, true"
//	preparedQuery = "SELECT * FROM t WHERE id = ? AND n = ? AND active = ?"
//
// Returns:
//
//	"SELECT * FROM t WHERE id = 'abc' AND n = 42 AND active = true"
//
// Returns an empty string when:
//   - the EXECUTE statement has no USING clause,
//   - the USING clause cannot be parsed (unterminated string, etc.),
//   - the number of literals does not match the number of `?` placeholders.
//
// This is intentionally non-fatal: callers should treat "" as "could not
// reconstruct, agent should fall back to manual substitution from sql +
// prepared_query".
//
// The substitution is purely textual — placeholders inside string literals,
// quoted identifiers, line comments, and block comments are left alone.
func SubstitutePreparedLiterals(executeSQL, preparedQuery string) string {
	if executeSQL == "" || preparedQuery == "" {
		return ""
	}
	values, ok := parseUsingValues(executeSQL)
	if !ok || len(values) == 0 {
		return ""
	}
	out, count := substituteQuestionMarks(preparedQuery, values)
	if count != len(values) {
		// Mismatch — the prepared template's ? count must equal the USING
		// list length. Don't return a partially-substituted SQL.
		return ""
	}
	return out
}

// parseUsingValues locates the USING clause in an EXECUTE statement and
// returns the comma-separated literal tokens verbatim (single-quoted strings
// keep their quotes, numbers / booleans / NULL stay as written).
//
// Requires an EXECUTE prefix to guard against false-positive USING matches in
// queries that aren't prepared statements (e.g. SELECT statements that happen
// to use "using" as a column name or alias).
func parseUsingValues(sql string) ([]string, bool) {
	trimmed := strings.TrimSpace(sql)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "EXECUTE") {
		return nil, false
	}
	// The leading EXECUTE must be a whole word (e.g. not "EXECUTE_LOG").
	if len(trimmed) > len("EXECUTE") && !isSQLWhitespace(trimmed[len("EXECUTE")]) {
		return nil, false
	}
	idx := indexUsingKeyword(sql)
	if idx < 0 {
		return nil, false
	}
	return splitLiterals(sql[idx:])
}

// indexUsingKeyword finds the start of the literal list that follows the
// USING keyword, returning the byte offset of the first non-whitespace
// character after USING. Whitespace and the USING token itself are
// case-insensitive but must be a whole word (so we don't match identifiers
// like `using_status`).
func indexUsingKeyword(sql string) int {
	upper := strings.ToUpper(sql)
	const kw = "USING"
	start := 0
	for {
		k := strings.Index(upper[start:], kw)
		if k < 0 {
			return -1
		}
		absolute := start + k
		// Must be preceded by whitespace (or be at offset 0 — unusual but
		// harmless).
		if absolute > 0 && !isSQLWhitespace(sql[absolute-1]) {
			start = absolute + len(kw)
			continue
		}
		after := absolute + len(kw)
		// Must be followed by whitespace, end of string, or punctuation.
		if after < len(sql) && !isSQLWhitespace(sql[after]) && sql[after] != '(' {
			start = after
			continue
		}
		// Skip whitespace after the keyword to land on the first literal.
		i := after
		for i < len(sql) && isSQLWhitespace(sql[i]) {
			i++
		}
		return i
	}
}

func isSQLWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// splitLiterals walks a comma-separated literal list and returns each token
// verbatim. Handles single-quoted strings (with ” as escape) so that commas
// inside strings don't end a token.
//
// Returns (nil, false) when the input ends inside an unterminated string,
// which signals "could not parse USING list — give up cleanly."
func splitLiterals(s string) ([]string, bool) {
	var out []string
	var cur strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '\'':
			ni, ok := absorbQuotedLiteral(s, i, &cur)
			if !ok {
				return nil, false
			}
			i = ni
		case ',':
			out = flushToken(&cur, out)
			i++
		case ';':
			// Statement terminator — end of USING list.
			out = flushToken(&cur, out)
			return out, true
		default:
			cur.WriteByte(c)
			i++
		}
	}
	out = flushToken(&cur, out)
	return out, true
}

// absorbQuotedLiteral copies a single-quoted string starting at s[i] into cur,
// including ” escape sequences, and returns the index just past the closing
// quote. When the input ends without a closing quote it returns ok=false so
// the caller can bail out cleanly.
func absorbQuotedLiteral(s string, i int, cur *strings.Builder) (int, bool) {
	cur.WriteByte(s[i])
	i++
	for i < len(s) {
		c := s[i]
		cur.WriteByte(c)
		i++
		if c == '\'' {
			if i < len(s) && s[i] == '\'' {
				cur.WriteByte('\'')
				i++
				continue
			}
			return i, true
		}
	}
	return i, false
}

// flushToken trims the current builder, appends it to out if non-empty, and
// resets the builder. Returns the (possibly grown) slice.
func flushToken(cur *strings.Builder, out []string) []string {
	tok := strings.TrimSpace(cur.String())
	cur.Reset()
	if tok == "" {
		return out
	}
	return append(out, tok)
}

// substituteQuestionMarks walks the prepared query and replaces top-level `?`
// placeholders (those not inside a string literal, quoted identifier, or
// comment) with values[i] in order. Returns the rewritten string and the
// count of `?` placeholders encountered.
//
// If there are more placeholders than values, the surplus placeholders are
// left intact and the count keeps incrementing — the caller checks for the
// mismatch and discards the result.
func substituteQuestionMarks(sql string, values []string) (string, int) {
	var b strings.Builder
	b.Grow(len(sql) + 32*len(values))
	n := 0
	i := 0
	for i < len(sql) {
		switch sqlTokenKind(sql, i) {
		case tokenStringLiteral:
			i = copyStringLiteral(sql, i, &b)
		case tokenDoubleQuoted:
			i = copyDelimited(sql, i, '"', &b)
		case tokenBacktickQuoted:
			i = copyDelimited(sql, i, '`', &b)
		case tokenLineComment:
			i = copyLineComment(sql, i, &b)
		case tokenBlockComment:
			i = copyBlockComment(sql, i, &b)
		case tokenPlaceholder:
			if n < len(values) {
				b.WriteString(values[n])
			} else {
				b.WriteByte('?')
			}
			n++
			i++
		default:
			b.WriteByte(sql[i])
			i++
		}
	}
	return b.String(), n
}

// sqlTokenKind classifies the token starting at sql[i] so substituteQuestionMarks
// can dispatch to a single-purpose copier. Keeps the main loop short and the
// branching trivial.
type sqlTokenKindT int

const (
	tokenOther sqlTokenKindT = iota
	tokenStringLiteral
	tokenDoubleQuoted
	tokenBacktickQuoted
	tokenLineComment
	tokenBlockComment
	tokenPlaceholder
)

func sqlTokenKind(sql string, i int) sqlTokenKindT {
	c := sql[i]
	switch c {
	case '\'':
		return tokenStringLiteral
	case '"':
		return tokenDoubleQuoted
	case '`':
		return tokenBacktickQuoted
	case '?':
		return tokenPlaceholder
	}
	if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
		return tokenLineComment
	}
	if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
		return tokenBlockComment
	}
	return tokenOther
}

// copyStringLiteral writes a single-quoted SQL string literal verbatim,
// including ” escape sequences. Returns the index just past the closing
// quote (or len(sql) if the string is unterminated, which is recoverable —
// downstream code will just see no further placeholders).
func copyStringLiteral(sql string, i int, b *strings.Builder) int {
	b.WriteByte(sql[i])
	i++
	for i < len(sql) {
		c := sql[i]
		b.WriteByte(c)
		i++
		if c == '\'' {
			if i < len(sql) && sql[i] == '\'' {
				b.WriteByte('\'')
				i++
				continue
			}
			return i
		}
	}
	return i
}

// copyDelimited writes a delimited token verbatim until the matching closing
// delimiter. Used for "double-quoted identifiers" and `backtick identifiers`.
// No escape handling — the inner content is opaque text we just want to
// skip over.
func copyDelimited(sql string, i int, delim byte, b *strings.Builder) int {
	b.WriteByte(sql[i])
	i++
	for i < len(sql) {
		c := sql[i]
		b.WriteByte(c)
		i++
		if c == delim {
			return i
		}
	}
	return i
}

func copyLineComment(sql string, i int, b *strings.Builder) int {
	for i < len(sql) && sql[i] != '\n' {
		b.WriteByte(sql[i])
		i++
	}
	return i
}

func copyBlockComment(sql string, i int, b *strings.Builder) int {
	b.WriteByte('/')
	b.WriteByte('*')
	i += 2
	for i < len(sql) {
		if sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/' {
			b.WriteByte('*')
			b.WriteByte('/')
			return i + 2
		}
		b.WriteByte(sql[i])
		i++
	}
	return i
}
