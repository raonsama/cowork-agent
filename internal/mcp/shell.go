package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// shellTools implements shell-execution MCP tool handlers.
type shellTools struct {
	root string
}

func newShellTools(root string) *shellTools {
	return &shellTools{root: root}
}

// ── RunShell ─────────────────────────────────────────────

type runShellParams struct {
	Command    string `json:"command"`
	TimeoutSec int    `json:"timeout_sec"`
}

// blockedCommands is a hard deny-list to prevent catastrophic ops.
var blockedCommands = []string{
	"rm -rf /", "rmdir /", "mkfs", "dd if=",
	":(){ :|:& };:", // fork bomb
	"shutdown", "reboot", "halt",
}

func (s *shellTools) RunShell(ctx context.Context, raw json.RawMessage) ToolResult {
	var p runShellParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return errResult(ToolRunShell, "bad params: "+err.Error())
	}
	if p.Command == "" {
		return errResult(ToolRunShell, "command is empty")
	}

	// Safety check
	cmdLower := strings.ToLower(p.Command)
	for _, blocked := range blockedCommands {
		if strings.Contains(cmdLower, blocked) {
			return errResult(ToolRunShell, fmt.Sprintf("blocked command: %q", blocked))
		}
	}

	timeout := time.Duration(p.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", p.Command)
	cmd.Dir = s.root

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	combined := stdout.String()
	if stderr.Len() > 0 {
		combined += "\n[stderr]\n" + stderr.String()
	}

	// Truncate to avoid flooding context
	if len(combined) > 8000 {
		combined = combined[:8000] + "\n… [truncated]"
	}

	if err != nil {
		return ToolResult{
			Tool:    ToolRunShell,
			Success: false,
			Output:  combined,
			Error:   err.Error(),
		}
	}
	return ToolResult{Tool: ToolRunShell, Success: true, Output: combined}
}

// ── GitStatus ────────────────────────────────────────────

func (s *shellTools) GitStatus(ctx context.Context, _ json.RawMessage) ToolResult {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "status", "--short", "--branch")
	cmd.Dir = s.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{
			Tool: ToolGitStatus, Success: false,
			Output: string(out), Error: err.Error(),
		}
	}
	return ToolResult{Tool: ToolGitStatus, Success: true, Output: string(out)}
}

// ── GitDiff ──────────────────────────────────────────────

type gitDiffParams struct {
	Staged bool `json:"staged"`
}

func (s *shellTools) GitDiff(ctx context.Context, raw json.RawMessage) ToolResult {
	var p gitDiffParams
	_ = json.Unmarshal(raw, &p)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	args := []string{"diff", "--stat"}
	if p.Staged {
		args = append(args, "--cached")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{
			Tool: ToolGitDiff, Success: false,
			Output: string(out), Error: err.Error(),
		}
	}

	// Also get the actual diff (limited)
	args2 := []string{"diff"}
	if p.Staged {
		args2 = append(args2, "--cached")
	}
	cmd2 := exec.CommandContext(ctx, "git", args2...)
	cmd2.Dir = s.root
	diff, _ := cmd2.CombinedOutput()

	result := string(out) + "\n" + string(diff)
	if len(result) > 6000 {
		result = result[:6000] + "\n… [diff truncated]"
	}
	return ToolResult{Tool: ToolGitDiff, Success: true, Output: result}
}
