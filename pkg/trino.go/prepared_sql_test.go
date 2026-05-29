package trino

import (
	"strings"
	"testing"
)

func TestSubstitutePreparedLiterals_SimpleStringsAndNumbers(t *testing.T) {
	execSQL := "EXECUTE _trino_go USING 'abc', 42, true"
	template := "SELECT * FROM t WHERE id = ? AND n = ? AND active = ?"
	want := "SELECT * FROM t WHERE id = 'abc' AND n = 42 AND active = true"

	got := SubstitutePreparedLiterals(execSQL, template)
	if got != want {
		t.Errorf("got:\n  %q\nwant:\n  %q", got, want)
	}
}

func TestSubstitutePreparedLiterals_RealisticQuery(t *testing.T) {
	// Lifted from analysis-20260521_183828_00550_httz8 (real Trino payload).
	execSQL := "EXECUTE _trino_go USING '6731edb157a396a818057c1b', '6731edb157a396a818057c1b', true, 1780005599"
	template := `WITH members AS (SELECT _id FROM app_reporting."app".members WHERE origin_branch_id = ?) SELECT COUNT(*) FROM tasks WHERE location_id = ? AND active = ? AND due_date > ?`

	got := SubstitutePreparedLiterals(execSQL, template)
	if !strings.Contains(got, "origin_branch_id = '6731edb157a396a818057c1b'") {
		t.Errorf("missing first substitution in: %q", got)
	}
	if !strings.Contains(got, "location_id = '6731edb157a396a818057c1b'") {
		t.Errorf("missing second substitution in: %q", got)
	}
	if !strings.Contains(got, "active = true") {
		t.Errorf("missing third substitution in: %q", got)
	}
	if !strings.Contains(got, "due_date > 1780005599") {
		t.Errorf("missing fourth substitution in: %q", got)
	}
}

func TestSubstitutePreparedLiterals_PreservesStringInQuery(t *testing.T) {
	// A ? inside a string literal in the template should NOT be substituted.
	execSQL := "EXECUTE _trino_go USING 42"
	template := "SELECT '? this is a literal' AS lbl, ? AS n FROM dual"
	want := "SELECT '? this is a literal' AS lbl, 42 AS n FROM dual"

	got := SubstitutePreparedLiterals(execSQL, template)
	if got != want {
		t.Errorf("got:\n  %q\nwant:\n  %q", got, want)
	}
}

func TestSubstitutePreparedLiterals_HandlesQuotedIdentifier(t *testing.T) {
	// A ? inside a double-quoted identifier should not be substituted.
	execSQL := "EXECUTE _trino_go USING 1"
	template := `SELECT "weird?name" FROM t WHERE n = ?`
	want := `SELECT "weird?name" FROM t WHERE n = 1`

	got := SubstitutePreparedLiterals(execSQL, template)
	if got != want {
		t.Errorf("got:\n  %q\nwant:\n  %q", got, want)
	}
}

func TestSubstitutePreparedLiterals_LineAndBlockComments(t *testing.T) {
	execSQL := "EXECUTE _trino_go USING 7"
	template := `SELECT n -- ? in a line comment
FROM /* ? in a block comment */ t WHERE n = ?`
	got := SubstitutePreparedLiterals(execSQL, template)
	if strings.Count(got, "?") != 2 {
		t.Errorf("expected 2 remaining ? (inside comments), got: %q", got)
	}
	if !strings.HasSuffix(got, "n = 7") {
		t.Errorf("trailing substitution failed: %q", got)
	}
}

func TestSubstitutePreparedLiterals_EscapedQuoteInString(t *testing.T) {
	// SQL escapes a single quote inside a string by doubling it.
	execSQL := `EXECUTE _trino_go USING 'O''Brien', 1`
	template := `SELECT * FROM t WHERE name = ? AND id = ?`
	got := SubstitutePreparedLiterals(execSQL, template)
	if !strings.Contains(got, `name = 'O''Brien'`) {
		t.Errorf("escaped quote lost: %q", got)
	}
	if !strings.HasSuffix(got, "id = 1") {
		t.Errorf("trailing substitution failed: %q", got)
	}
}

func TestSubstitutePreparedLiterals_ReturnsEmptyOnCountMismatch(t *testing.T) {
	execSQL := "EXECUTE _trino_go USING 'abc', 42"
	template := "SELECT ? FROM t WHERE n = ? AND extra = ?"
	got := SubstitutePreparedLiterals(execSQL, template)
	if got != "" {
		t.Errorf("expected empty on count mismatch, got: %q", got)
	}
}

func TestSubstitutePreparedLiterals_ReturnsEmptyWhenNoUsing(t *testing.T) {
	got := SubstitutePreparedLiterals("EXECUTE my_stmt", "SELECT ? FROM t")
	if got != "" {
		t.Errorf("expected empty when EXECUTE has no USING, got: %q", got)
	}
}

func TestSubstitutePreparedLiterals_ReturnsEmptyOnUnterminatedString(t *testing.T) {
	got := SubstitutePreparedLiterals("EXECUTE _trino_go USING 'unterminated", "SELECT ?")
	if got != "" {
		t.Errorf("expected empty on unterminated USING string, got: %q", got)
	}
}

func TestSubstitutePreparedLiterals_EmptyInputsReturnEmpty(t *testing.T) {
	if got := SubstitutePreparedLiterals("", "SELECT ?"); got != "" {
		t.Errorf("empty executeSQL should return empty, got %q", got)
	}
	if got := SubstitutePreparedLiterals("EXECUTE x USING 1", ""); got != "" {
		t.Errorf("empty preparedQuery should return empty, got %q", got)
	}
}

func TestSplitLiterals_StringWithCommaInside(t *testing.T) {
	// A comma inside a string literal must NOT end the token.
	values, ok := splitLiterals("'a, b, c', 42")
	if !ok {
		t.Fatal("expected ok")
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(values), values)
	}
	if values[0] != "'a, b, c'" {
		t.Errorf("first token = %q, want %q", values[0], "'a, b, c'")
	}
	if values[1] != "42" {
		t.Errorf("second token = %q, want %q", values[1], "42")
	}
}

func TestSplitLiterals_HandlesBooleansAndNulls(t *testing.T) {
	values, ok := splitLiterals("true, false, NULL, null, -42, 3.14")
	if !ok {
		t.Fatal("expected ok")
	}
	want := []string{"true", "false", "NULL", "null", "-42", "3.14"}
	if len(values) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(values), len(want), values)
	}
	for i := range want {
		if values[i] != want[i] {
			t.Errorf("token[%d] = %q, want %q", i, values[i], want[i])
		}
	}
}

func TestIndexUsingKeyword_WholeWordMatch(t *testing.T) {
	// Lower-level helper: only checks "USING" is a whole word inside the
	// string. Rejecting non-EXECUTE statements is parseUsingValues's job
	// (see TestParseUsingValues_RejectsNonExecute).
	cases := []struct {
		sql    string
		wantOk bool
	}{
		{"EXECUTE x USING 'a'", true},
		{"EXECUTE x using 'a'", true},
		{"SELECT using_status FROM t", false}, // USING is part of an identifier — no match
	}
	for _, c := range cases {
		idx := indexUsingKeyword(c.sql)
		hasIt := idx >= 0
		if hasIt != c.wantOk {
			t.Errorf("indexUsingKeyword(%q) = %d, wantOk=%v", c.sql, idx, c.wantOk)
		}
	}
}

func TestParseUsingValues_RejectsNonExecute(t *testing.T) {
	// Even if the statement contains a whole-word "USING" (e.g. a future
	// CREATE … USING DDL form, or someone smuggling "using" as an alias),
	// parseUsingValues must refuse to parse it unless the statement begins
	// with EXECUTE. This guards SubstitutePreparedLiterals from clobbering
	// non-prepared SQL.
	cases := []string{
		"SELECT * FROM t WHERE using = 1",
		"CREATE TABLE x USING parquet AS SELECT 1",
		"  SELECT 1  ",
	}
	for _, sql := range cases {
		if _, ok := parseUsingValues(sql); ok {
			t.Errorf("parseUsingValues(%q) accepted non-EXECUTE input", sql)
		}
	}
}

func TestParseUsingValues_AcceptsLeadingWhitespace(t *testing.T) {
	// Real Trino payloads sometimes have leading whitespace.
	values, ok := parseUsingValues("  \n  EXECUTE _trino_go USING 'a', 1")
	if !ok {
		t.Fatal("expected ok")
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d: %v", len(values), values)
	}
}
