package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/raonsama/cowork-agent/internal/config"
	"github.com/raonsama/cowork-agent/internal/indexer"
	"github.com/raonsama/cowork-agent/internal/llm"
	"github.com/raonsama/cowork-agent/internal/mcp"
	"github.com/raonsama/cowork-agent/internal/shadow"
	"github.com/raonsama/cowork-agent/internal/termux"
	"github.com/raonsama/cowork-agent/internal/thermal"
	"github.com/raonsama/cowork-agent/pkg/reporter"
)

// Phase labels for UI status reporting.
type Phase string

const (
	PhaseIdle     Phase = "idle"
	PhasePlan     Phase = "planning"
	PhaseSearch   Phase = "searching"
	PhaseExecute  Phase = "executing"
	PhaseVerify   Phase = "verifying"
	PhaseCommit   Phase = "committing"
	PhaseDone     Phase = "done"
	PhaseError    Phase = "error"
	PhaseThrottle Phase = "throttled"
)

// Event is emitted by the agent loop so the TUI can render live status.
type Event struct {
	Phase      Phase
	StepID     int
	StepDesc   string
	ToolName   string
	ToolOutput string
	Message    string
	Branch     string
	Err        error
	Final      *reporter.Report
}

// Agent is the autonomous cowork engine.
type Agent struct {
	cfg      *config.Config
	client   *llm.Client
	planner  *Planner
	verifier *Verifier
	mcpSrv   *mcp.Server
	db       *indexer.DB
	thermal  *thermal.Monitor
	termux   *termux.Bridge
	eventCh  chan Event
}

// New constructs the Agent with all sub-systems wired up.
func New(cfg *config.Config) (*Agent, error) {
	client := llm.NewClient(cfg.OllamaBaseURL, cfg.DefaultModel)

	db, err := indexer.OpenDB(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open index db: %w", err)
	}

	return &Agent{
		cfg:      cfg,
		client:   client,
		planner:  NewPlanner(client, cfg.ContextWindow),
		verifier: NewVerifier(client, cfg.ContextWindow),
		mcpSrv:   mcp.NewServer(cfg.ProjectRoot),
		db:       db,
		thermal:  thermal.NewMonitor(cfg.ThermalThresholdCelsius, cfg.CPUThrottlePercent),
		termux:   termux.NewBridge(cfg.TermuxNotifyEnabled),
		eventCh:  make(chan Event, 64),
	}, nil
}

// Events returns the channel on which Agent broadcasts state events.
func (a *Agent) Events() <-chan Event {
	return a.eventCh
}

// Close releases resources.
func (a *Agent) Close() {
	a.db.Close()
	a.thermal.Stop()
}

// Run executes the full cowork loop for a given task description.
// It is designed to run in its own goroutine; the TUI consumes Events().
func (a *Agent) Run(ctx context.Context, task string) {
	defer close(a.eventCh)

	a.thermal.Start()
	startTime := time.Now()

	// ── Ping Ollama ──────────────────────────────────────────
	if err := a.client.Ping(ctx); err != nil {
		a.emit(Event{Phase: PhaseError, Err: fmt.Errorf("ollama unreachable: %w", err)})
		return
	}

	// ── Shadow workspace ─────────────────────────────────────
	ws := shadow.NewWorkspace(a.cfg.ProjectRoot, a.cfg.BranchPrefix)
	if err := ws.Begin(task); err != nil {
		// Non-fatal: continue without shadow workspace (e.g., no git repo)
		a.emit(Event{Phase: PhasePlan, Message: "⚠️  Shadow workspace unavailable — working on current branch"})
	} else {
		a.emit(Event{Phase: PhasePlan, Branch: ws.BranchName(), Message: "Shadow branch: " + ws.BranchName()})
	}

	// ── Plan ─────────────────────────────────────────────────
	a.emit(Event{Phase: PhasePlan, Message: "Decomposing task into steps…"})
	plan, err := a.planner.CreatePlan(ctx, task)
	if err != nil {
		a.emit(Event{Phase: PhaseError, Err: err})
		return
	}
	a.emit(Event{Phase: PhasePlan, Message: fmt.Sprintf("Plan ready: %d steps — %s", len(plan.Steps), plan.Summary)})

	// ── Execution loop ───────────────────────────────────────
	log := []reporter.StepLog{}
	maxRetries := 2

	for i := range plan.Steps {
		step := &plan.Steps[i]
		if step.Done {
			continue
		}

		// Thermal throttle check
		if a.thermal.IsThrottled() {
			status := a.thermal.Current()
			a.emit(Event{Phase: PhaseThrottle, Message: status.ThrottleMsg})
			a.thermal.WaitIfThrottled(ctx.Done())
		}

		// ── Context-aware code search ──────────────────────
		a.emit(Event{
			Phase: PhaseSearch, StepID: step.ID, StepDesc: step.Description,
			Message: "Searching relevant code context…",
		})
		snippets := a.searchContext(step.Description)
		contextBlock := ""
		if len(snippets) > 0 {
			cm := llm.NewContextManager(a.cfg.ContextWindow, "")
			contextBlock = cm.InjectContext(snippets)
		}

		// ── Execute step (with retries) ────────────────────
		var toolResult mcp.ToolResult
		var verdict Verdict
		retries := 0

		for retries <= maxRetries {
			// Ask LLM what tool call to make for this step
			toolCall, err := a.resolveToolCall(ctx, plan, step, contextBlock)
			if err != nil {
				a.emit(Event{Phase: PhaseError, StepID: step.ID, Err: err})
				break
			}

			a.emit(Event{
				Phase:    PhaseExecute,
				StepID:   step.ID,
				StepDesc: step.Description,
				ToolName: string(toolCall.Name),
				Message:  fmt.Sprintf("→ %s", toolCall.Name),
			})

			toolResult = a.mcpSrv.Dispatch(ctx, *toolCall)

			// Verify
			a.emit(Event{Phase: PhaseVerify, StepID: step.ID, Message: "Verifying result…"})
			verdict, _ = a.verifier.Verify(ctx, *step, toolResult.Output+toolResult.Error)

			if verdict.Passed || retries >= maxRetries {
				break
			}

			retries++
			a.emit(Event{
				Phase:   PhaseVerify,
				StepID:  step.ID,
				Message: fmt.Sprintf("⚠️  Retry %d/%d — %s", retries, maxRetries, verdict.Reason),
			})

			// Refine plan on repeated failure
			if retries == maxRetries {
				plan, _ = a.planner.RefinePlan(ctx, plan, *step, verdict.Reason)
			}

			a.emit(Event{
				Phase: PhaseExecute, StepID: step.ID,
				ToolName: string(toolCall.Name), ToolOutput: toolResult.Output,
			})
		}

		step.Done = true
		step.Result = toolResult.Output

		log = append(log, reporter.StepLog{
			ID:     step.ID,
			Desc:   step.Description,
			Tool:   string(toolResult.Tool),
			Output: truncateOutput(toolResult.Output, 500),
			Passed: verdict.Passed,
			Reason: verdict.Reason,
		})

		a.emit(Event{
			Phase:      PhaseExecute,
			StepID:     step.ID,
			ToolOutput: toolResult.Output,
			Message:    fmt.Sprintf("Step %d %s", step.ID, boolIcon(verdict.Passed)),
		})
	}

	// ── Commit to shadow branch ───────────────────────────
	a.emit(Event{Phase: PhaseCommit, Message: "Committing changes to shadow branch…"})
	diff, _ := ws.Diff()
	stat, _ := ws.Stat()
	_ = ws.Commit(plan.Summary)

	// ── Wake-up report ────────────────────────────────────
	elapsed := time.Since(startTime)
	rpt := &reporter.Report{
		Task:        task,
		Branch:      ws.BranchName(),
		BaseBranch:  ws.BaseBranch(),
		Steps:       log,
		Diff:        diff,
		DiffStat:    stat,
		ElapsedSecs: int(elapsed.Seconds()),
		Timestamp:   time.Now(),
	}

	a.emit(Event{Phase: PhaseDone, Final: rpt, Message: "Tugas selesai. Mimpimu sudah aman di kode ini, silakan lanjut rebahan. 🌙"})

	// Termux notification
	_ = a.termux.NotifyTaskComplete(task, rpt.OneLiner())
	a.termux.Vibrate(300)
}

// resolveToolCall asks the LLM to produce the correct MCP tool call for a step.
func (a *Agent) resolveToolCall(ctx context.Context, plan *Plan, step *Step, codeCtx string) (*mcp.ToolCall, error) {
	toolDefsBlock := a.mcpSrv.FormatToolDefsForPrompt()
	planJSON, _ := json.MarshalIndent(plan, "", "  ")

	prompt := strings.Join([]string{
		toolDefsBlock,
		"",
		"### Current Plan",
		"```json",
		string(planJSON),
		"```",
		"",
		codeCtx,
		"",
		fmt.Sprintf("### Current Step (ID %d)", step.ID),
		step.Description,
		"",
		"Select the best tool and parameters. Respond with a single <tool_call> block.",
	}, "\n")

	systemPrompt := `You are an autonomous coding agent executing a task plan.
For each step, respond with exactly one tool call inside <tool_call> tags.
Do not add any other text.`

	cm := llm.NewContextManager(a.cfg.ContextWindow, systemPrompt)
	cm.AddMessage("user", prompt)

	resp, err := a.client.ChatSync(ctx, cm.Build(), llm.Options{Temperature: 0.1})
	if err != nil {
		return nil, err
	}

	call, ok := mcp.ParseToolCall(resp)
	if !ok {
		// Fallback: best-effort parse from tool hint
		return &mcp.ToolCall{
			Name:   mcp.ToolKind(step.ToolHint),
			Params: json.RawMessage(`{}`),
		}, nil
	}
	return call, nil
}

// searchContext queries the FTS index for snippets relevant to a query.
func (a *Agent) searchContext(query string) []llm.CodeSnippet {
	results, err := a.db.Search(query, 5)
	if err != nil || len(results) == 0 {
		return nil
	}
	snippets := make([]llm.CodeSnippet, 0, len(results))
	for _, r := range results {
		lang := extToLang(r.FilePath)
		snippets = append(snippets, llm.CodeSnippet{
			FilePath:     r.FilePath,
			FunctionName: r.Name,
			Language:     lang,
			Content:      r.Body,
			Score:        r.Score,
		})
	}
	return snippets
}

func (a *Agent) emit(e Event) {
	select {
	case a.eventCh <- e:
	default:
	}
}

// Cfg returns the agent's configuration.
func (a *Agent) Cfg() *config.Config { return a.cfg }

// ThermalMonitor returns the underlying thermal monitor (for TUI polling).
func (a *Agent) ThermalMonitor() *thermal.Monitor { return a.thermal }

// ─── helpers ─────────────────────────────────────────────

func boolIcon(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

func extToLang(path string) string {
	m := map[string]string{
		".go": "go", ".py": "python", ".ts": "typescript",
		".js": "javascript", ".lua": "lua", ".rs": "rust",
		".cpp": "cpp", ".c": "c", ".h": "c", ".java": "java",
	}
	for ext, lang := range m {
		if strings.HasSuffix(path, ext) {
			return lang
		}
	}
	return "text"
}
