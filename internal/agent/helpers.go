// Package agent — shared helper utilities used across the agent sub-system.
package agent

import (
	"strings"
)

// boolIcon returns a checkmark or cross depending on whether ok is true.
func boolIcon(ok bool) string {
	if ok {
		return "✅"
	}
	return "❌"
}

// extToLang maps a file extension to a human-readable language identifier.
// Returns "text" for unknown extensions.
func extToLang(path string) string {
	switch {
	case strings.HasSuffix(path, ".go"):
		return "go"
	case strings.HasSuffix(path, ".py"):
		return "python"
	case strings.HasSuffix(path, ".ts"):
		return "typescript"
	case strings.HasSuffix(path, ".js"):
		return "javascript"
	case strings.HasSuffix(path, ".lua"):
		return "lua"
	case strings.HasSuffix(path, ".rs"):
		return "rust"
	case strings.HasSuffix(path, ".cpp"):
		return "cpp"
	case strings.HasSuffix(path, ".c"), strings.HasSuffix(path, ".h"):
		return "c"
	case strings.HasSuffix(path, ".java"):
		return "java"
	default:
		return "text"
	}
}

// truncateOutput trims a string to max bytes, inserting an ellipsis in the
// middle so both the beginning and end of the output remain visible.
func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	half := max / 2
	return s[:half] + "\n…[truncated]…\n" + s[len(s)-half:]
}
