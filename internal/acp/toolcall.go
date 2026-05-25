package acp

import "context"

// ToolHandler handles agent-initiated requests. The ACP protocol is
// bidirectional: the agent can ask the client to read/write files,
// run terminal commands, or request permission for operations.
//
// Implementations decide how to fulfill each request. Return a nil
// response and non-nil error to signal a failure to the agent.
type ToolHandler interface {
	ReadTextFile(ctx context.Context, req ReadTextFileRequest) (*ReadTextFileResponse, error)
	WriteTextFile(ctx context.Context, req WriteTextFileRequest) (*WriteTextFileResponse, error)
	CreateTerminal(ctx context.Context, req CreateTerminalRequest) (*CreateTerminalResponse, error)
	KillTerminal(ctx context.Context, req KillTerminalRequest) (*KillTerminalResponse, error)
	ReleaseTerminal(ctx context.Context, req ReleaseTerminalRequest) (*ReleaseTerminalResponse, error)
	TerminalOutput(ctx context.Context, req TerminalOutputRequest) (*TerminalOutputResponse, error)
	WaitForTerminalExit(ctx context.Context, req WaitForTerminalExitRequest) (*WaitForTerminalExitResponse, error)
	RequestPermission(ctx context.Context, req RequestPermissionRequest) (*RequestPermissionResponse, error)
}

// NoopToolHandler rejects all tool calls. Use it as a default or embed it
// to override only the methods you care about.
type NoopToolHandler struct{}

func (NoopToolHandler) ReadTextFile(context.Context, ReadTextFileRequest) (*ReadTextFileResponse, error) {
	return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: "read_text_file not supported"}
}

func (NoopToolHandler) WriteTextFile(context.Context, WriteTextFileRequest) (*WriteTextFileResponse, error) {
	return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: "write_text_file not supported"}
}

func (NoopToolHandler) CreateTerminal(context.Context, CreateTerminalRequest) (*CreateTerminalResponse, error) {
	return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: "create_terminal not supported"}
}

func (NoopToolHandler) KillTerminal(context.Context, KillTerminalRequest) (*KillTerminalResponse, error) {
	return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: "kill_terminal not supported"}
}

func (NoopToolHandler) ReleaseTerminal(context.Context, ReleaseTerminalRequest) (*ReleaseTerminalResponse, error) {
	return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: "release_terminal not supported"}
}

func (NoopToolHandler) TerminalOutput(context.Context, TerminalOutputRequest) (*TerminalOutputResponse, error) {
	return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: "terminal_output not supported"}
}

func (NoopToolHandler) WaitForTerminalExit(context.Context, WaitForTerminalExitRequest) (*WaitForTerminalExitResponse, error) {
	return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: "wait_for_terminal_exit not supported"}
}

func (NoopToolHandler) RequestPermission(_ context.Context, req RequestPermissionRequest) (*RequestPermissionResponse, error) {
	for _, opt := range req.Options {
		if opt.Kind == PermissionRejectOnce {
			return &RequestPermissionResponse{
				Outcome: RequestPermissionOutcome{
					Selected: &PermissionSelected{OptionID: opt.ID},
				},
			}, nil
		}
	}
	return &RequestPermissionResponse{
		Outcome: RequestPermissionOutcome{
			Cancelled: &PermissionCancelled{},
		},
	}, nil
}
