package rules

import (
	"strings"
	"testing"

	"github.com/Flgado/trino-insights-mcp/pkg/queryinfo"
)

// ----- pattern catalogue smoke test -----

// TestUnpushableCatalogue_AllRegexesCompile is a paranoia check: regexp.MustCompile
// already panics at package init if any pattern is malformed, but this test makes
// sure the catalogue is non-empty and every entry has the required fields wired up.
// If someone adds a new pattern but forgets the reason or remediation, this fails.
func TestUnpushableCatalogue_AllRegexesCompile(t *testing.T) {
	if len(unpushableCatalogue) == 0 {
		t.Fatal("catalogue is empty")
	}
	for _, p := range unpushableCatalogue {
		if p.id == "" {
			t.Errorf("pattern has empty id: %+v", p)
		}
		if p.re == nil {
			t.Errorf("pattern %q has nil regexp", p.id)
		}
		if p.reason == "" {
			t.Errorf("pattern %q has empty reason", p.id)
		}
		if p.remediation == "" {
			t.Errorf("pattern %q has empty remediation", p.id)
		}
	}
}

// ----- patternAppliesToConnector -----

func TestPatternAppliesToConnector(t *testing.T) {
	agnostic := &unpushablePattern{}
	mongoOnly := &unpushablePattern{applicableConnectors: []string{"mongo"}}
	jdbcOnly := &unpushablePattern{applicableConnectors: []string{"mysql", "postgres"}}

	cases := []struct {
		name      string
		pattern   *unpushablePattern
		connector string
		want      bool
	}{
		{"agnostic-empty-connector", agnostic, "", true},
		{"agnostic-any-connector", agnostic, "hive", true},
		{"mongo-matches-mongodb", mongoOnly, "mongodb", true},
		{"mongo-matches-MongoDB-casing", mongoOnly, "MongoDB", true},
		{"mongo-matches-mongo-cdc-variant", mongoOnly, "mongo-cdc", true},
		{"mongo-rejects-mysql", mongoOnly, "mysql", false},
		{"jdbc-matches-mysql", jdbcOnly, "mysql", true},
		{"jdbc-matches-postgresql", jdbcOnly, "postgresql", true},
		{"jdbc-rejects-mongo", jdbcOnly, "mongodb", false},
		{"connector-specific-empty-connector-rejects", mongoOnly, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := patternAppliesToConnector(tc.pattern, tc.connector)
			if got != tc.want {
				t.Errorf("patternAppliesToConnector(%v, %q) = %v, want %v",
					tc.pattern.applicableConnectors, tc.connector, got, tc.want)
			}
		})
	}
}

// ----- agnostic patterns (any connector) -----

// TestUnpushable_AgnosticPatterns drives one positive case per agnostic pattern
// against an arbitrary connector (hive here — none of these patterns are
// connector-scoped, so connector_type should be irrelevant). If we wire a new
// agnostic pattern, add a fixture row here.
func TestUnpushable_AgnosticPatterns(t *testing.T) {
	cases := []struct {
		name        string
		localFilter string
		wantPattern string
	}{
		{"lower-wrap", `(lower("name") = 'alice')`, "wrap.function"},
		{"upper-wrap", `(upper("status") = 'ACTIVE')`, "wrap.function"},
		{"trim-wrap", `(trim("code") = 'X')`, "wrap.function"},
		{"substring-wrap", `(substring("phone", 1, 3) = '555')`, "wrap.function"},
		{"cast-wrap", `(CAST("id" AS varchar) LIKE '42%')`, "wrap.cast"},
		{"arithmetic-on-column-plus", `("score" + 1 > 100)`, "wrap.arithmetic"},
		{"arithmetic-on-column-mul", `("amount" * 2 = 50)`, "wrap.arithmetic"},
		{"case-when", `(CASE WHEN "tier" = 'A' THEN 1 ELSE 0 END = 1)`, "wrap.case"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := baseFacts()
			f.ScanPushdown = []queryinfo.ScanPushdownFact{{
				StageID:                "q.4",
				Catalog:                "hive",
				Schema:                 "public",
				Table:                  "events",
				ConnectorType:          "hive",
				LocalFilter:            tc.localFilter,
				PhysicalInputPositions: 1000,
				OutputRows:             5,
			}}
			finding := UnpushableExpression{}.Eval(f)
			if finding == nil {
				t.Fatalf("expected finding for %q", tc.localFilter)
			}
			if !strings.Contains(asString(finding.Evidence), tc.wantPattern) {
				t.Errorf("evidence should mention pattern %q, got %s", tc.wantPattern, asString(finding.Evidence))
			}
		})
	}
}

// ----- MongoDB-specific patterns -----

func TestUnpushable_MongoCardinality_FiresOnMongo(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{{
		StageID:                "q.7",
		Catalog:                "app_documents",
		Schema:                 "platform",
		Table:                  "members",
		ConnectorType:          "mongodb",
		LocalFilter:            `(CARDINALITY("emails_to_email_emails") > 0)`,
		PhysicalInputPositions: 50000,
	}}
	finding := UnpushableExpression{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for CARDINALITY on mongodb")
	}
	if !strings.Contains(asString(finding.Evidence), "mongo.cardinality") {
		t.Errorf("evidence should mention mongo.cardinality, got %s", asString(finding.Evidence))
	}
	if !strings.Contains(finding.Remediation, "element_at") {
		t.Errorf("remediation should propose the element_at rewrite, got %q", finding.Remediation)
	}
}

func TestUnpushable_MongoCardinality_DoesNotFireOnJDBC(t *testing.T) {
	// CARDINALITY is unpushable on JDBC too, but we deliberately scope it
	// to Mongo in this PR because Postgres has native array operators that
	// CAN sometimes push (and we don't want to fire a remediation that
	// recommends `element_at` against a Postgres array column where the
	// real fix is something else entirely). This test pins that scoping.
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{{
		StageID:                "q.7",
		Catalog:                "warehouse",
		Schema:                 "public",
		Table:                  "tags",
		ConnectorType:          "postgresql",
		LocalFilter:            `(CARDINALITY("tags") > 0)`,
		PhysicalInputPositions: 50000,
	}}
	if finding := (UnpushableExpression{}).Eval(f); finding != nil {
		t.Errorf("CARDINALITY should not fire on postgresql in this PR; got %+v", finding)
	}
}

func TestUnpushable_MongoElementAt(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{{
		StageID:                "q.7",
		ConnectorType:          "mongodb",
		Catalog:                "mongo",
		Table:                  "users",
		LocalFilter:            `(element_at("aliases", 1) = 'admin')`,
		PhysicalInputPositions: 1000,
	}}
	finding := UnpushableExpression{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for element_at on mongodb")
	}
	if !strings.Contains(asString(finding.Evidence), "mongo.element-at") {
		t.Errorf("evidence should mention mongo.element-at")
	}
}

// ----- JDBC-specific patterns -----

func TestUnpushable_JDBCCoalesce_FiresOnMySQL(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{{
		StageID:                "q.10",
		Catalog:                "app_reporting",
		Schema:                 "public",
		Table:                  "user_membership",
		ConnectorType:          "mysql",
		LocalFilter:            `(COALESCE("is_payg", false) = true)`,
		PhysicalInputPositions: 12000,
	}}
	finding := UnpushableExpression{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for COALESCE on mysql")
	}
	if !strings.Contains(asString(finding.Evidence), "jdbc.coalesce") {
		t.Errorf("evidence should mention jdbc.coalesce, got %s", asString(finding.Evidence))
	}
}

// TestUnpushable_JDBCCoalesce_RealMySQLPlanText is a regression test pinned
// against the verbatim LocalFilter from a production query
// (20260526_183128_83136_2cdua, stage .1, members scan). The members table
// is a MySQL-backed Iceberg-adjacent table and its plan text uses BARE
// (unquoted) column names AND wraps a boolean expression inside the
// COALESCE call, both of which broke the original regex anchored on a
// quoted column. Keeping this fixture verbatim in the suite makes sure we
// never silently regress on real-world plan-text formatting.
func TestUnpushable_JDBCCoalesce_RealMySQLPlanText(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{{
		StageID:                "20260526_183128_83136_2cdua.1",
		Catalog:                "app_reporting",
		Schema:                 "app",
		Table:                  "members",
		ConnectorType:          "mysql",
		LocalFilter:            `(COALESCE((active = tinyint '1'), boolean 'false') = boolean 'true')`,
		PhysicalInputPositions: 48643,
	}}
	finding := UnpushableExpression{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding for boolean-expression COALESCE on mysql (real plan text)")
	}
	if !strings.Contains(asString(finding.Evidence), "jdbc.coalesce") {
		t.Errorf("evidence should mention jdbc.coalesce, got %s", asString(finding.Evidence))
	}
	// Remediation should mention the boolean-expression variant specifically,
	// because the COALESCE(expr = X, FALSE) = TRUE pattern needs a different
	// rewrite from the COALESCE(col, default) = X pattern.
	if !strings.Contains(finding.Remediation, "three-valued logic") {
		t.Errorf("remediation should cover the boolean-expression variant, got %q", finding.Remediation)
	}
}

func TestUnpushable_JDBCCoalesce_FiresOnPostgres(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{{
		StageID:                "q.10",
		ConnectorType:          "postgresql",
		Catalog:                "warehouse",
		Table:                  "users",
		LocalFilter:            `(COALESCE("active", false) = true)`,
		PhysicalInputPositions: 12000,
	}}
	if finding := (UnpushableExpression{}).Eval(f); finding == nil {
		t.Fatal("expected finding for COALESCE on postgresql")
	}
}

func TestUnpushable_JDBCCoalesce_DoesNotFireOnMongo(t *testing.T) {
	// COALESCE is JDBC-scoped in this PR. On Mongo the pushdown story is
	// different (the connector deals with NULLs differently), so we don't
	// want to recommend the two-arm OR rewrite there.
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{{
		StageID:                "q.10",
		ConnectorType:          "mongodb",
		Catalog:                "mongo",
		Table:                  "users",
		LocalFilter:            `(COALESCE("active", false) = true)`,
		PhysicalInputPositions: 12000,
	}}
	if finding := (UnpushableExpression{}).Eval(f); finding != nil {
		t.Errorf("COALESCE should not fire on mongo in this PR; got %+v", finding)
	}
}

// ----- shared JSON_EXTRACT (mongo + jdbc) -----

func TestUnpushable_JSONExtract_FiresOnAllSupportedConnectors(t *testing.T) {
	for _, connector := range []string{"mongodb", "mysql", "postgresql"} {
		t.Run(connector, func(t *testing.T) {
			f := baseFacts()
			f.ScanPushdown = []queryinfo.ScanPushdownFact{{
				StageID:                "q.5",
				ConnectorType:          connector,
				Catalog:                "src",
				Table:                  "events",
				LocalFilter:            `(JSON_EXTRACT("payload", '$.kind') = 'login')`,
				PhysicalInputPositions: 5000,
			}}
			finding := UnpushableExpression{}.Eval(f)
			if finding == nil {
				t.Fatalf("expected JSON_EXTRACT finding on %s", connector)
			}
			if !strings.Contains(asString(finding.Evidence), "json-extract") {
				t.Errorf("evidence should mention json-extract, got %s", asString(finding.Evidence))
			}
		})
	}
}

// ----- negative cases -----

func TestUnpushable_NoLocalFilter_DoesNotFire(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{{
		StageID:                "q.4",
		ConnectorType:          "mongodb",
		Table:                  "x",
		PhysicalInputPositions: 1000,
	}}
	if finding := (UnpushableExpression{}).Eval(f); finding != nil {
		t.Errorf("should not fire when LocalFilter is empty; got %+v", finding)
	}
}

func TestUnpushable_NoRecognisedPattern_DoesNotFire(t *testing.T) {
	// A plain equality predicate on a regular column. Nothing in the
	// catalogue should match.
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{{
		StageID:                "q.4",
		ConnectorType:          "mongodb",
		Table:                  "users",
		LocalFilter:            `("status" = 'ACTIVE')`,
		PhysicalInputPositions: 1000,
	}}
	if finding := (UnpushableExpression{}).Eval(f); finding != nil {
		t.Errorf("should not fire on plain equality; got %+v", finding)
	}
}

func TestUnpushable_NilFacts(t *testing.T) {
	if finding := (UnpushableExpression{}).Eval(nil); finding != nil {
		t.Errorf("should not fire on nil facts; got %+v", finding)
	}
}

func TestUnpushable_BelowMinRows_DoesNotFire(t *testing.T) {
	// With MinPhysicalRows configured the rule should suppress small scans.
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{{
		StageID:                "q.4",
		ConnectorType:          "mongodb",
		Table:                  "x",
		LocalFilter:            `(CARDINALITY("emails") > 0)`,
		PhysicalInputPositions: 5,
	}}
	if finding := (UnpushableExpression{MinPhysicalRows: 100}).Eval(f); finding != nil {
		t.Errorf("should not fire below MinPhysicalRows; got %+v", finding)
	}
}

// ----- multi-hit behaviour: sort, count, cap -----

func TestUnpushable_MultiplePatternsAndScans_SortAndCount(t *testing.T) {
	f := baseFacts()
	f.ScanPushdown = []queryinfo.ScanPushdownFact{
		{
			StageID:                "q.4",
			ConnectorType:          "mongodb",
			Table:                  "tiny",
			LocalFilter:            `(CARDINALITY("a") > 0)`,
			PhysicalInputPositions: 100,
		},
		{
			StageID:                "q.5",
			ConnectorType:          "mysql",
			Table:                  "big",
			LocalFilter:            `(COALESCE("flag", false) = true AND lower("name") = 'x')`,
			PhysicalInputPositions: 1_000_000,
		},
		{
			StageID:                "q.6",
			ConnectorType:          "mongodb",
			Table:                  "mid",
			LocalFilter:            `(element_at("arr", 1) = 'x')`,
			PhysicalInputPositions: 5000,
		},
	}
	finding := UnpushableExpression{}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding")
	}
	// The biggest scan (the mysql one) should anchor the title.
	if !strings.Contains(finding.Title, "big") || !strings.Contains(finding.Title, "mysql") {
		t.Errorf("title should lead with the biggest scan + its connector, got %q", finding.Title)
	}
	ev, ok := finding.Evidence.(map[string]any)
	if !ok {
		t.Fatalf("evidence wrong type: %T", finding.Evidence)
	}
	// Expect 4 hits: tiny:cardinality + big:coalesce + big:wrap.function + mid:element-at
	if hitsMatched := ev["hits_matched"].(int); hitsMatched != 4 {
		t.Errorf("hits_matched = %d, want 4", hitsMatched)
	}
	if distinct := ev["distinct_scans"].(int); distinct != 3 {
		t.Errorf("distinct_scans = %d, want 3", distinct)
	}
	if distinct := ev["distinct_patterns"].(int); distinct != 4 {
		t.Errorf("distinct_patterns = %d, want 4", distinct)
	}
}

func TestUnpushable_EvidenceCap(t *testing.T) {
	f := baseFacts()
	// Build many scans, all with the same pattern, to verify the evidence cap
	// kicks in but hits_matched still reflects the total.
	for i := 0; i < 15; i++ {
		f.ScanPushdown = append(f.ScanPushdown, queryinfo.ScanPushdownFact{
			StageID:                "q.x",
			ConnectorType:          "mongodb",
			Table:                  "t",
			LocalFilter:            `(CARDINALITY("a") > 0)`,
			PhysicalInputPositions: int64(1000 - i),
		})
	}
	finding := UnpushableExpression{MaxHitsInEvidence: 3}.Eval(f)
	if finding == nil {
		t.Fatal("expected finding")
	}
	ev := finding.Evidence.(map[string]any)
	hits := ev["hits"].([]map[string]any)
	if len(hits) != 3 {
		t.Errorf("evidence should be capped at 3, got %d", len(hits))
	}
	if total := ev["hits_matched"].(int); total != 15 {
		t.Errorf("hits_matched should reflect untapped total of 15, got %d", total)
	}
}

// ----- helpers -----

// asString stringifies an arbitrary value via fmt-like fallback so tests can
// substring-search across the whole evidence blob without unpacking it.
func asString(v any) string {
	return strings.Join(flattenStrings(v, nil), " ")
}

func flattenStrings(v any, acc []string) []string {
	switch x := v.(type) {
	case string:
		return append(acc, x)
	case []map[string]any:
		for _, m := range x {
			acc = flattenStrings(m, acc)
		}
		return acc
	case map[string]any:
		for _, val := range x {
			acc = flattenStrings(val, acc)
		}
		return acc
	default:
		return acc
	}
}
