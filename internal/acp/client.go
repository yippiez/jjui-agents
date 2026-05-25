package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// State represents the lifecycle state of the client connection.
type State int

const (
	Disconnected State = iota
	Starting
	Initializing
	Ready
	Closed
)

func (s State) String() string {
	switch s {
	case Disconnected:
		return "disconnected"
	case Starting:
		return "starting"
	case Initializing:
		return "initializing"
	case Ready:
		return "ready"
	case Closed:
		return "closed"
	default:
		return "unknown"
	}
}

// Client is an ACP client that spawns an agent subprocess and
// communicates with it over JSON-RPC 2.0 on stdin/stdout.
type Client struct {
	config  AdapterConfig
	workDir string
	handler ToolHandler

	cmd       *exec.Cmd
	transport *Transport
	agentInfo *InitializeResponse

	mu       sync.Mutex
	sessions map[string]*Session
	state    State

	onStateChange func(State)
	onUpdate      func(SessionUpdateNotification)

	stopOnce sync.Once
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewClient creates an ACP client. The handler is called when the agent
// makes tool call requests back to the client. Pass NoopToolHandler{} if
// you don't need tool call support yet.
func NewClient(cfg AdapterConfig, workDir string, handler ToolHandler) *Client {
	if handler == nil {
		handler = NoopToolHandler{}
	}
	return &Client{
		config:   cfg,
		workDir:  workDir,
		handler:  handler,
		sessions: make(map[string]*Session),
		state:    Disconnected,
	}
}

// SetOnStateChange registers a callback invoked when the client state changes.
func (c *Client) SetOnStateChange(fn func(State)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onStateChange = fn
}

// SetOnUpdate registers a callback invoked for every session update
// notification from the agent.
func (c *Client) SetOnUpdate(fn func(SessionUpdateNotification)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onUpdate = fn
}

// State returns the current connection state.
func (c *Client) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// AgentInfo returns the agent's initialize response, or nil if not yet initialized.
func (c *Client) AgentInfo() *InitializeResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentInfo
}

// Start spawns the agent subprocess and starts the transport.
func (c *Client) Start(ctx context.Context) error {
	c.setState(Starting)

	c.ctx, c.cancel = context.WithCancel(ctx)

	if len(c.config.Command) == 0 {
		return fmt.Errorf("adapter config has no command")
	}

	c.cmd = exec.CommandContext(c.ctx, c.config.Command[0], c.config.Command[1:]...)
	c.cmd.Dir = c.workDir
	if len(c.config.Env) > 0 {
		c.cmd.Env = append(os.Environ(), c.config.Env...)
	}
	c.cmd.Stderr = os.Stderr

	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		c.setState(Disconnected)
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		c.setState(Disconnected)
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		c.setState(Disconnected)
		return fmt.Errorf("start agent: %w", err)
	}

	c.transport = NewTransport(stdout, stdin)
	go c.transport.Run()
	go c.handleIncoming()

	return nil
}

// Initialize performs the ACP initialize handshake with the agent.
func (c *Client) Initialize(ctx context.Context) (*InitializeResponse, error) {
	c.setState(Initializing)

	req := InitializeRequest{
		ProtocolVersion: "1.0",
		ClientInfo: ClientInfo{
			Name:    "jjui",
			Version: "0.1.0",
		},
		ClientCapabilities: ClientCapabilities{
			Fs: FileSystemCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
	}

	if c.workDir != "" {
		req.WorkspaceRoots = []WorkspaceRoot{{URI: "file://" + c.workDir}}
	}

	raw, err := c.transport.SendRequest(ctx, MethodInitialize, req)
	if err != nil {
		c.setState(Disconnected)
		return nil, fmt.Errorf("initialize: %w", err)
	}

	var resp InitializeResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		c.setState(Disconnected)
		return nil, fmt.Errorf("unmarshal initialize response: %w", err)
	}

	c.mu.Lock()
	c.agentInfo = &resp
	c.mu.Unlock()

	c.setState(Ready)
	return &resp, nil
}

// NewSession creates a new agent session.
func (c *Client) NewSession(ctx context.Context) (*Session, error) {
	req := NewSessionRequest{
		Cwd: c.workDir,
	}

	raw, err := c.transport.SendRequest(ctx, MethodNewSession, req)
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}

	var resp NewSessionResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal new session response: %w", err)
	}

	sess := newSession(resp.SessionID, c)
	c.mu.Lock()
	c.sessions[resp.SessionID] = sess
	c.mu.Unlock()

	return sess, nil
}

// LoadSession attaches to an existing session by ID.
func (c *Client) LoadSession(ctx context.Context, id string) (*Session, error) {
	req := LoadSessionRequest{SessionID: id}

	_, err := c.transport.SendRequest(ctx, MethodLoadSession, req)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	sess := newSession(id, c)
	c.mu.Lock()
	c.sessions[id] = sess
	c.mu.Unlock()

	return sess, nil
}

// ListSessions asks the agent for available sessions.
func (c *Client) ListSessions(ctx context.Context) ([]SessionInfo, error) {
	raw, err := c.transport.SendRequest(ctx, MethodListSessions, ListSessionsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var resp ListSessionsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp.Sessions, nil
}

// ResumeSession resumes a previously loaded session.
func (c *Client) ResumeSession(ctx context.Context, id string) error {
	req := ResumeSessionRequest{SessionID: id}
	_, err := c.transport.SendRequest(ctx, MethodResumeSession, req)
	return err
}

// SetSessionMode changes the agent's mode for a session.
func (c *Client) SetSessionMode(ctx context.Context, sessionID string, mode string) error {
	req := SetSessionModeRequest{SessionID: sessionID, Mode: mode}
	_, err := c.transport.SendRequest(ctx, MethodSetSessionMode, req)
	return err
}

// SetSessionConfigOption sets a config option on a session.
func (c *Client) SetSessionConfigOption(ctx context.Context, sessionID, key string, value any) error {
	req := SetSessionConfigOptionRequest{SessionID: sessionID, Key: key, Value: value}
	_, err := c.transport.SendRequest(ctx, MethodSetSessionConfig, req)
	return err
}

// Shutdown sends a graceful shutdown to the agent and stops the subprocess.
func (c *Client) Shutdown(ctx context.Context) error {
	var firstErr error
	c.stopOnce.Do(func() {
		c.mu.Lock()
		for _, sess := range c.sessions {
			_ = sess.Close(ctx)
		}
		c.sessions = make(map[string]*Session)
		c.mu.Unlock()

		if c.cancel != nil {
			c.cancel()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			firstErr = c.cmd.Wait()
		}
		c.setState(Closed)
	})
	return firstErr
}

// Close is an alias for Shutdown with a background context.
func (c *Client) Close() error {
	return c.Shutdown(context.Background())
}

// sendPrompt is the internal implementation called by Session.Prompt.
// It sends the prompt request and routes streaming updates to the output channel.
func (c *Client) sendPrompt(ctx context.Context, sessionID string, blocks []ContentBlock, out chan<- SessionUpdateNotification) (*PromptResponse, error) {
	req := PromptRequest{
		SessionID: sessionID,
		Prompt:    blocks,
	}

	raw, err := c.transport.SendRequest(ctx, MethodPrompt, req)
	if err != nil {
		return nil, err
	}

	var resp PromptResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// sendCancel sends a cancel notification for a session.
func (c *Client) sendCancel(sessionID string) error {
	return c.transport.SendNotification(MethodCancel, CancelNotification{SessionID: sessionID})
}

// closeSession closes a session with the agent.
func (c *Client) closeSession(ctx context.Context, id string) error {
	req := CloseSessionRequest{SessionID: id}
	_, err := c.transport.SendRequest(ctx, MethodCloseSession, req)

	c.mu.Lock()
	delete(c.sessions, id)
	c.mu.Unlock()

	return err
}

// handleIncoming runs in a goroutine, processing agent-initiated requests
// and routing notifications to sessions.
func (c *Client) handleIncoming() {
	for {
		select {
		case req, ok := <-c.transport.Requests:
			if !ok {
				return
			}
			go c.dispatchRequest(req)

		case notif, ok := <-c.transport.Notifications:
			if !ok {
				return
			}
			c.dispatchNotification(notif)

		case <-c.transport.Done():
			return
		}
	}
}

func (c *Client) dispatchRequest(req *rpcRequest) {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	var result any
	var rpcErr *RPCError

	switch req.Method {
	case MethodReadTextFile:
		var params ReadTextFileRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			rpcErr = &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
		} else {
			resp, err := c.handler.ReadTextFile(ctx, params)
			if err != nil {
				rpcErr = asRPCError(err)
			} else {
				result = resp
			}
		}

	case MethodWriteTextFile:
		var params WriteTextFileRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			rpcErr = &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
		} else {
			resp, err := c.handler.WriteTextFile(ctx, params)
			if err != nil {
				rpcErr = asRPCError(err)
			} else {
				result = resp
			}
		}

	case MethodCreateTerminal:
		var params CreateTerminalRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			rpcErr = &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
		} else {
			resp, err := c.handler.CreateTerminal(ctx, params)
			if err != nil {
				rpcErr = asRPCError(err)
			} else {
				result = resp
			}
		}

	case MethodKillTerminal:
		var params KillTerminalRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			rpcErr = &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
		} else {
			resp, err := c.handler.KillTerminal(ctx, params)
			if err != nil {
				rpcErr = asRPCError(err)
			} else {
				result = resp
			}
		}

	case MethodReleaseTerminal:
		var params ReleaseTerminalRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			rpcErr = &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
		} else {
			resp, err := c.handler.ReleaseTerminal(ctx, params)
			if err != nil {
				rpcErr = asRPCError(err)
			} else {
				result = resp
			}
		}

	case MethodTerminalOutput:
		var params TerminalOutputRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			rpcErr = &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
		} else {
			resp, err := c.handler.TerminalOutput(ctx, params)
			if err != nil {
				rpcErr = asRPCError(err)
			} else {
				result = resp
			}
		}

	case MethodWaitForTerminalExit:
		var params WaitForTerminalExitRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			rpcErr = &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
		} else {
			resp, err := c.handler.WaitForTerminalExit(ctx, params)
			if err != nil {
				rpcErr = asRPCError(err)
			} else {
				result = resp
			}
		}

	case MethodRequestPermission:
		var params RequestPermissionRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			rpcErr = &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
		} else {
			resp, err := c.handler.RequestPermission(ctx, params)
			if err != nil {
				rpcErr = asRPCError(err)
			} else {
				result = resp
			}
		}

	default:
		rpcErr = &RPCError{Code: ErrCodeMethodNotFound, Message: fmt.Sprintf("unknown method: %s", req.Method)}
	}

	_ = c.transport.SendResponse(req.ID, result, rpcErr)
}

func (c *Client) dispatchNotification(notif *rpcNotification) {
	if notif.Method != MethodSessionUpdate {
		return
	}

	var update SessionUpdateNotification
	if err := json.Unmarshal(notif.Params, &update); err != nil {
		return
	}

	c.mu.Lock()
	sess, ok := c.sessions[update.SessionID]
	onUpdate := c.onUpdate
	c.mu.Unlock()

	if ok {
		sess.deliverUpdate(update)
	}
	if onUpdate != nil {
		onUpdate(update)
	}
}

func (c *Client) setState(s State) {
	c.mu.Lock()
	c.state = s
	fn := c.onStateChange
	c.mu.Unlock()

	if fn != nil {
		fn(s)
	}
}

func asRPCError(err error) *RPCError {
	if rpcErr, ok := err.(*RPCError); ok {
		return rpcErr
	}
	return &RPCError{Code: ErrCodeInternal, Message: err.Error()}
}
