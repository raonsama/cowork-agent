// Package termux wraps the termux-api notification and vibration commands,
// silently disabling itself in non-Termux environments.
package termux

import (
	"fmt"
	"os/exec"
	"strings"
)

// Priority levels for termux-notification.
type Priority string

const (
	PriorityDefault Priority = "default"
	PriorityHigh    Priority = "high"
	PriorityLow     Priority = "low"
	PriorityMax     Priority = "max"
	PriorityMin     Priority = "min"
)

// Bridge wraps termux-api notification commands.
type Bridge struct {
	enabled bool
}

// NewBridge creates a Bridge. It silently disables itself if
// termux-notification is not in PATH (non-Termux environments).
func NewBridge(enabled bool) *Bridge {
	if enabled {
		_, err := exec.LookPath("termux-notification")
		if err != nil {
			enabled = false
		}
	}
	return &Bridge{enabled: enabled}
}

// IsAvailable returns whether the Termux bridge is active.
func (b *Bridge) IsAvailable() bool {
	return b.enabled
}

// NotifyTaskComplete sends a "wake-up" notification after cowork mode finishes.
func (b *Bridge) NotifyTaskComplete(taskName, summary string) error {
	if !b.enabled {
		return nil
	}
	title := fmt.Sprintf("✅ CoworkAgent — Task Done")
	content := fmt.Sprintf("%s\n\n%s\n\nTugas selesai. Lanjut rebahan, bos.", taskName, truncate(summary, 200))
	return b.notify(title, content, PriorityHigh, "🤖")
}

// NotifyError sends a notification when the agent encounters a fatal error.
func (b *Bridge) NotifyError(taskName, errMsg string) error {
	if !b.enabled {
		return nil
	}
	title := "❌ CoworkAgent — Error"
	content := fmt.Sprintf("Task: %s\n\n%s", taskName, truncate(errMsg, 200))
	return b.notify(title, content, PriorityMax, "🚨")
}

// NotifyProgress sends a lightweight status update.
func (b *Bridge) NotifyProgress(phase, detail string) error {
	if !b.enabled {
		return nil
	}
	title := fmt.Sprintf("⚙️  CoworkAgent — %s", phase)
	return b.notify(title, truncate(detail, 150), PriorityLow, "")
}

// notify is the low-level wrapper around termux-notification.
func (b *Bridge) notify(title, content string, priority Priority, icon string) error {
	args := []string{
		"--title", title,
		"--content", content,
		"--priority", string(priority),
		"--id", "cowork-agent-1",
		"--group", "cowork-agent",
		"--ongoing",
	}
	if icon != "" {
		// termux-notification doesn't support emoji icons natively,
		// but we embed them in the title/content already.
		_ = icon
	}

	cmd := exec.Command("termux-notification", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("termux-notification: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Vibrate triggers a short haptic buzz (requires termux-api).
func (b *Bridge) Vibrate(ms int) {
	if !b.enabled {
		return
	}
	cmd := exec.Command("termux-vibrate", "-d", fmt.Sprintf("%d", ms))
	_ = cmd.Run()
}

// Dismiss removes the persistent notification.
func (b *Bridge) Dismiss() {
	if !b.enabled {
		return
	}
	cmd := exec.Command("termux-notification-remove", "cowork-agent-1")
	_ = cmd.Run()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
