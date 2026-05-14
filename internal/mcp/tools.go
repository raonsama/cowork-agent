package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolKind identifies the MCP tool category.
type ToolKind string

const (
	ToolReadFile   ToolKind = "read_file"
	ToolWriteFile  ToolKind = "write_file"
	ToolListDir    ToolKind = "list_dir"
	ToolRunShell   ToolKind = "run_shell"
	ToolSearchCode ToolKind = "search_code"
	ToolApplyPatch ToolKind = "apply_patch"
	ToolCreateDir  ToolKind = "create_dir"
	ToolDeleteFile ToolKind = "delete_file"
	ToolGitStatus  ToolKind = "git_status"
	ToolGitDiff    ToolKind = "git_diff"
)

// ToolDef describes an MCP tool to the LLM.
type ToolDef struct {
	Name        ToolKind `json:"name"`
	Description string   `json:"description"`
	Parameters  Schema   `json:"parameters"`
}

// Schema is a simplified JSON Schema for tool parameters.
type Schema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property is a single parameter property.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolCall represents the LLM's request to execute a tool.
type ToolCall struct {
	Name   ToolKind        `json:"name"`
	Params json.RawMessage `json:"parameters"`
}

// ToolResult is the result returned from a tool execution.
type ToolResult struct {
	Tool    ToolKind `json:"tool"`
	Success bool     `json:"success"`
	Output  string   `json:"output"`
	Error   string   `json:"error,omitempty"`
}

// ToolHandler is a function that executes a tool call.
type ToolHandler func(ctx context.Context, params json.RawMessage) ToolResult

// Server manages available tools and dispatches calls.
type Server struct {
	tools       map[ToolKind]ToolHandler
	defs        []ToolDef
	projectRoot string
}

// NewServer creates an MCP server rooted at projectRoot.
func NewServer(projectRoot string) *Server {
	s := &Server{
		tools:       make(map[ToolKind]ToolHandler),
		projectRoot: projectRoot,
	}
	s.registerDefaults()
	return s
}

// registerDefaults wires up all built-in tools.
func (s *Server) registerDefaults() {
	fs := newFSTools(s.projectRoot)
	sh := newShellTools(s.projectRoot)

	s.Register(ToolReadFile, "Read the full content of a file at the given path.",
		Schema{
			Type: "object",
			Properties: map[string]Property{
				"path": {Type: "string", Description: "Relative or absolute path to the file"},
			},
			Required: []string{"path"},
		}, fs.ReadFile)

	s.Register(ToolWriteFile, "Write content to a file, creating it if it does not exist.",
		Schema{
			Type: "object",
			Properties: map[string]Property{
				"path":    {Type: "string", Description: "Target file path"},
				"content": {Type: "string", Description: "Content to write"},
			},
			Required: []string{"path", "content"},
		}, fs.WriteFile)

	s.Register(ToolListDir, "List files and directories at a path.",
		Schema{
			Type: "object",
			Properties: map[string]Property{
				"path": {Type: "string", Description: "Directory path (default: project root)"},
			},
		}, fs.ListDir)

	s.Register(ToolCreateDir, "Create a directory (including parents).",
		Schema{
			Type: "object",
			Properties: map[string]Property{
				"path": {Type: "string", Description: "Directory path to create"},
			},
			Required: []string{"path"},
		}, fs.CreateDir)

	s.Register(ToolDeleteFile, "Delete a file or empty directory.",
		Schema{
			Type: "object",
			Properties: map[string]Property{
				"path": {Type: "string", Description: "Path to delete"},
			},
			Required: []string{"path"},
		}, fs.DeleteFile)

	s.Register(ToolApplyPatch, "Apply a unified diff patch to the project.",
		Schema{
			Type: "object",
			Properties: map[string]Property{
				"patch": {Type: "string", Description: "Unified diff content"},
			},
			Required: []string{"patch"},
		}, fs.ApplyPatch)

	s.Register(ToolRunShell, "Execute a shell command in the project root. Use with caution.",
		Schema{
			Type: "object",
			Properties: map[string]Property{
				"command":     {Type: "string", Description: "Shell command to execute"},
				"timeout_sec": {Type: "integer", Description: "Timeout in seconds (default: 30)"},
			},
			Required: []string{"command"},
		}, sh.RunShell)

	s.Register(ToolGitStatus, "Get the current git status of the project.",
		Schema{Type: "object", Properties: map[string]Property{}},
		sh.GitStatus)

	s.Register(ToolGitDiff, "Get the git diff (staged or unstaged) of the project.",
		Schema{
			Type: "object",
			Properties: map[string]Property{
				"staged": {Type: "boolean", Description: "Show staged diff (default: false)"},
			},
		}, sh.GitDiff)
}

// Register adds a tool with its definition and handler.
func (s *Server) Register(name ToolKind, description string, schema Schema, handler ToolHandler) {
	s.tools[name] = handler
	s.defs = append(s.defs, ToolDef{
		Name:        name,
		Description: description,
		Parameters:  schema,
	})
}

// Dispatch executes a tool call and returns the result.
func (s *Server) Dispatch(ctx context.Context, call ToolCall) ToolResult {
	handler, ok := s.tools[call.Name]
	if !ok {
		return ToolResult{
			Tool:    call.Name,
			Success: false,
			Error:   fmt.Sprintf("unknown tool: %s", call.Name),
		}
	}
	return handler(ctx, call.Params)
}

// Definitions returns the tool definitions for LLM context injection.
func (s *Server) Definitions() []ToolDef {
	return s.defs
}

// FormatToolDefsForPrompt renders tool definitions as a prompt block.
func (s *Server) FormatToolDefsForPrompt() string {
	data, _ := json.MarshalIndent(s.defs, "", "  ")
	return fmt.Sprintf("### Available Tools\n\nWhen you need to perform an action, respond with a JSON tool call inside <tool_call> tags:\n\n```json\n<tool_call>\n{\"name\": \"<tool_name>\", \"parameters\": {...}}\n</tool_call>\n```\n\nAvailable tools:\n```json\n%s\n```\n", data)
}

// ParseToolCall attempts to extract a tool call from LLM output.
func ParseToolCall(output string) (*ToolCall, bool) {
	start := findTag(output, "<tool_call>")
	end := findTag(output, "</tool_call>")
	if start == -1 || end == -1 || end <= start {
		return nil, false
	}
	jsonStr := output[start+len("<tool_call>") : end]

	// Strip possible code fence
	jsonStr = stripCodeFence(jsonStr)

	var call ToolCall
	if err := json.Unmarshal([]byte(jsonStr), &call); err != nil {
		return nil, false
	}
	return &call, true
}

func findTag(s, tag string) int {
	for i := 0; i <= len(s)-len(tag); i++ {
		if s[i:i+len(tag)] == tag {
			return i
		}
	}
	return -1
}

func stripCodeFence(s string) string {
	s = trimPrefix(s, "```json\n")
	s = trimPrefix(s, "```\n")
	s = trimSuffix(s, "\n```")
	return s
}

func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}
