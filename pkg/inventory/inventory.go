package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ToolsetID string

type ToolsetMetadata struct {
	ID               ToolsetID
	Description      string
	Default          bool
	InstructionsFunc func(inv *Inventory) string
}

type HandleFunc func(deps any) mcp.ToolHandler

type ServerTool struct {
	Tool        mcp.Tool
	ToolSet     ToolsetMetadata
	HandlerFunc HandleFunc
}

func (st ServerTool) IsReadOnly() bool {
	return st.Tool.Annotations != nil && st.Tool.Annotations.ReadOnlyHint
}

// --- Inventory (the built, filtered result) ---

type Inventory struct {
	tools           []ServerTool
	enabledToolsets map[ToolsetID]bool
	enabledTools    map[string]bool
	excludeTools    map[string]bool
	readOnly        bool
	instructions    string
}

func (i *Inventory) Tools() []ServerTool {
	return i.tools
}

func (i *Inventory) HasToolset(id ToolsetID) bool {
	return i.enabledToolsets[id]
}

func (i *Inventory) HasTool(name string) bool {
	return i.enabledTools[name]
}

func (i *Inventory) IsExcluded(name string) bool {
	return i.excludeTools[name]
}

func (i *Inventory) Instructions() string {
	return i.instructions
}

func (i *Inventory) RegisterAll(_ context.Context, server *mcp.Server, deps any) {
	for _, st := range i.tools {
		if st.HandlerFunc == nil {
			continue
		}
		toolCopy := st.Tool
		server.AddTool(&toolCopy, st.HandlerFunc(deps))
	}
}

// --- Builder ---

type Builder struct {
	allTools []ServerTool
	toolsets []string // nil = use defaults
	tools    []string // additive opt-ins
	exclude  []string
	readOnly bool
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) SetTools(tools []ServerTool) *Builder {
	b.allTools = tools
	return b
}

func (b *Builder) WithToolsets(ids []string) *Builder {
	b.toolsets = ids
	return b
}

func (b *Builder) WithTools(names []string) *Builder {
	b.tools = names
	return b
}

func (b *Builder) WithExclude(names []string) *Builder {
	b.exclude = names
	return b
}

func (b *Builder) WithReadOnly(ro bool) *Builder {
	b.readOnly = ro
	return b
}

func (b *Builder) Build() (*Inventory, error) {
	enabledToolsets := map[ToolsetID]bool{}

	if len(b.toolsets) == 0 {
		for _, st := range b.allTools {
			if st.ToolSet.Default {
				enabledToolsets[st.ToolSet.ID] = true
			}
		}
	} else {
		for _, id := range b.toolsets {
			if id == "all" {
				for _, st := range b.allTools {
					enabledToolsets[st.ToolSet.ID] = true
				}
				break
			}
			enabledToolsets[ToolsetID(id)] = true
		}
	}

	additionalTools := map[string]bool{}
	for _, name := range b.tools {
		additionalTools[name] = true
	}

	excludeTools := map[string]bool{}
	for _, name := range b.exclude {
		excludeTools[name] = true
	}

	var filtered []ServerTool
	enabledNames := map[string]bool{}

	for _, st := range b.allTools {
		name := st.Tool.Name
		if excludeTools[name] {
			continue
		}
		if b.readOnly && !st.IsReadOnly() {
			continue
		}
		if !enabledToolsets[st.ToolSet.ID] && !additionalTools[name] {
			continue
		}
		filtered = append(filtered, st)
		enabledNames[name] = true
	}

	seen := map[ToolsetID]bool{}
	var instrParts []string
	for _, st := range filtered {
		id := st.ToolSet.ID
		if seen[id] || st.ToolSet.InstructionsFunc == nil {
			continue
		}
		seen[id] = true
	}

	inv := &Inventory{
		tools:           filtered,
		enabledToolsets: enabledToolsets,
		enabledTools:    enabledNames,
		excludeTools:    excludeTools,
		readOnly:        b.readOnly,
	}

	for id := range seen {
		for _, st := range filtered {
			if st.ToolSet.ID == id && st.ToolSet.InstructionsFunc != nil {
				instrParts = append(instrParts, st.ToolSet.InstructionsFunc(inv))
				break
			}
		}
	}
	inv.instructions = strings.Join(instrParts, "\n")

	return inv, nil
}

// --- Generic tool constructors ---

func NewServerTool[In any](
	tool mcp.Tool,
	toolset ToolsetMetadata,
	handler func(ctx context.Context, req *mcp.CallToolRequest, args In) (*mcp.CallToolResult, error)) ServerTool {

	return ServerTool{
		Tool:    tool,
		ToolSet: toolset,
		HandlerFunc: func(_ any) mcp.ToolHandler {
			return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var args In
				if len(req.Params.Arguments) > 0 {
					if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
						return nil, fmt.Errorf("invalid arguments: %w", err)
					}
				}
				return handler(ctx, req, args)
			}
		},
	}
}

func NewServerToolWithDeps[In any, Deps any](
	tool mcp.Tool,
	toolset ToolsetMetadata,
	handler func(deps Deps) func(ctx context.Context, req *mcp.CallToolRequest, args In) (*mcp.CallToolResult, error)) ServerTool {
	return ServerTool{
		Tool:    tool,
		ToolSet: toolset,
		HandlerFunc: func(rawDeps any) mcp.ToolHandler {
			deps, _ := rawDeps.(Deps)
			typed := handler(deps)
			return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var args In
				if len(req.Params.Arguments) > 0 {
					if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
						return nil, fmt.Errorf("invalid arguments: %w", err)
					}
				}

				return typed(ctx, req, args)
			}
		},
	}
}
