package acp

import "encoding/json"

// ACP JSON-RPC method names.
const (
	MethodInitialize          = "initialize"
	MethodAuthenticate        = "authenticate"
	MethodCancel              = "cancel"
	MethodNewSession          = "session/new"
	MethodLoadSession         = "session/load"
	MethodListSessions        = "session/list"
	MethodResumeSession       = "session/resume"
	MethodCloseSession        = "session/close"
	MethodPrompt              = "session/prompt"
	MethodSetSessionMode      = "session/setMode"
	MethodSetSessionConfig    = "session/setConfigOption"
	MethodSessionUpdate       = "session/update"
	MethodReadTextFile        = "textFile/read"
	MethodWriteTextFile       = "textFile/write"
	MethodCreateTerminal      = "terminal/create"
	MethodKillTerminal        = "terminal/kill"
	MethodReleaseTerminal     = "terminal/release"
	MethodTerminalOutput      = "terminal/output"
	MethodWaitForTerminalExit = "terminal/waitForExit"
	MethodRequestPermission   = "permission/request"
)

// --- Initialization ---

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ClientCapabilities struct {
	Fs           FileSystemCapabilities `json:"fs"`
	Terminal     bool                   `json:"terminal"`
	Experimental *ClientExperimental    `json:"experimental,omitempty"`
	Meta         map[string]any         `json:"meta,omitempty"`
}

type FileSystemCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type ClientExperimental struct {
	Nes *bool `json:"nes,omitempty"`
	Mcp *bool `json:"mcp,omitempty"`
}

type WorkspaceRoot struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

type InitializeRequest struct {
	ProtocolVersion    string             `json:"protocolVersion"`
	ClientInfo         ClientInfo         `json:"clientInfo"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	WorkspaceRoots     []WorkspaceRoot    `json:"workspaceRoots,omitempty"`
}

type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type AgentCapabilities struct {
	LoadSession  bool              `json:"loadSession,omitempty"`
	Auth         *AgentAuth        `json:"auth,omitempty"`
	Experimental *AgentExperimental `json:"experimental,omitempty"`
	Meta         map[string]any    `json:"meta,omitempty"`
}

type AgentAuth struct {
	Method string `json:"method"` // "agent", "terminal", "envVar"
}

type AgentExperimental struct {
	Nes *bool `json:"nes,omitempty"`
	Mcp *bool `json:"mcp,omitempty"`
}

type InitializeResponse struct {
	ProtocolVersion    string            `json:"protocolVersion"`
	AgentInfo          AgentInfo         `json:"agentInfo"`
	AgentCapabilities  AgentCapabilities `json:"agentCapabilities"`
	Instructions       string            `json:"instructions,omitempty"`
}

// --- Authentication ---

type AuthenticateRequest struct{}

type AuthenticateResponse struct{}

// --- Session Management ---

type McpServer struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type NewSessionRequest struct {
	Cwd        string      `json:"cwd,omitempty"`
	McpServers []McpServer `json:"mcpServers,omitempty"`
}

type NewSessionResponse struct {
	SessionID string `json:"sessionId"`
}

type LoadSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type LoadSessionResponse struct {
	SessionState map[string]any `json:"sessionState,omitempty"`
}

type SessionInfo struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title,omitempty"`
}

type ListSessionsRequest struct{}

type ListSessionsResponse struct {
	Sessions []SessionInfo `json:"sessions"`
}

type ResumeSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type ResumeSessionResponse struct{}

type CloseSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type CloseSessionResponse struct{}

// --- Prompting ---

type ContentBlock struct {
	Type     string `json:"type"` // "text", "image", "audio", "resource_link", "resource"
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Name     string `json:"name,omitempty"`
	URI      string `json:"uri,omitempty"`
}

func TextContent(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

func ImageContent(data, mimeType string) ContentBlock {
	return ContentBlock{Type: "image", Data: data, MimeType: mimeType}
}

func ResourceLinkContent(name, uri string) ContentBlock {
	return ContentBlock{Type: "resource_link", Name: name, URI: uri}
}

type PromptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type StopReason string

const (
	StopReasonEndTurn           StopReason = "end_turn"
	StopReasonRequestPermission StopReason = "request_permission"
	StopReasonCancelled         StopReason = "cancelled"
)

type PromptResponse struct {
	StopReason StopReason `json:"stopReason"`
}

// --- Cancel ---

type CancelNotification struct {
	SessionID string `json:"sessionId"`
}

// --- Session Mode ---

type SetSessionModeRequest struct {
	SessionID string `json:"sessionId"`
	Mode      string `json:"mode"`
}

type SetSessionModeResponse struct{}

type SetSessionConfigOptionRequest struct {
	SessionID string `json:"sessionId"`
	Key       string `json:"key"`
	Value     any    `json:"value"`
}

type SetSessionConfigOptionResponse struct{}

// --- Session Updates (Notifications from Agent -> Client) ---

type SessionUpdateKind string

const (
	UpdateAgentMessageChunk       SessionUpdateKind = "agent_message_chunk"
	UpdateAgentThoughtChunk       SessionUpdateKind = "agent_thought_chunk"
	UpdateUserMessageChunk        SessionUpdateKind = "user_message_chunk"
	UpdateToolCall                SessionUpdateKind = "tool_call"
	UpdateToolCallUpdate          SessionUpdateKind = "tool_call_update"
	UpdatePlan                    SessionUpdateKind = "plan"
	UpdateUsage                   SessionUpdateKind = "usage"
	UpdateSessionInfo             SessionUpdateKind = "session_info"
	UpdateAvailableCommands       SessionUpdateKind = "available_commands"
)

type ToolCallID = string
type ToolKind = string

const (
	ToolKindRead    ToolKind = "read"
	ToolKindEdit    ToolKind = "edit"
	ToolKindCreate  ToolKind = "create"
	ToolKindExecute ToolKind = "execute"
)

type ToolCallStatus string

const (
	ToolCallStatusPending   ToolCallStatus = "pending"
	ToolCallStatusRunning   ToolCallStatus = "running"
	ToolCallStatusCompleted ToolCallStatus = "completed"
	ToolCallStatusFailed    ToolCallStatus = "failed"
)

type ToolCallLocation struct {
	URI   string `json:"uri"`
	Range *Range `json:"range,omitempty"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type ToolCallContentBlock struct {
	Type       string       `json:"type"` // "content", "diff", "terminal"
	Block      ContentBlock `json:"block,omitempty"`
	Path       string       `json:"path,omitempty"`
	OldText    string       `json:"oldText,omitempty"`
	NewText    string       `json:"newText,omitempty"`
	TerminalID string       `json:"terminalId,omitempty"`
}

type ToolCallInfo struct {
	ToolCallID ToolCallID            `json:"toolCallId"`
	Title      string                `json:"title,omitempty"`
	Kind       ToolKind              `json:"kind,omitempty"`
	Status     ToolCallStatus        `json:"status,omitempty"`
	Content    []ToolCallContentBlock `json:"content,omitempty"`
	Locations  []ToolCallLocation    `json:"locations,omitempty"`
	RawInput   json.RawMessage       `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage       `json:"rawOutput,omitempty"`
}

type UsageInfo struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalCost    float64 `json:"totalCost,omitempty"`
}

type SessionInfoUpdate struct {
	Title string `json:"title,omitempty"`
}

type PlanStep struct {
	Title  string `json:"title"`
	Status string `json:"status,omitempty"`
}

type SessionUpdateNotification struct {
	SessionID string            `json:"sessionId"`
	Kind      SessionUpdateKind `json:"kind"`

	// agent_message_chunk / agent_thought_chunk / user_message_chunk
	Content string `json:"content,omitempty"`

	// tool_call / tool_call_update
	ToolCall *ToolCallInfo `json:"toolCall,omitempty"`

	// plan
	Steps []PlanStep `json:"steps,omitempty"`

	// usage
	Usage *UsageInfo `json:"usage,omitempty"`

	// session_info
	SessionInfo *SessionInfoUpdate `json:"sessionInfo,omitempty"`

	// available_commands
	Commands []string `json:"commands,omitempty"`
}

// --- File Operations (Requests from Agent -> Client) ---

type ReadTextFileRequest struct {
	Path  string `json:"path"`
	Line  *int   `json:"line,omitempty"`
	Limit *int   `json:"limit,omitempty"`
}

type ReadTextFileResponse struct {
	Content string `json:"content"`
}

type WriteTextFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type WriteTextFileResponse struct{}

// --- Terminal Operations (Requests from Agent -> Client) ---

type CreateTerminalRequest struct {
	Name string `json:"name,omitempty"`
}

type CreateTerminalResponse struct {
	TerminalID string `json:"terminalId"`
}

type KillTerminalRequest struct {
	TerminalID string `json:"terminalId"`
}

type KillTerminalResponse struct{}

type ReleaseTerminalRequest struct {
	TerminalID string `json:"terminalId"`
}

type ReleaseTerminalResponse struct{}

type TerminalOutputRequest struct {
	TerminalID string `json:"terminalId"`
	Output     string `json:"output"`
}

type TerminalOutputResponse struct {
	Output    string `json:"output"`
	Truncated bool   `json:"truncated"`
}

type WaitForTerminalExitRequest struct {
	TerminalID string `json:"terminalId"`
}

type WaitForTerminalExitResponse struct {
	ExitCode int `json:"exitCode"`
}

// --- Permission (Requests from Agent -> Client) ---

type PermissionOptionKind string

const (
	PermissionAllowOnce  PermissionOptionKind = "allow_once"
	PermissionAllowAll   PermissionOptionKind = "allow_all"
	PermissionRejectOnce PermissionOptionKind = "reject_once"
	PermissionRejectAll  PermissionOptionKind = "reject_all"
)

type PermissionOption struct {
	ID   string               `json:"id"`
	Kind PermissionOptionKind `json:"kind"`
	Text string               `json:"text,omitempty"`
}

type RequestPermissionRequest struct {
	SessionID string         `json:"sessionId"`
	ToolCall  ToolCallInfo   `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

type RequestPermissionOutcome struct {
	Selected  *PermissionSelected  `json:"selected,omitempty"`
	Cancelled *PermissionCancelled `json:"cancelled,omitempty"`
}

type PermissionSelected struct {
	OptionID string `json:"optionId"`
}

type PermissionCancelled struct{}

type RequestPermissionResponse struct {
	Outcome RequestPermissionOutcome `json:"outcome"`
}
