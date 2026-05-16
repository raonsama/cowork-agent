// Package agent implements the autonomous cowork engine.
// This file contains the Verifier, which uses the LLM to assess step results,
// with a fast heuristic fallback when the LLM is unavailable or disabled.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/raonsama/cowork-agent/internal/llm"
)

// Verdict is the verifier's assessment of a step result.
type Verdict struct {
	Passed  bool
	Reason  string
	Suggest string // optional follow-up action
}

// Verifier uses the LLM to assess whether a step completed correctly.
type Verifier struct {
	client *llm.Client
	ctx    *llm.ContextManager
}

// NewVerifier creates a Verifier with its own lightweight context.
func NewVerifier(client *llm.Client, contextWindow int) *Verifier {
	system := `You are a code review assistant verifying the output of automated coding steps.
Given the step description and its output, decide if it succeeded.

Respond in this exact format (no markdown):
PASSED: <yes|no>
REASON: <one sentence>
SUGGEST: <optional one-line follow-up, or "none">`

	return &Verifier{
		client: client,
		ctx:    llm.NewContextManager(contextWindow/2, system),
	}
}

// Verify checks whether a step's output indicates success.
func (v *Verifier) Verify(ctx context.Context, step Step, toolOutput string) (Verdict, error) {
	prompt := fmt.Sprintf(
		"Step: %s\n\nTool output:\n%s",
		step.Description,
		truncateOutput(toolOutput, 1500),
	)
	v.ctx.AddMessage("user", prompt)

	messages := v.ctx.Build()
	response, err := v.client.ChatSync(ctx, messages, llm.Options{
		Temperature: 0.1,
	})
	if err != nil {
		// On LLM failure, fall back to heuristic check
		return heuristicVerdict(toolOutput), nil
	}

	v.ctx.AddMessage("assistant", response)
	return parseVerdict(response), nil
}

// VerifyBuild runs a build/test command and verifies it passes.
func (v *Verifier) VerifyBuild(ctx context.Context, buildOutput string) Verdict {
	lower := strings.ToLower(buildOutput)
	fail := strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "panic")

	if fail {
		return Verdict{
			Passed:  false,
			Reason:  "Build output contains errors",
			Suggest: "fix compilation errors",
		}
	}
	return Verdict{Passed: true, Reason: "Build appears clean"}
}

// parseVerdict parses the structured LLM verifier response.
func parseVerdict(raw string) Verdict {
	var v Verdict
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "PASSED:"); ok {
			val := strings.TrimSpace(after)
			v.Passed = strings.EqualFold(val, "yes")
		} else if after, ok := strings.CutPrefix(line, "REASON:"); ok {
			v.Reason = strings.TrimSpace(after)
		} else if after, ok := strings.CutPrefix(line, "SUGGEST:"); ok {
			v.Suggest = strings.TrimSpace(after)
			if strings.EqualFold(v.Suggest, "none") {
				v.Suggest = ""
			}
		}
	}
	if v.Reason == "" {
		v.Reason = "No reason provided"
	}
	return v
}

// heuristicVerdict is a fallback when the LLM is unavailable.
func heuristicVerdict(output string) Verdict {
	lower := strings.ToLower(output)
	bad := []string{"error:", "failed", "permission denied", "not found", "panic:", "exception"}
	for _, b := range bad {
		if strings.Contains(lower, b) {
			return Verdict{Passed: false, Reason: fmt.Sprintf("Output contains %q", b)}
		}
	}
	return Verdict{Passed: true, Reason: "No error signals detected (heuristic)"}
}
