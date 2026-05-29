package inventory

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func toolMeta(id string, def bool) ToolsetMetadata {
	return ToolsetMetadata{
		ID:      ToolsetID(id),
		Default: def,
	}
}

func toolMetaWithInstructions(id string, def bool, instr string) ToolsetMetadata {
	return ToolsetMetadata{
		ID:      ToolsetID(id),
		Default: def,
		InstructionsFunc: func(_ *Inventory) string {
			return instr
		},
	}
}

func mkTool(name string, ts ToolsetMetadata, readOnly bool) ServerTool {
	var annotations *mcp.ToolAnnotations
	if readOnly {
		annotations = &mcp.ToolAnnotations{ReadOnlyHint: true}
	}
	return ServerTool{
		Tool: mcp.Tool{
			Name:        name,
			Annotations: annotations,
		},
		ToolSet: ts,
		HandlerFunc: func(_ any) mcp.ToolHandler {
			return func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{}, nil
			}
		},
	}
}

func TestServerTool_IsReadOnly(t *testing.T) {
	ro := mkTool("read", toolMeta("ts", true), true)
	rw := mkTool("write", toolMeta("ts", true), false)

	if !ro.IsReadOnly() {
		t.Error("expected IsReadOnly() = true for read-only tool")
	}
	if rw.IsReadOnly() {
		t.Error("expected IsReadOnly() = false for read-write tool")
	}
}

func TestServerTool_IsReadOnly_NilAnnotations(t *testing.T) {
	st := ServerTool{Tool: mcp.Tool{Name: "test"}}
	if st.IsReadOnly() {
		t.Error("nil annotations should mean not read-only")
	}
}

func TestBuilder_DefaultToolsets(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMeta("default-ts", true), false),
		mkTool("b", toolMeta("optional-ts", false), false),
	}

	inv, err := NewBuilder().SetTools(tools).Build()
	if err != nil {
		t.Fatal(err)
	}

	if len(inv.Tools()) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(inv.Tools()))
	}
	if inv.Tools()[0].Tool.Name != "a" {
		t.Errorf("expected tool 'a', got %q", inv.Tools()[0].Tool.Name)
	}
}

func TestBuilder_ExplicitToolsets(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMeta("ts1", true), false),
		mkTool("b", toolMeta("ts2", false), false),
	}

	inv, err := NewBuilder().SetTools(tools).WithToolsets([]string{"ts2"}).Build()
	if err != nil {
		t.Fatal(err)
	}

	if len(inv.Tools()) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(inv.Tools()))
	}
	if inv.Tools()[0].Tool.Name != "b" {
		t.Errorf("expected tool 'b', got %q", inv.Tools()[0].Tool.Name)
	}
}

func TestBuilder_ToolsetAll(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMeta("ts1", false), false),
		mkTool("b", toolMeta("ts2", false), false),
		mkTool("c", toolMeta("ts3", false), false),
	}

	inv, err := NewBuilder().SetTools(tools).WithToolsets([]string{"all"}).Build()
	if err != nil {
		t.Fatal(err)
	}

	if len(inv.Tools()) != 3 {
		t.Fatalf("expected 3 tools with 'all', got %d", len(inv.Tools()))
	}
}

func TestBuilder_AdditiveTools(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMeta("ts1", true), false),
		mkTool("b", toolMeta("ts2", false), false),
	}

	inv, err := NewBuilder().SetTools(tools).WithTools([]string{"b"}).Build()
	if err != nil {
		t.Fatal(err)
	}

	if len(inv.Tools()) != 2 {
		t.Fatalf("expected 2 tools (default + additive), got %d", len(inv.Tools()))
	}
}

func TestBuilder_ExcludeTools(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMeta("ts1", true), false),
		mkTool("b", toolMeta("ts1", true), false),
	}

	inv, err := NewBuilder().SetTools(tools).WithExclude([]string{"b"}).Build()
	if err != nil {
		t.Fatal(err)
	}

	if len(inv.Tools()) != 1 {
		t.Fatalf("expected 1 tool after exclude, got %d", len(inv.Tools()))
	}
	if inv.Tools()[0].Tool.Name != "a" {
		t.Errorf("expected tool 'a', got %q", inv.Tools()[0].Tool.Name)
	}
}

func TestBuilder_ReadOnlyFilter(t *testing.T) {
	tools := []ServerTool{
		mkTool("reader", toolMeta("ts1", true), true),
		mkTool("writer", toolMeta("ts1", true), false),
	}

	inv, err := NewBuilder().SetTools(tools).WithReadOnly(true).Build()
	if err != nil {
		t.Fatal(err)
	}

	if len(inv.Tools()) != 1 {
		t.Fatalf("expected 1 read-only tool, got %d", len(inv.Tools()))
	}
	if inv.Tools()[0].Tool.Name != "reader" {
		t.Errorf("expected tool 'reader', got %q", inv.Tools()[0].Tool.Name)
	}
}

func TestBuilder_ExcludeTakesPrecedenceOverAdditive(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMeta("ts1", false), false),
	}

	inv, err := NewBuilder().
		SetTools(tools).
		WithTools([]string{"a"}).
		WithExclude([]string{"a"}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	if len(inv.Tools()) != 0 {
		t.Fatalf("exclude should override additive, got %d tools", len(inv.Tools()))
	}
}

func TestBuilder_EmptyInput(t *testing.T) {
	inv, err := NewBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Tools()) != 0 {
		t.Errorf("expected 0 tools, got %d", len(inv.Tools()))
	}
}

func TestInventory_HasToolset(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMeta("plans", true), false),
	}
	inv, _ := NewBuilder().SetTools(tools).Build()

	if !inv.HasToolset("plans") {
		t.Error("expected HasToolset('plans') = true")
	}
	if inv.HasToolset("nonexistent") {
		t.Error("expected HasToolset('nonexistent') = false")
	}
}

func TestInventory_HasTool(t *testing.T) {
	tools := []ServerTool{
		mkTool("analyze_query", toolMeta("plans", true), false),
	}
	inv, _ := NewBuilder().SetTools(tools).Build()

	if !inv.HasTool("analyze_query") {
		t.Error("expected HasTool('analyze_query') = true")
	}
	if inv.HasTool("missing") {
		t.Error("expected HasTool('missing') = false")
	}
}

func TestInventory_IsExcluded(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMeta("ts1", true), false),
		mkTool("b", toolMeta("ts1", true), false),
	}
	inv, _ := NewBuilder().SetTools(tools).WithExclude([]string{"b"}).Build()

	if !inv.IsExcluded("b") {
		t.Error("expected IsExcluded('b') = true")
	}
	if inv.IsExcluded("a") {
		t.Error("expected IsExcluded('a') = false")
	}
}

func TestInventory_Instructions(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMetaWithInstructions("plans", true, "Use analyze_query first"), false),
	}
	inv, _ := NewBuilder().SetTools(tools).Build()

	if inv.Instructions() != "Use analyze_query first" {
		t.Errorf("Instructions() = %q, want %q", inv.Instructions(), "Use analyze_query first")
	}
}

func TestInventory_Instructions_MultipleToolsets(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMetaWithInstructions("ts1", true, "instructions for ts1"), false),
		mkTool("b", toolMetaWithInstructions("ts2", true, "instructions for ts2"), false),
	}
	inv, _ := NewBuilder().SetTools(tools).Build()

	instr := inv.Instructions()
	if instr == "" {
		t.Error("expected non-empty instructions")
	}
}

func TestInventory_Instructions_NoFunc(t *testing.T) {
	tools := []ServerTool{
		mkTool("a", toolMeta("ts1", true), false),
	}
	inv, _ := NewBuilder().SetTools(tools).Build()

	if inv.Instructions() != "" {
		t.Errorf("expected empty instructions, got %q", inv.Instructions())
	}
}

func TestInventory_RegisterAll_SkipsNilHandler(t *testing.T) {
	st := ServerTool{
		Tool:    mcp.Tool{Name: "nilhandler"},
		ToolSet: toolMeta("ts1", true),
	}
	inv := &Inventory{
		tools:           []ServerTool{st},
		enabledToolsets: map[ToolsetID]bool{"ts1": true},
		enabledTools:    map[string]bool{"nilhandler": true},
		excludeTools:    map[string]bool{},
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test",
		Version: "1.0",
	}, nil)
	inv.RegisterAll(context.Background(), server, nil)
}

func TestNewServerTool(t *testing.T) {
	type args struct {
		QueryID string `json:"query_id"`
	}

	tool := mcp.Tool{Name: "test_tool"}
	ts := toolMeta("ts1", true)

	st := NewServerTool(tool, ts, func(ctx context.Context, req *mcp.CallToolRequest, a args) (*mcp.CallToolResult, error) {
		if a.QueryID != "abc123" {
			t.Errorf("QueryID = %q, want %q", a.QueryID, "abc123")
		}
		return &mcp.CallToolResult{}, nil
	})

	if st.Tool.Name != "test_tool" {
		t.Errorf("Tool.Name = %q", st.Tool.Name)
	}

	handler := st.HandlerFunc(nil)
	argsJSON, _ := json.Marshal(args{QueryID: "abc123"})
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test_tool",
			Arguments: argsJSON,
		},
	}
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
}

func TestNewServerTool_InvalidArgs(t *testing.T) {
	type args struct {
		Count int `json:"count"`
	}

	st := NewServerTool(mcp.Tool{Name: "test"}, toolMeta("ts", true),
		func(_ context.Context, _ *mcp.CallToolRequest, a args) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})

	handler := st.HandlerFunc(nil)
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "test",
			Arguments: json.RawMessage(`{"count": "not a number"}`),
		},
	}
	_, err := handler(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid args")
	}
}

func TestNewServerTool_EmptyArgs(t *testing.T) {
	type args struct{}

	called := false
	st := NewServerTool(mcp.Tool{Name: "test"}, toolMeta("ts", true),
		func(_ context.Context, _ *mcp.CallToolRequest, _ args) (*mcp.CallToolResult, error) {
			called = true
			return &mcp.CallToolResult{}, nil
		})

	handler := st.HandlerFunc(nil)
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "test",
		},
	}
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

func TestNewServerToolWithDeps(t *testing.T) {
	type args struct {
		ID string `json:"id"`
	}
	type deps struct {
		Prefix string
	}

	st := NewServerToolWithDeps[args, *deps](
		mcp.Tool{Name: "dep_tool"},
		toolMeta("ts", true),
		func(d *deps) func(context.Context, *mcp.CallToolRequest, args) (*mcp.CallToolResult, error) {
			return func(_ context.Context, _ *mcp.CallToolRequest, a args) (*mcp.CallToolResult, error) {
				if d.Prefix != "pfx" {
					t.Errorf("Prefix = %q, want %q", d.Prefix, "pfx")
				}
				if a.ID != "xyz" {
					t.Errorf("ID = %q, want %q", a.ID, "xyz")
				}
				return &mcp.CallToolResult{}, nil
			}
		},
	)

	handler := st.HandlerFunc(&deps{Prefix: "pfx"})
	argsJSON, _ := json.Marshal(args{ID: "xyz"})
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "dep_tool",
			Arguments: argsJSON,
		},
	}
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
}

func TestNewBuilder_Chaining(t *testing.T) {
	b := NewBuilder().
		SetTools(nil).
		WithToolsets([]string{"ts1"}).
		WithTools([]string{"t1"}).
		WithExclude([]string{"t2"}).
		WithReadOnly(true)

	if b == nil {
		t.Fatal("builder should not be nil")
	}
}
