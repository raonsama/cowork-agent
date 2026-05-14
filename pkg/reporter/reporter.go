package reporter

import (
	"fmt"
	"strings"
	"time"
)

// StepLog captures the result of one agent step.
type StepLog struct {
	ID     int
	Desc   string
	Tool   string
	Output string
	Passed bool
	Reason string
}

// Report is the final wake-up report produced after cowork mode finishes.
type Report struct {
	Task        string
	Branch      string
	BaseBranch  string
	Steps       []StepLog
	Diff        string
	DiffStat    string
	ElapsedSecs int
	Timestamp   time.Time
}

// OneLiner returns a compact summary for the Termux notification.
func (r *Report) OneLiner() string {
	passed := 0
	for _, s := range r.Steps {
		if s.Passed {
			passed++
		}
	}
	return fmt.Sprintf("%d/%d steps passed · %s", passed, len(r.Steps), r.formatElapsed())
}

// Markdown renders the full wake-up report as markdown text.
func (r *Report) Markdown() string {
	var sb strings.Builder

	sb.WriteString("---\n\n")
	sb.WriteString("# 🌙 Wake-Up Report\n\n")
	sb.WriteString(fmt.Sprintf("**Task:** %s\n\n", r.Task))
	sb.WriteString(fmt.Sprintf("**Branch:** `%s` → `%s`\n\n", r.Branch, r.BaseBranch))
	sb.WriteString(fmt.Sprintf("**Elapsed:** %s\n\n", r.formatElapsed()))
	sb.WriteString(fmt.Sprintf("**Timestamp:** %s\n\n", r.Timestamp.Format("2006-01-02 15:04:05")))

	// Step summary
	sb.WriteString("## Steps\n\n")
	passed, failed := 0, 0
	for _, s := range r.Steps {
		icon := "✅"
		if !s.Passed {
			icon = "❌"
			failed++
		} else {
			passed++
		}
		sb.WriteString(fmt.Sprintf("%s **[%d]** `%s` — %s\n", icon, s.ID, s.Tool, s.Desc))
		if s.Reason != "" && s.Reason != "No error signals detected (heuristic)" {
			sb.WriteString(fmt.Sprintf("   > %s\n", s.Reason))
		}
		if !s.Passed && s.Output != "" {
			sb.WriteString("   ```\n")
			lines := strings.Split(s.Output, "\n")
			if len(lines) > 6 {
				lines = lines[:6]
				lines = append(lines, "…")
			}
			for _, l := range lines {
				sb.WriteString("   " + l + "\n")
			}
			sb.WriteString("   ```\n")
		}
	}
	sb.WriteString(fmt.Sprintf("\n**Result:** %d passed · %d failed\n\n", passed, failed))

	// Diff stat
	if r.DiffStat != "" {
		sb.WriteString("## Changes\n\n")
		sb.WriteString("```diff\n")
		sb.WriteString(r.DiffStat)
		sb.WriteString("```\n\n")
	}

	// Closing quote (the night-shift persona sign-off)
	closing := closingQuote(passed, failed)
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("_\"%s\"_\n", closing))

	return sb.String()
}

// closingQuote returns a contextual sign-off line.
func closingQuote(passed, failed int) string {
	switch {
	case failed == 0:
		return "Tugas selesai. Mimpimu sudah aman di kode ini, silakan lanjut rebahan. 🌙"
	case passed == 0:
		return "Semua step gagal — tapi gue udah catat semuanya. Kita review bareng pas kamu bangun. ☀️"
	default:
		return fmt.Sprintf("%d step sukses, %d butuh perhatianmu. Review diff-nya dulu, baru merge. 🔍", passed, failed)
	}
}

func (r *Report) formatElapsed() string {
	secs := r.ElapsedSecs
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%dm %ds", secs/60, secs%60)
	}
	return fmt.Sprintf("%dh %dm", secs/3600, (secs%3600)/60)
}
