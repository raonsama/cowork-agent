// Package llm provides a thin client for the Ollama /api/chat endpoint,
// supporting both streaming and synchronous completion modes.
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

// Message represents a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GenerateRequest is the Ollama /api/chat request body.
type GenerateRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Options  Options   `json:"options,omitempty"`
}

// Options holds Ollama model parameters.
type Options struct {
	NumCtx      int      `json:"num_ctx,omitempty"`
	Temperature float64  `json:"temperature,omitempty"`
	TopP        float64  `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// StreamChunk is a single SSE chunk from Ollama.
type StreamChunk struct {
	Model   string  `json:"model"`
	Message Message `json:"message"`
	Done    bool    `json:"done"`
}

// Client is a thin Ollama API client.
type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// stripThink removes <think>…</think> blocks that reasoning models
// (e.g. qwen3, deepseek-r1) emit before their actual response.
// It works on a per-chunk basis using a small state machine so it is
// safe to call on every streaming token without buffering the full reply.
type thinkStripper struct {
	buf     strings.Builder
	inside  bool   // currently inside a <think> block
	pending string // partial tag accumulator
}

func newThinkStripper() *thinkStripper { return &thinkStripper{} }

const (
	tagOpen  = "<think>"
	tagClose = "</think>"
)

// Feed accepts one raw token and returns the cleaned output (may be empty
// if the token is part of a think block).
func (s *thinkStripper) Feed(token string) string {
	s.pending += token
	s.buf.Reset()

	for len(s.pending) > 0 {
		if s.inside {
			// Looking for </think>
			idx := strings.Index(s.pending, tagClose)
			if idx >= 0 {
				// Found closing tag — discard everything up to and including it.
				s.pending = s.pending[idx+len(tagClose):]
				s.inside = false
				// Skip optional single newline directly after </think>.
				s.pending = strings.TrimPrefix(s.pending, "\n")
			} else if couldBeSuffix(s.pending, tagClose) {
				// Partial closing tag at end — wait for more tokens.
				break
			} else {
				// No closing tag anywhere — discard the whole buffer.
				s.pending = ""
			}
		} else {
			// Looking for <think>
			idx := strings.Index(s.pending, tagOpen)
			if idx >= 0 {
				// Emit everything before the opening tag.
				s.buf.WriteString(s.pending[:idx])
				s.pending = s.pending[idx+len(tagOpen):]
				s.inside = true
			} else if couldBeSuffix(s.pending, tagOpen) {
				// Partial opening tag at end — emit safe prefix, hold the rest.
				safe := len(s.pending) - (len(tagOpen) - 1)
				if safe > 0 {
					s.buf.WriteString(s.pending[:safe])
					s.pending = s.pending[safe:]
				}
				break
			} else {
				// No tag — emit everything.
				s.buf.WriteString(s.pending)
				s.pending = ""
			}
		}
	}

	return s.buf.String()
}

// Flush emits any remaining buffered content (call after stream ends).
func (s *thinkStripper) Flush() string {
	if s.inside {
		// Unclosed <think> — discard.
		s.pending = ""
		s.inside = false
		return ""
	}
	out := s.pending
	s.pending = ""
	return out
}

// couldBeSuffix returns true if s ends with a non-empty prefix of tag,
// meaning we might be in the middle of receiving that tag.
func couldBeSuffix(s, tag string) bool {
	for l := len(tag) - 1; l > 0; l-- {
		if strings.HasSuffix(s, tag[:l]) {
			return true
		}
	}
	return false
}

// NewClient constructs a new Ollama client.
func NewClient(baseURL, model string) *Client {
	return &Client{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// Chat sends a conversation and streams tokens via the returned channel.
// The channel is closed when done or if an error occurs (sent as a sentinel message).
func (c *Client) Chat(ctx context.Context, messages []Message, opts Options) (<-chan string, <-chan error) {
	tokenCh := make(chan string, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(tokenCh)
		defer close(errCh)

		req := GenerateRequest{
			Model:    c.model,
			Messages: messages,
			Stream:   true,
			Options:  opts,
		}

		body, err := json.Marshal(req)
		if err != nil {
			errCh <- fmt.Errorf("marshal request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/api/chat", bytes.NewReader(body))
		if err != nil {
			errCh <- fmt.Errorf("create request: %w", err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("ollama request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errCh <- fmt.Errorf("ollama returned status %d", resp.StatusCode)
			return
		}

		stripper := newThinkStripper()
		scanner := bufio.NewScanner(resp.Body)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
			}

			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chunk StreamChunk
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}

			if chunk.Message.Content != "" {
				if cleaned := stripper.Feed(chunk.Message.Content); cleaned != "" {
					tokenCh <- cleaned
				}
			}

			if chunk.Done {
				// Flush any held partial content.
				if tail := stripper.Flush(); tail != "" {
					tokenCh <- tail
				}
				return
			}
		}

		// Stream ended without Done=true — flush anyway.
		if tail := stripper.Flush(); tail != "" {
			tokenCh <- tail
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("scanner error: %w", err)
		}
	}()

	return tokenCh, errCh
}

// ChatSync sends a conversation and returns the full response (non-streaming).
func (c *Client) ChatSync(ctx context.Context, messages []Message, opts Options) (string, error) {
	tokenCh, errCh := c.Chat(ctx, messages, opts)
	var result string
	for token := range tokenCh {
		result += token
	}
	if err := <-errCh; err != nil {
		return result, err
	}
	return result, nil
}

// Ping checks if Ollama is reachable.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama ping failed: status %d", resp.StatusCode)
	}
	return nil
}
