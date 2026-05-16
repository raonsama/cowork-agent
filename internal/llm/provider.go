// Package llm defines the Provider interface shared by all LLM backends,
// plus an OpenAI-compatible client for cloud providers (OpenAI, DeepSeek,
// Groq, Anthropic-proxy, etc.) and the existing Ollama client.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ── Provider interface ────────────────────────────────────────────────────────

// Provider is the unified interface for streaming LLM backends.
// Both OllamaClient (local) and OpenAIClient (cloud) implement this.
type Provider interface {
	// Chat sends messages and streams tokens via the returned channels.
	Chat(ctx context.Context, messages []Message, opts Options) (<-chan string, <-chan error)
	// ChatSync blocks until the full response is available.
	ChatSync(ctx context.Context, messages []Message, opts Options) (string, error)
	// Ping checks reachability; returns nil if the backend is up.
	Ping(ctx context.Context) error
	// ModelName returns the currently active model identifier.
	ModelName() string
	// ProviderName returns a human-readable backend name ("ollama", "openai", …).
	ProviderName() string
}

// ProviderConfig holds init parameters for any provider.
type ProviderConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// NewProvider constructs the correct Provider from a config.
// kind must be "ollama" or "openai" (covers all OpenAI-compatible APIs).
func NewProvider(kind string, cfg ProviderConfig) (Provider, error) {
	switch kind {
	case "ollama", "":
		return NewOllamaProvider(cfg.BaseURL, cfg.Model), nil
	case "openai":
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.openai.com"
		}
		return NewOpenAIProvider(cfg.BaseURL, cfg.APIKey, cfg.Model), nil
	default:
		return nil, fmt.Errorf("unknown provider kind %q", kind)
	}
}

// ── OllamaProvider ───────────────────────────────────────────────────────────

// OllamaProvider wraps the existing Ollama client as a Provider.
type OllamaProvider struct {
	*Client // embeds the original Client
}

func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	return &OllamaProvider{Client: NewClient(baseURL, model)}
}

func (o *OllamaProvider) ModelName() string    { return o.model }
func (o *OllamaProvider) ProviderName() string { return "ollama" }

// ── OpenAI-compatible Provider ────────────────────────────────────────────────

// openAIMsg is the OpenAI chat message format.
type openAIMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model    string      `json:"model"`
	Messages []openAIMsg `json:"messages"`
	Stream   bool        `json:"stream"`
	// Options
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
}

// openAIDelta is one SSE chunk from /v1/chat/completions (stream mode).
type openAIDelta struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// OpenAIProvider implements Provider against any OpenAI-compatible /v1/chat/completions endpoint.
// Works with: OpenAI, DeepSeek, Groq, Together, Anthropic-proxy, etc.
type OpenAIProvider struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenAIProvider creates an OpenAI-compatible provider.
//
//	baseURL: e.g. "https://api.openai.com" or "https://api.groq.com/openai"
//	apiKey: Bearer token
//	model: e.g. "gpt-4o", "deepseek-chat", "llama-3.3-70b-versatile"
func NewOpenAIProvider(baseURL, apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (p *OpenAIProvider) ModelName() string    { return p.model }
func (p *OpenAIProvider) ProviderName() string { return "openai" }

// Chat streams tokens from /v1/chat/completions with SSE.
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, opts Options) (<-chan string, <-chan error) {
	tokenCh := make(chan string, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(tokenCh)
		defer close(errCh)

		msgs := make([]openAIMsg, len(messages))
		for i, m := range messages {
			msgs[i] = openAIMsg{Role: m.Role, Content: m.Content}
		}

		var temp *float64
		if opts.Temperature != 0 {
			t := opts.Temperature
			temp = &t
		}

		req := openAIRequest{
			Model:       p.model,
			Messages:    msgs,
			Stream:      true,
			Temperature: temp,
			MaxTokens:   opts.NumCtx,
		}
		if req.MaxTokens == 0 {
			req.MaxTokens = 4096
		}

		body, err := json.Marshal(req)
		if err != nil {
			errCh <- err
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			errCh <- err
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

		resp, err := p.httpClient.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("openai request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errCh <- fmt.Errorf("openai status %d", resp.StatusCode)
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
			}

			line := strings.TrimPrefix(scanner.Text(), "data: ")
			if line == "" || line == "[DONE]" {
				continue
			}

			var chunk openAIDelta
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) > 0 {
				if tok := chunk.Choices[0].Delta.Content; tok != "" {
					tokenCh <- tok
				}
				if chunk.Choices[0].FinishReason != nil {
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("scanner: %w", err)
		}
	}()

	return tokenCh, errCh
}

// ChatSync collects the full streamed response.
func (p *OpenAIProvider) ChatSync(ctx context.Context, messages []Message, opts Options) (string, error) {
	tokenCh, errCh := p.Chat(ctx, messages, opts)
	var sb strings.Builder
	for tok := range tokenCh {
		sb.WriteString(tok)
	}
	if err := <-errCh; err != nil {
		return sb.String(), err
	}
	return sb.String(), nil
}

// Ping checks the /v1/models endpoint for reachability.
func (p *OpenAIProvider) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.baseURL+"/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("openai ping: status %d", resp.StatusCode)
	}
	return nil
}
