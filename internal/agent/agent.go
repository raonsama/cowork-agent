// Package agent implements the autonomous cowork engine, orchestrating
// LLM-driven planning, MCP tool execution, and result verification.
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

func (a *Agent) PrepareRun() <-chan Event {
	a.eventCh = make(chan Event, 64)
	a.planner.ctx.Reset()
	a.verifier.ctx.Reset()
	return a.eventCh
}

// Run executes the full cowork loop for a given task description.
// It is designed to run in its own goroutine; the TUI consumes Events().
func (a *Agent) Run(ctx context.Context, task string) {
	defer close(a.eventCh)

	a.thermal.Start()
	startTime := time.Now()

	// ── Ping Ollama ───────────────────────────────────────
	if err := a.client.Ping(ctx); err != nil {
		a.emit(Event{Phase: PhaseError, Err: fmt.Errorf("ollama unreachable: %w", err)})
		return
	}

	// ── Intent detection ──────────────────────────────────
	// Conversational inputs (greetings, questions, short chitchat) are
	// answered directly via LLM chat — no tool pipeline, no shadow branch.
	if isConversational(task) {
		a.runChat(ctx, task)
		return
	}

	// ── Shadow workspace ──────────────────────────────────
	ws := shadow.NewWorkspace(a.cfg.ProjectRoot, a.cfg.BranchPrefix)
	if err := ws.Begin(task); err != nil {
		a.emit(Event{Phase: PhasePlan, Message: "⚠️  Shadow workspace unavailable — working on current branch"})
	} else {
		defer func() {
			if ctx.Err() != nil {
				_ = ws.Abort()
			}
		}()
		a.emit(Event{Phase: PhasePlan, Branch: ws.BranchName(), Message: "Shadow branch: " + ws.BranchName()})
	}

	// ── Plan ──────────────────────────────────────────────
	a.emit(Event{Phase: PhasePlan, Message: "Decomposing task into steps…"})

	var plan *Plan
	if a.cfg.PlannerEnabled {
		var err error
		plan, err = a.planner.CreatePlan(ctx, task)
		if err != nil {
			a.emit(Event{Phase: PhaseError, Err: err})
			return
		}
	} else {
		plan = &Plan{
			Task:    task,
			Steps:   []Step{{ID: 1, Description: task, ToolHint: "run_shell"}},
			Summary: task,
		}
	}
	a.emit(Event{Phase: PhasePlan, Message: fmt.Sprintf("Plan ready: %d steps — %s", len(plan.Steps), plan.Summary)})

	// ── Execution loop ────────────────────────────────────
	log := []reporter.StepLog{}
	maxRetries := 2

	for i := range plan.Steps {
		step := &plan.Steps[i]
		if step.Done {
			continue
		}

		if a.thermal.IsThrottled() {
			status := a.thermal.Current()
			a.emit(Event{Phase: PhaseThrottle, Message: status.ThrottleMsg})
			a.thermal.WaitIfThrottled(ctx.Done())
		}

		// Context search hanya jalan jika planner aktif — tanpa planner
		// tidak ada cukup sinyal untuk query FTS yang bermakna.
		contextBlock := ""
		if a.cfg.PlannerEnabled {
			a.emit(Event{
				Phase: PhaseSearch, StepID: step.ID, StepDesc: step.Description,
				Message: "Searching relevant code context…",
			})
			snippets := a.searchContext(step.Description)
			if len(snippets) > 0 {
				cm := llm.NewContextManager(a.cfg.ContextWindow, "")
				contextBlock = cm.InjectContext(snippets)
			}
		}

		var toolResult mcp.ToolResult
		var verdict Verdict
		retries := 0

		for retries <= maxRetries {
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

			// ── Verify (toggle-aware) ─────────────────────
			a.emit(Event{Phase: PhaseVerify, StepID: step.ID, Message: "Verifying result…"})
			if a.cfg.VerifierEnabled {
				verdict, _ = a.verifier.Verify(ctx, *step, toolResult.Output+toolResult.Error)
			} else {
				verdict = heuristicVerdict(toolResult.Output + toolResult.Error)
			}

			if verdict.Passed || retries >= maxRetries {
				break
			}

			retries++
			a.emit(Event{
				Phase:   PhaseVerify,
				StepID:  step.ID,
				Message: fmt.Sprintf("⚠️  Retry %d/%d — %s", retries, maxRetries, verdict.Reason),
			})

			if a.cfg.PlannerEnabled && retries == maxRetries {
				plan, _ = a.planner.RefinePlan(ctx, plan, *step, verdict.Reason)
			}

			a.emit(Event{
				Phase:      PhaseExecute,
				StepID:     step.ID,
				ToolName:   string(toolCall.Name),
				ToolOutput: toolResult.Output,
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

	// ── Commit ────────────────────────────────────────────
	a.emit(Event{Phase: PhaseCommit, Message: "Committing changes to shadow branch…"})
	diff, _ := ws.Diff()
	stat, _ := ws.Stat()
	_ = ws.Commit(plan.Summary)

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

	_ = a.termux.NotifyTaskComplete(task, rpt.OneLiner())
	a.termux.Vibrate(300)
}

// runChat handles conversational inputs that do not require tool execution.
// It streams the LLM response token-by-token as a single assistant message.
func (a *Agent) runChat(ctx context.Context, input string) {
	system := `You are CoworkAgent, a senior software engineer assistant.
Answer the user's question or message naturally and concisely.
Do not use tool calls. Do not produce JSON plans.`

	cm := llm.NewContextManager(a.cfg.ContextWindow, system)
	cm.AddMessage("user", input)

	tokenCh, errCh := a.client.Chat(ctx, cm.Build(), llm.Options{Temperature: 0.7})

	var full strings.Builder
	for token := range tokenCh {
		full.WriteString(token)
		a.emit(Event{Phase: PhaseIdle, Message: token})
	}

	if err := <-errCh; err != nil && ctx.Err() == nil {
		a.emit(Event{Phase: PhaseError, Err: err})
		return
	}

	a.emit(Event{Phase: PhaseDone, Message: full.String()})
}

// isConversational returns true for inputs that are clearly not coding tasks:
// short messages, greetings, questions without imperative action words, etc.
func isConversational(input string) bool {
	t := strings.TrimSpace(strings.ToLower(input))

	// Very short inputs are almost never tasks.
	if len([]rune(t)) < 20 {
		return true
	}

	// Starts with a question word → conversational.
	questionPrefixes := []string{
		"apa ", "siapa ", "kapan ", "kenapa ", "mengapa ", "bagaimana ",
		"what ", "who ", "when ", "why ", "how ", "is ", "are ", "can ",
		"could ", "would ", "should ", "do you ", "did you ",
	}
	for _, p := range questionPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}

	// Greetings / chitchat keywords.
	greetings := []string{
		"hai", "hi", "hello", "halo", "hey", "hei",
		"selamat", "pagi", "siang", "sore", "malam",
		"thanks", "thank you", "terima kasih", "makasih",
		"oke", "ok", "oke deh", "mantap", "keren", "good",
	}
	for _, g := range greetings {
		if t == g || strings.HasPrefix(t, g+" ") || strings.HasSuffix(t, " "+g) {
			return true
		}
	}

	// Contains explicit task verbs → not conversational.
	taskVerbs := []string{
		"buat ", "create ", "add ", "fix ", "refactor ", "implement ",
		"write ", "update ", "delete ", "remove ", "migrate ", "deploy ",
		"run ", "build ", "test ", "generate ", "parse ", "tambah ",
		"perbaiki ", "ubah ", "hapus ", "jalankan ",
	}
	for _, v := range taskVerbs {
		if strings.Contains(t, v) {
			return false
		}
	}

	return false
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
