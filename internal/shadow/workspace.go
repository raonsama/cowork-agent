package shadow

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Workspace manages a temporary Git branch for isolated agent work.
type Workspace struct {
	root         string
	branchPrefix string
	branchName   string
	baseBranch   string
	active       bool
}

// NewWorkspace creates a Workspace for the given project root.
func NewWorkspace(root, branchPrefix string) *Workspace {
	return &Workspace{
		root:         root,
		branchPrefix: branchPrefix,
	}
}

// Begin creates and checks out a new shadow branch based on the task slug.
func (w *Workspace) Begin(taskSlug string) error {
	base, err := w.currentBranch()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	w.baseBranch = base

	slug := sanitizeSlug(taskSlug)
	ts := time.Now().Format("0102-1504")
	w.branchName = fmt.Sprintf("%s/%s-%s", w.branchPrefix, slug, ts)

	if err := w.git("checkout", "-b", w.branchName); err != nil {
		return fmt.Errorf("create branch %q: %w", w.branchName, err)
	}
	w.active = true
	return nil
}

// BranchName returns the active shadow branch name.
func (w *Workspace) BranchName() string {
	return w.branchName
}

// BaseBranch returns the branch that was active when Begin was called.
func (w *Workspace) BaseBranch() string {
	return w.baseBranch
}

// StageAll stages all changed files.
func (w *Workspace) StageAll() error {
	return w.git("add", "-A")
}

// Commit creates a commit with the given message.
func (w *Workspace) Commit(msg string) error {
	if !w.active {
		return nil
	}
	out, err := w.gitOutput("status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil // nothing to commit
	}
	if err := w.git("add", "-A"); err != nil {
		return err
	}
	return w.git("commit", "-m", fmt.Sprintf("[cowork] %s", msg))
}

// Diff returns a unified diff against the base branch.
func (w *Workspace) Diff() (string, error) {
	out, err := w.gitOutput("diff", w.baseBranch+"..."+w.branchName)
	if err != nil {
		return "", err
	}
	return out, nil
}

// Stat returns a short stat of changes vs the base branch.
func (w *Workspace) Stat() (string, error) {
	return w.gitOutput("diff", "--stat", w.baseBranch+"..."+w.branchName)
}

// MergeInto fast-forward merges the shadow branch into baseBranch and deletes
// the shadow branch. Call only after the human has reviewed the diff.
func (w *Workspace) MergeInto() error {
	if err := w.git("checkout", w.baseBranch); err != nil {
		return fmt.Errorf("checkout base: %w", err)
	}
	if err := w.git("merge", "--ff-only", w.branchName); err != nil {
		return fmt.Errorf("fast-forward merge: %w", err)
	}
	return w.git("branch", "-d", w.branchName)
}

// Abort checks out the base branch and deletes the shadow branch.
func (w *Workspace) Abort() error {
	if err := w.git("checkout", w.baseBranch); err != nil {
		return err
	}
	return w.git("branch", "-D", w.branchName)
}

// IsClean returns true if there are no uncommitted changes.
func (w *Workspace) IsClean() bool {
	out, err := w.gitOutput("status", "--porcelain")
	return err == nil && strings.TrimSpace(out) == ""
}

// ─── helpers ─────────────────────────────────────────────

func (w *Workspace) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = w.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

func (w *Workspace) gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = w.root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (w *Workspace) currentBranch() (string, error) {
	out, err := w.gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(out), err
}

// sanitizeSlug converts a task description to a git-safe branch slug.
func sanitizeSlug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 30 {
		slug = slug[:30]
	}
	if slug == "" {
		slug = "task"
	}
	return slug
}
