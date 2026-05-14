package llm

import (
	"strings"
	"unicode/utf8"
)

// ContextManager implements a sliding-window context strategy
// optimized for small context windows (e.g., 4k tokens on mobile LLMs).
type ContextManager struct {
	maxTokens    int
	systemPrompt string
	messages     []Message
	tokenCache   map[string]int
}

// NewContextManager creates a context manager with a hard token ceiling.
func NewContextManager(maxTokens int, systemPrompt string) *ContextManager {
	return &ContextManager{
		maxTokens:    maxTokens,
		systemPrompt: systemPrompt,
		tokenCache:   make(map[string]int),
	}
}

// estimateTokens approximates token count (1 token ≈ 4 chars for most LLMs).
func estimateTokens(s string) int {
	chars := utf8.RuneCountInString(s)
	return (chars / 4) + 1
}

// systemTokens returns the token count of the system prompt.
func (cm *ContextManager) systemTokens() int {
	return estimateTokens(cm.systemPrompt)
}

// AddMessage appends a user or assistant message and prunes if necessary.
func (cm *ContextManager) AddMessage(role, content string) {
	cm.messages = append(cm.messages, Message{Role: role, Content: content})
	cm.prune()
}

// prune removes the oldest non-system messages until we fit within the window.
// It always keeps at least the last 2 messages (human + assistant pair) intact.
func (cm *ContextManager) prune() {
	for {
		total := cm.systemTokens()
		for _, m := range cm.messages {
			total += estimateTokens(m.Content) + 4 // 4 for role overhead
		}

		// Reserve 20% headroom for the next response
		budget := int(float64(cm.maxTokens) * 0.80)
		if total <= budget || len(cm.messages) <= 2 {
			break
		}

		// Drop the oldest message
		cm.messages = cm.messages[1:]
	}
}

// Build returns the complete message slice ready for the API call.
// The system prompt is prepended as the first message.
func (cm *ContextManager) Build() []Message {
	result := make([]Message, 0, len(cm.messages)+1)
	if cm.systemPrompt != "" {
		result = append(result, Message{Role: "system", Content: cm.systemPrompt})
	}
	result = append(result, cm.messages...)
	return result
}

// InjectContext prepends a context block (e.g., retrieved code snippets)
// into the next user message. Call this before AddMessage("user", ...).
func (cm *ContextManager) InjectContext(snippets []CodeSnippet) string {
	if len(snippets) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### Relevant Code Context\n\n")
	for _, s := range snippets {
		sb.WriteString("```")
		sb.WriteString(s.Language)
		sb.WriteString("\n// ")
		sb.WriteString(s.FilePath)
		if s.FunctionName != "" {
			sb.WriteString(" — ")
			sb.WriteString(s.FunctionName)
		}
		sb.WriteString("\n")
		sb.WriteString(s.Content)
		sb.WriteString("\n```\n\n")
	}
	return sb.String()
}

// CodeSnippet is a retrieved code fragment from the indexer.
type CodeSnippet struct {
	FilePath     string
	FunctionName string
	Language     string
	Content      string
	Score        float64
}

// Reset clears all messages but keeps the system prompt.
func (cm *ContextManager) Reset() {
	cm.messages = nil
}

// MessageCount returns the number of stored messages.
func (cm *ContextManager) MessageCount() int {
	return len(cm.messages)
}

// TokenUsage returns approximate total token count in use.
func (cm *ContextManager) TokenUsage() int {
	total := cm.systemTokens()
	for _, m := range cm.messages {
		total += estimateTokens(m.Content) + 4
	}
	return total
}

// TokenBudget returns remaining token budget.
func (cm *ContextManager) TokenBudget() int {
	return cm.maxTokens - cm.TokenUsage()
}
