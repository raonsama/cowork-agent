// Package agent implements the autonomous cowork engine.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/raonsama/cowork-agent/internal/llm"
)

// Step is one discrete action in the execution plan.
type Step struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	ToolHint    string `json:"tool_hint,omitempty"` // expected MCP tool
	Done        bool   `json:"-"`
	Result      string `json:"-"`
}

// Plan is the full task decomposition.
type Plan struct {
	Task    string `json:"task"`
	Steps   []Step `json:"steps"`
	Summary string `json:"summary"`
}

// Planner uses the LLM to decompose a high-level task into steps.
type Planner struct {
	client *llm.Client
	ctx    *llm.ContextManager
}

// NewPlanner creates a Planner with a dedicated context.
func NewPlanner(client *llm.Client, contextWindow int) *Planner {
	system := `You are a senior software engineer planning autonomous coding tasks.
Given a task description, produce a step-by-step execution plan in JSON.

Respond ONLY with a valid JSON object (no markdown, no preamble) matching this schema:
{
  "task": "<task summary>",
  "steps": [
    {"id": 1, "description": "<action>", "tool_hint": "<mcp_tool_name>"},
    ...
  ],
  "summary": "<one sentence overview>"
}

Available tool hints: read_file, write_file, list_dir, run_shell, search_code, apply_patch, create_dir, git_status, git_diff.
Keep steps atomic and specific. Max 10 steps.`

	return &Planner{
		client: client,
		ctx:    llm.NewContextManager(contextWindow, system),
	}
}

// CreatePlan sends the task to the LLM and parses the resulting plan.
func (p *Planner) CreatePlan(ctx context.Context, task string) (*Plan, error) {
	prompt := fmt.Sprintf("Task: %s\n\nGenerate the execution plan.", task)
	p.ctx.AddMessage("user", prompt)

	messages := p.ctx.Build()
	response, err := p.client.ChatSync(ctx, messages, llm.Options{
		Temperature: 0.2,
		NumCtx:      p.ctx.TokenUsage() + 512,
	})
	if err != nil {
		return nil, fmt.Errorf("planner llm call: %w", err)
	}

	p.ctx.AddMessage("assistant", response)

	plan, err := parsePlan(response)
	if err != nil {
		// Fallback: single-step plan
		return &Plan{
			Task: task,
			Steps: []Step{
				{ID: 1, Description: task, ToolHint: "run_shell"},
			},
			Summary: task,
		}, nil
	}
	return plan, nil
}

// RefinePlan sends a correction prompt when a step fails.
func (p *Planner) RefinePlan(ctx context.Context, plan *Plan, failedStep Step, errorMsg string) (*Plan, error) {
	if p.client == nil {
		return plan, nil
	}

	prompt := fmt.Sprintf(
		"Step %d failed: %q\nError: %s\n\nRevise the remaining plan from step %d onwards. Respond with updated JSON.",
		failedStep.ID, failedStep.Description, errorMsg, failedStep.ID,
	)
	p.ctx.AddMessage("user", prompt)
	messages := p.ctx.Build()

	response, err := p.client.ChatSync(ctx, messages, llm.Options{
		Temperature: 0.1,
	})
	if err != nil {
		return plan, err
	}
	p.ctx.AddMessage("assistant", response)

	revised, err := parsePlan(response)
	if err != nil {
		return plan, nil
	}
	return revised, nil
}

// parsePlan extracts JSON from potentially dirty LLM output.
func parsePlan(raw string) (*Plan, error) {
	// Strip markdown code fences if present
	raw = stripJSON(raw)

	var plan Plan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		// Try to find the JSON object in free-form text
		start := strings.Index(raw, "{")
		end := strings.LastIndex(raw, "}")
		if start == -1 || end == -1 || end <= start {
			return nil, fmt.Errorf("no JSON found in response")
		}
		if err2 := json.Unmarshal([]byte(raw[start:end+1]), &plan); err2 != nil {
			return nil, fmt.Errorf("parse plan: %w", err2)
		}
	}
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("plan has no steps")
	}
	return &plan, nil
}

func stripJSON(s string) string {
	s = strings.TrimSpace(s)
	for _, fence := range []string{"```json\n", "```\n", "```"} {
		if strings.HasPrefix(s, fence) {
			s = s[len(fence):]
			break
		}
	}
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}
