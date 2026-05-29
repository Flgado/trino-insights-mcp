package translations

import (
	"os"
	"testing"
)

func TestHelper_ReturnsFallback(t *testing.T) {
	helper, _ := NewHelper()
	got := helper("MY_TOOL", "default description")
	if got != "default description" {
		t.Errorf("got %q, want %q", got, "default description")
	}
}

func TestHelper_ReturnsEnvOverride(t *testing.T) {
	t.Setenv("TRINO_INSIGHTS_DESC_MY_TOOL", "custom desc")

	helper, _ := NewHelper()
	got := helper("MY_TOOL", "default description")
	if got != "custom desc" {
		t.Errorf("got %q, want %q", got, "custom desc")
	}
}

func TestHelper_EnvOverrideTakesPrecedence(t *testing.T) {
	t.Setenv("TRINO_INSIGHTS_DESC_FOO", "overridden")

	helper, _ := NewHelper()

	got := helper("FOO", "original")
	if got != "overridden" {
		t.Errorf("got %q, want %q", got, "overridden")
	}

	got2 := helper("BAR", "bar default")
	if got2 != "bar default" {
		t.Errorf("got %q, want %q", got2, "bar default")
	}
}

func TestHelper_MultipleKeys(t *testing.T) {
	t.Setenv("TRINO_INSIGHTS_DESC_A", "override A")

	helper, _ := NewHelper()

	if got := helper("A", "fallback A"); got != "override A" {
		t.Errorf("A: got %q, want %q", got, "override A")
	}
	if got := helper("B", "fallback B"); got != "fallback B" {
		t.Errorf("B: got %q, want %q", got, "fallback B")
	}
}

func TestHelper_EmptyOverrideValue(t *testing.T) {
	t.Setenv("TRINO_INSIGHTS_DESC_EMPTY", "")

	helper, _ := NewHelper()
	got := helper("EMPTY", "fallback")
	if got != "" {
		t.Errorf("got %q, want empty string (env override is empty)", got)
	}
}

func TestHelper_DumpWritesFile(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	helper, dump := NewHelper()
	helper("KEY1", "val1")
	helper("KEY2", "val2")
	dump()

	data, err := os.ReadFile("translations.json")
	if err != nil {
		t.Fatalf("expected translations.json to be written: %v", err)
	}

	content := string(data)
	if !contains(content, "KEY1") || !contains(content, "val1") {
		t.Errorf("translations.json missing KEY1/val1: %s", content)
	}
	if !contains(content, "KEY2") || !contains(content, "val2") {
		t.Errorf("translations.json missing KEY2/val2: %s", content)
	}
}

func TestHelper_DumpIncludesOverrides(t *testing.T) {
	t.Setenv("TRINO_INSIGHTS_DESC_OVER", "override value")

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	helper, dump := NewHelper()
	helper("OVER", "fallback")
	dump()

	data, err := os.ReadFile("translations.json")
	if err != nil {
		t.Fatal(err)
	}

	if !contains(string(data), "override value") {
		t.Errorf("expected override value in dump, got: %s", string(data))
	}
}

func TestLoadEnvOverrides_NoMatchingVars(t *testing.T) {
	m := loadEnvOvrrides()
	for k := range m {
		if k == "" {
			t.Error("should not have empty key")
		}
	}
}

func TestLoadEnvOverrides_WithMatchingVars(t *testing.T) {
	t.Setenv("TRINO_INSIGHTS_DESC_TOOL_A", "desc A")
	t.Setenv("TRINO_INSIGHTS_DESC_TOOL_B", "desc B")

	m := loadEnvOvrrides()
	if m["TOOL_A"] != "desc A" {
		t.Errorf("TOOL_A = %q, want %q", m["TOOL_A"], "desc A")
	}
	if m["TOOL_B"] != "desc B" {
		t.Errorf("TOOL_B = %q, want %q", m["TOOL_B"], "desc B")
	}
}

func TestLoadEnvOverrides_IgnoresUnrelatedVars(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("TRINO_INSIGHTS_DESC_X", "val")

	m := loadEnvOvrrides()
	if _, ok := m["HOME"]; ok {
		t.Error("should not include unrelated env vars")
	}
	if m["X"] != "val" {
		t.Errorf("X = %q, want %q", m["X"], "val")
	}
}

func TestLoadEnvOverrides_MalformedEntry(t *testing.T) {
	t.Setenv("TRINO_INSIGHTS_DESC_", "no key")

	m := loadEnvOvrrides()
	if _, ok := m[""]; ok {
		t.Error("should not include entry with empty key after prefix")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
