// Package agent implements the autonomous cowork engine, orchestrating
// LLM-driven planning, MCP tool execution, and result verification.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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

// Phase labels emitted in Event so the TUI can render live phase badges.
type Phase string

const (
	PhaseCommit   Phase = "committing"
	PhaseDone     Phase = "done"
	PhaseError    Phase = "error"
	PhaseExecute  Phase = "executing"
	PhaseIdle     Phase = "idle"
	PhasePlan     Phase = "planning"
	PhaseSearch   Phase = "searching"
	PhaseThrottle Phase = "throttled"
	PhaseVerify   Phase = "verifying"
)

// Event is broadcast on the event channel so the TUI can render live status.
type Event struct {
	Branch     string
	Err        error
	Final      *reporter.Report
	Message    string
	Phase      Phase
	StepDesc   string
	StepID     int
	ToolName   string
	ToolOutput string
}

// Agent is the autonomous cowork engine.
type Agent struct {
	cfg      *config.Config
	client   *llm.Client
	db       *indexer.DB
	eventCh  chan Event
	mcpSrv   *mcp.Server
	mu       sync.Mutex // guards eventCh between PrepareRun/emit
	planner  *Planner
	termux   *termux.Bridge
	thermal  *thermal.Monitor
	verifier *Verifier
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
		db:       db,
		eventCh:  make(chan Event, 64),
		mcpSrv:   mcp.NewServer(cfg.ProjectRoot),
		planner:  NewPlanner(client, cfg.ContextWindow),
		termux:   termux.NewBridge(cfg.TermuxNotifyEnabled),
		thermal:  thermal.NewMonitor(cfg.ThermalThresholdCelsius, cfg.CPUThrottlePercent),
		verifier: NewVerifier(client, cfg.ContextWindow),
	}, nil
}

// Cfg returns the agent configuration (read-only).
func (a *Agent) Cfg() *config.Config { return a.cfg }

// Close releases database and thermal monitor resources.
func (a *Agent) Close() {
	a.db.Close()
	a.thermal.Stop()
}

// Events returns the channel on which the agent broadcasts state events.
func (a *Agent) Events() <-chan Event { return a.eventCh }

// ThermalMonitor exposes the thermal monitor for TUI polling.
func (a *Agent) ThermalMonitor() *thermal.Monitor { return a.thermal }

// PrepareRun resets planner/verifier state and opens a fresh event channel.
// Must be called before each Run invocation. Thread-safe.
func (a *Agent) PrepareRun() <-chan Event {
	a.mu.Lock()
	a.eventCh = make(chan Event, 64)
	ch := a.eventCh
	a.mu.Unlock()

	a.planner.ctx.Reset()
	a.verifier.ctx.Reset()
	return ch
}

// Run executes the full cowork loop for the given task.
// Designed to run in its own goroutine; the TUI consumes events via Events().
func (a *Agent) Run(ctx context.Context, task string) {
	a.mu.Lock()
	ch := a.eventCh
	a.mu.Unlock()
	defer close(ch)

	a.thermal.Start()
	startTime := time.Now()

	if err := a.client.Ping(ctx); err != nil {
		a.emit(Event{Phase: PhaseError, Err: fmt.Errorf("ollama unreachable: %w", err)})
		return
	}

	// Conversational inputs skip the tool pipeline entirely.
	if isConversational(task) {
		a.runChat(ctx, task)
		return
	}

	// Isolate agent work on a shadow branch.
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

	a.emit(Event{Phase: PhasePlan, Message: "Decomposing task into steps…"})

	plan, err := a.planner.CreatePlan(ctx, task)
	if err != nil {
		a.emit(Event{Phase: PhaseError, Err: err})
		return
	}
	a.emit(Event{Phase: PhasePlan, Message: fmt.Sprintf("Plan ready: %d steps — %s", len(plan.Steps), plan.Summary)})

	var stepLog []reporter.StepLog
	const maxRetries = 2

	for i := range plan.Steps {
		step := &plan.Steps[i]
		if step.Done {
			continue
		}

		if a.thermal.IsThrottled() {
			st := a.thermal.Current()
			a.emit(Event{Phase: PhaseThrottle, Message: st.ThrottleMsg})
			a.thermal.WaitIfThrottled(ctx.Done())
		}

		a.emit(Event{Phase: PhaseSearch, StepID: step.ID, StepDesc: step.Description, Message: "Searching relevant code context…"})
		contextBlock := a.buildContextBlock(step.Description)

		var (
			toolResult mcp.ToolResult
			verdict    Verdict
			retries    int
		)

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

			a.emit(Event{Phase: PhaseVerify, StepID: step.ID, Message: "Verifying result…"})
			verdict, _ = a.verifier.Verify(ctx, *step, toolResult.Output+toolResult.Error)

			if verdict.Passed || retries >= maxRetries {
				break
			}

			retries++
			a.emit(Event{
				Phase: PhaseVerify, StepID: step.ID,
				Message: fmt.Sprintf("⚠️  Retry %d/%d — %s", retries, maxRetries, verdict.Reason),
			})
			if retries == maxRetries {
				plan, _ = a.planner.RefinePlan(ctx, plan, *step, verdict.Reason)
			}

			a.emit(Event{Phase: PhaseExecute, StepID: step.ID, ToolName: string(toolCall.Name), ToolOutput: toolResult.Output})
		}

		step.Done = true
		step.Result = toolResult.Output

		stepLog = append(stepLog, reporter.StepLog{
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

	a.emit(Event{Phase: PhaseCommit, Message: "Committing changes to shadow branch…"})
	diff, _ := ws.Diff()
	stat, _ := ws.Stat()
	_ = ws.Commit(plan.Summary)

	elapsed := time.Since(startTime)
	rpt := &reporter.Report{
		Task:        task,
		Branch:      ws.BranchName(),
		BaseBranch:  ws.BaseBranch(),
		Steps:       stepLog,
		Diff:        diff,
		DiffStat:    stat,
		ElapsedSecs: int(elapsed.Seconds()),
		Timestamp:   time.Now(),
	}

	a.emit(Event{Phase: PhaseDone, Final: rpt, Message: "Tugas selesai. Mimpimu sudah aman di kode ini, silakan lanjut rebahan. 🌙"})
	_ = a.termux.NotifyTaskComplete(task, rpt.OneLiner())
	a.termux.Vibrate(300)
}

// runChat handles conversational inputs, streaming tokens without tool calls.
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

// isConversational returns true for inputs that are greetings, short messages,
// or questions without imperative coding verbs.
func isConversational(input string) bool {
	t := strings.TrimSpace(strings.ToLower(input))

	if len([]rune(t)) < 20 {
		return true
	}

	for _, prefix := range []string{
		"apa ", "siapa ", "kapan ", "kenapa ", "mengapa ", "bagaimana ",
		"what ", "who ", "when ", "why ", "how ", "is ", "are ", "can ",
		"could ", "would ", "should ", "do you ", "did you ",
	} {
		if strings.HasPrefix(t, prefix) {
			return true
		}
	}

	for _, g := range []string{
		"hai", "hi", "hello", "halo", "hey", "hei",
		"selamat", "pagi", "siang", "sore", "malam",
		"thanks", "thank you", "terima kasih", "makasih",
		"oke", "ok", "mantap", "keren", "good",
	} {
		if t == g || strings.HasPrefix(t, g+" ") || strings.HasSuffix(t, " "+g) {
			return true
		}
	}

	// Explicit task verbs override all conversational signals.
	for _, v := range []string{
		"buat ", "create ", "add ", "fix ", "refactor ", "implement ",
		"write ", "update ", "delete ", "remove ", "migrate ", "deploy ",
		"run ", "build ", "test ", "generate ", "parse ",
		"tambah ", "perbaiki ", "ubah ", "hapus ", "jalankan ",
	} {
		if strings.Contains(t, v) {
			return false
		}
	}

	return false
}

// resolveToolCall asks the LLM to select the correct MCP tool call for a step.
func (a *Agent) resolveToolCall(ctx context.Context, plan *Plan, step *Step, codeCtx string) (*mcp.ToolCall, error) {
	toolDefs := a.mcpSrv.FormatToolDefsForPrompt()
	planJSON, _ := json.MarshalIndent(plan, "", "  ")

	prompt := strings.Join([]string{
		toolDefs,
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

	system := `You are an autonomous coding agent executing a task plan.
For each step, respond with exactly one tool call inside <tool_call> tags.
Do not add any other text.`

	cm := llm.NewContextManager(a.cfg.ContextWindow, system)
	cm.AddMessage("user", prompt)

	resp, err := a.client.ChatSync(ctx, cm.Build(), llm.Options{Temperature: 0.1})
	if err != nil {
		return nil, err
	}

	if call, ok := mcp.ParseToolCall(resp); ok {
		return call, nil
	}
	// Fallback: honour the planner's tool hint with empty params.
	return &mcp.ToolCall{
		Name:   mcp.ToolKind(step.ToolHint),
		Params: json.RawMessage(`{}`),
	}, nil
}

// buildContextBlock fetches FTS snippets for a step and formats them for the prompt.
func (a *Agent) buildContextBlock(query string) string {
	snippets := a.searchContext(query)
	if len(snippets) == 0 {
		return ""
	}
	cm := llm.NewContextManager(a.cfg.ContextWindow, "")
	return cm.InjectContext(snippets)
}

// searchContext queries the FTS index for code snippets relevant to query.
func (a *Agent) searchContext(query string) []llm.CodeSnippet {
	results, err := a.db.Search(query, 5)
	if err != nil || len(results) == 0 {
		return nil
	}
	snippets := make([]llm.CodeSnippet, 0, len(results))
	for _, r := range results {
		snippets = append(snippets, llm.CodeSnippet{
			FilePath:     r.FilePath,
			FunctionName: r.Name,
			Language:     extToLang(r.FilePath),
			Content:      r.Body,
			Score:        r.Score,
		})
	}
	return snippets
}

// emit sends an event non-blocking, using the mutex-guarded channel.
func (a *Agent) emit(e Event) {
	a.mu.Lock()
	ch := a.eventCh
	a.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- e:
	default:
	}
}
