// Package mcp implements the Model Context Protocol tool server,
// providing filesystem, shell, and git tools for the agent to invoke.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// fsTools implements all filesystem-related MCP tool handlers.
type fsTools struct {
	root string
}

func newFSTools(root string) *fsTools {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return &fsTools{root: abs}
}

// safePath resolves a relative path against the project root and
// ensures it does not escape the root (path traversal guard).
func (f *fsTools) safePath(p string) (string, error) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(f.root, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if !strings.HasPrefix(abs, f.root) {
		return "", fmt.Errorf("path escapes project root: %s", abs)
	}
	return abs, nil
}

// ── ReadFile ─────────────────────────────────────────────

type readFileParams struct {
	Path string `json:"path"`
}

func (f *fsTools) ReadFile(_ context.Context, raw json.RawMessage) ToolResult {
	var p readFileParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return errResult(ToolReadFile, "bad params: "+err.Error())
	}
	abs, err := f.safePath(p.Path)
	if err != nil {
		return errResult(ToolReadFile, err.Error())
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return errResult(ToolReadFile, err.Error())
	}
	return ToolResult{Tool: ToolReadFile, Success: true, Output: string(data)}
}

// ── WriteFile ────────────────────────────────────────────

type writeFileParams struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (f *fsTools) WriteFile(_ context.Context, raw json.RawMessage) ToolResult {
	var p writeFileParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return errResult(ToolWriteFile, "bad params: "+err.Error())
	}
	abs, err := f.safePath(p.Path)
	if err != nil {
		return errResult(ToolWriteFile, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return errResult(ToolWriteFile, "mkdir: "+err.Error())
	}
	if err := os.WriteFile(abs, []byte(p.Content), 0o644); err != nil {
		return errResult(ToolWriteFile, err.Error())
	}
	rel, _ := filepath.Rel(f.root, abs)
	return ToolResult{
		Tool: ToolWriteFile, Success: true,
		Output: fmt.Sprintf("written %d bytes → %s", len(p.Content), rel),
	}
}

// ── ListDir ──────────────────────────────────────────────

type listDirParams struct {
	Path string `json:"path"`
}

func (f *fsTools) ListDir(_ context.Context, raw json.RawMessage) ToolResult {
	var p listDirParams
	_ = json.Unmarshal(raw, &p)
	if p.Path == "" {
		p.Path = "."
	}
	abs, err := f.safePath(p.Path)
	if err != nil {
		return errResult(ToolListDir, err.Error())
	}

	var sb strings.Builder
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(abs, path)
		if rel == "." {
			return nil
		}
		depth := strings.Count(rel, string(filepath.Separator))
		if depth > 3 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		indent := strings.Repeat("  ", depth)
		if d.IsDir() {
			sb.WriteString(fmt.Sprintf("%s📁 %s/\n", indent, d.Name()))
		} else {
			info, _ := d.Info()
			size := ""
			if info != nil {
				size = fmt.Sprintf(" (%d B)", info.Size())
			}
			sb.WriteString(fmt.Sprintf("%s📄 %s%s\n", indent, d.Name(), size))
		}
		return nil
	})
	if err != nil {
		return errResult(ToolListDir, err.Error())
	}
	return ToolResult{Tool: ToolListDir, Success: true, Output: sb.String()}
}

// ── CreateDir ────────────────────────────────────────────

type createDirParams struct {
	Path string `json:"path"`
}

func (f *fsTools) CreateDir(_ context.Context, raw json.RawMessage) ToolResult {
	var p createDirParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return errResult(ToolCreateDir, "bad params: "+err.Error())
	}
	abs, err := f.safePath(p.Path)
	if err != nil {
		return errResult(ToolCreateDir, err.Error())
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return errResult(ToolCreateDir, err.Error())
	}
	return ToolResult{
		Tool: ToolCreateDir, Success: true,
		Output: fmt.Sprintf("created directory: %s", p.Path),
	}
}

// ── DeleteFile ───────────────────────────────────────────

type deleteFileParams struct {
	Path string `json:"path"`
}

func (f *fsTools) DeleteFile(_ context.Context, raw json.RawMessage) ToolResult {
	var p deleteFileParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return errResult(ToolDeleteFile, "bad params: "+err.Error())
	}
	abs, err := f.safePath(p.Path)
	if err != nil {
		return errResult(ToolDeleteFile, err.Error())
	}
	if err := os.Remove(abs); err != nil {
		return errResult(ToolDeleteFile, err.Error())
	}
	return ToolResult{
		Tool: ToolDeleteFile, Success: true,
		Output: fmt.Sprintf("deleted: %s", p.Path),
	}
}

// ── ApplyPatch ───────────────────────────────────────────

type applyPatchParams struct {
	Patch string `json:"patch"`
}

func (f *fsTools) ApplyPatch(ctx context.Context, raw json.RawMessage) ToolResult {
	var p applyPatchParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return errResult(ToolApplyPatch, "bad params: "+err.Error())
	}

	// Write patch to a temp file
	tmp, err := os.CreateTemp("", "cowork-*.patch")
	if err != nil {
		return errResult(ToolApplyPatch, "tmp file: "+err.Error())
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(p.Patch); err != nil {
		return errResult(ToolApplyPatch, "write patch: "+err.Error())
	}
	tmp.Close()

	cmd := exec.CommandContext(ctx, "patch", "-p1", "--input", tmp.Name())
	cmd.Dir = f.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{
			Tool:    ToolApplyPatch,
			Success: false,
			Output:  string(out),
			Error:   err.Error(),
		}
	}
	return ToolResult{Tool: ToolApplyPatch, Success: true, Output: string(out)}
}

// ── helpers ──────────────────────────────────────────────

func errResult(tool ToolKind, msg string) ToolResult {
	return ToolResult{Tool: tool, Success: false, Error: msg}
}
