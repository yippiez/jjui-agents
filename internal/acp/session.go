package acp

import (
	"context"
	"sync"
)

// Session represents an active ACP session with an agent.
type Session struct {
	ID     string
	client *Client

	mu       sync.Mutex
	active   bool
	cancelFn context.CancelFunc
	updates  chan SessionUpdateNotification
}

func newSession(id string, c *Client) *Session {
	return &Session{
		ID:      id,
		client:  c,
		updates: make(chan SessionUpdateNotification, 64),
	}
}

// Prompt sends a text prompt to the agent in this session and returns a
// channel that streams SessionUpdateNotification values until the prompt
// completes. The channel is closed when the agent finishes or the context
// is cancelled.
func (s *Session) Prompt(ctx context.Context, content string) (<-chan SessionUpdateNotification, error) {
	return s.PromptWithBlocks(ctx, []ContentBlock{TextContent(content)})
}

// PromptWithBlocks sends a multi-block prompt (text, images, resource links).
func (s *Session) PromptWithBlocks(ctx context.Context, blocks []ContentBlock) (<-chan SessionUpdateNotification, error) {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return nil, &RPCError{Code: ErrCodeInternal, Message: "a prompt is already active on this session"}
	}
	s.active = true
	promptCtx, cancel := context.WithCancel(ctx)
	s.cancelFn = cancel
	s.mu.Unlock()

	out := make(chan SessionUpdateNotification, 64)

	go func() {
		defer func() {
			close(out)
			s.mu.Lock()
			s.active = false
			s.cancelFn = nil
			s.mu.Unlock()
		}()

		result, err := s.client.sendPrompt(promptCtx, s.ID, blocks, out)
		if err != nil {
			return
		}
		_ = result
	}()

	return out, nil
}

// Cancel cancels the in-flight prompt on this session.
func (s *Session) Cancel() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancelFn != nil {
		s.cancelFn()
	}
	return s.client.sendCancel(s.ID)
}

// IsActive returns true if a prompt is currently in-flight.
func (s *Session) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// Close closes this session with the agent.
func (s *Session) Close(ctx context.Context) error {
	return s.client.closeSession(ctx, s.ID)
}

// deliverUpdate routes a session update notification to the session's channel.
func (s *Session) deliverUpdate(update SessionUpdateNotification) {
	select {
	case s.updates <- update:
	default:
	}
}
