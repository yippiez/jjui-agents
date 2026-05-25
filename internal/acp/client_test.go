package acp

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockToolHandler struct {
	NoopToolHandler
	readFileFn func(context.Context, ReadTextFileRequest) (*ReadTextFileResponse, error)
}

func (m *mockToolHandler) ReadTextFile(ctx context.Context, req ReadTextFileRequest) (*ReadTextFileResponse, error) {
	if m.readFileFn != nil {
		return m.readFileFn(ctx, req)
	}
	return m.NoopToolHandler.ReadTextFile(ctx, req)
}

func TestProtocolTypes(t *testing.T) {
	t.Run("content block constructors", func(t *testing.T) {
		text := TextContent("hello")
		assert.Equal(t, "text", text.Type)
		assert.Equal(t, "hello", text.Text)

		img := ImageContent("base64data", "image/png")
		assert.Equal(t, "image", img.Type)
		assert.Equal(t, "base64data", img.Data)
		assert.Equal(t, "image/png", img.MimeType)

		link := ResourceLinkContent("file.go", "file:///tmp/file.go")
		assert.Equal(t, "resource_link", link.Type)
		assert.Equal(t, "file.go", link.Name)
		assert.Equal(t, "file:///tmp/file.go", link.URI)
	})

	t.Run("initialize request serialization", func(t *testing.T) {
		req := InitializeRequest{
			ProtocolVersion: "1.0",
			ClientInfo:      ClientInfo{Name: "jjui", Version: "0.1.0"},
			ClientCapabilities: ClientCapabilities{
				Fs:       FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
				Terminal: true,
			},
			WorkspaceRoots: []WorkspaceRoot{{URI: "file:///home/user/project"}},
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var parsed InitializeRequest
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, req.ProtocolVersion, parsed.ProtocolVersion)
		assert.Equal(t, req.ClientInfo.Name, parsed.ClientInfo.Name)
		assert.True(t, parsed.ClientCapabilities.Fs.ReadTextFile)
		assert.True(t, parsed.ClientCapabilities.Terminal)
		assert.Len(t, parsed.WorkspaceRoots, 1)
	})

	t.Run("session update notification serialization", func(t *testing.T) {
		update := SessionUpdateNotification{
			SessionID: "sess-1",
			Kind:      UpdateAgentMessageChunk,
			Content:   "Hello, I can help with that.",
		}

		data, err := json.Marshal(update)
		require.NoError(t, err)

		var parsed SessionUpdateNotification
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "sess-1", parsed.SessionID)
		assert.Equal(t, UpdateAgentMessageChunk, parsed.Kind)
		assert.Equal(t, "Hello, I can help with that.", parsed.Content)
	})

	t.Run("tool call info serialization", func(t *testing.T) {
		tc := ToolCallInfo{
			ToolCallID: "tc-1",
			Title:      "Read file",
			Kind:       ToolKindRead,
			Status:     ToolCallStatusCompleted,
			Locations: []ToolCallLocation{
				{URI: "file:///tmp/main.go", Range: &Range{
					Start: Position{Line: 10, Character: 0},
					End:   Position{Line: 20, Character: 0},
				}},
			},
		}

		data, err := json.Marshal(tc)
		require.NoError(t, err)

		var parsed ToolCallInfo
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "tc-1", parsed.ToolCallID)
		assert.Equal(t, ToolKindRead, parsed.Kind)
		assert.Equal(t, ToolCallStatusCompleted, parsed.Status)
		require.Len(t, parsed.Locations, 1)
		assert.Equal(t, 10, parsed.Locations[0].Range.Start.Line)
	})
}

func TestToolCallDispatch(t *testing.T) {
	clientRead, agentWrite := io.Pipe()
	agentRead, clientWrite := io.Pipe()

	handler := &mockToolHandler{
		readFileFn: func(_ context.Context, req ReadTextFileRequest) (*ReadTextFileResponse, error) {
			return &ReadTextFileResponse{Content: "file contents of " + req.Path}, nil
		},
	}

	transport := NewTransport(clientRead, clientWrite)
	go transport.Run()

	client := &Client{
		handler:   handler,
		transport: transport,
		sessions:  make(map[string]*Session),
	}
	client.ctx, client.cancel = context.WithCancel(context.Background())
	go client.handleIncoming()

	fileReq, _ := marshalRequest(100, MethodReadTextFile, ReadTextFileRequest{Path: "/tmp/hello.go"})
	fileReq = append(fileReq, '\n')
	_, err := agentWrite.Write(fileReq)
	require.NoError(t, err)

	buf := make([]byte, 4096)
	n, err := agentRead.Read(buf)
	require.NoError(t, err)

	var resp rpcResponse
	require.NoError(t, json.Unmarshal(buf[:n], &resp))
	assert.Equal(t, int64(100), resp.ID)
	assert.Nil(t, resp.Error)

	var fileResp ReadTextFileResponse
	require.NoError(t, json.Unmarshal(resp.Result, &fileResp))
	assert.Equal(t, "file contents of /tmp/hello.go", fileResp.Content)

	client.cancel()
	_ = clientWrite.Close()
	_ = agentWrite.Close()
}

func TestSessionUpdateRouting(t *testing.T) {
	clientRead, agentWrite := io.Pipe()
	_, clientWrite := io.Pipe()

	transport := NewTransport(clientRead, clientWrite)
	go transport.Run()

	var receivedUpdate SessionUpdateNotification
	updateCh := make(chan struct{}, 1)

	client := &Client{
		handler:   NoopToolHandler{},
		transport: transport,
		sessions:  make(map[string]*Session),
		onUpdate: func(update SessionUpdateNotification) {
			receivedUpdate = update
			select {
			case updateCh <- struct{}{}:
			default:
			}
		},
	}
	client.ctx, client.cancel = context.WithCancel(context.Background())

	sess := newSession("test-session", client)
	client.sessions["test-session"] = sess
	go client.handleIncoming()

	update := SessionUpdateNotification{
		SessionID: "test-session",
		Kind:      UpdateAgentMessageChunk,
		Content:   "thinking...",
	}
	notif, _ := marshalNotification(MethodSessionUpdate, update)
	notif = append(notif, '\n')
	_, err := agentWrite.Write(notif)
	require.NoError(t, err)

	select {
	case <-updateCh:
		assert.Equal(t, "test-session", receivedUpdate.SessionID)
		assert.Equal(t, UpdateAgentMessageChunk, receivedUpdate.Kind)
		assert.Equal(t, "thinking...", receivedUpdate.Content)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for update callback")
	}

	select {
	case delivered := <-sess.updates:
		assert.Equal(t, "thinking...", delivered.Content)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session delivery")
	}

	client.cancel()
	_ = clientWrite.Close()
	_ = agentWrite.Close()
}

func TestAdapterConfigs(t *testing.T) {
	cursor := CursorAdapter()
	assert.Equal(t, "cursor", cursor.Name)
	assert.Equal(t, []string{"npx", "-y", "cursor-agent-acp"}, cursor.Command)

	pi := PiAdapter()
	assert.Equal(t, "pi", pi.Name)
	assert.Equal(t, []string{"npx", "-y", "pi-acp"}, pi.Command)

	piDirect := PiDirectAdapter()
	assert.Equal(t, "pi-direct", piDirect.Name)
	assert.Equal(t, []string{"pi", "--mode", "rpc"}, piDirect.Command)

	custom := CustomAdapter("myagent", []string{"./my-agent"}, []string{"KEY=val"})
	assert.Equal(t, "myagent", custom.Name)
	assert.Equal(t, []string{"./my-agent"}, custom.Command)
	assert.Equal(t, []string{"KEY=val"}, custom.Env)
}

func TestNoopToolHandler(t *testing.T) {
	h := NoopToolHandler{}
	ctx := context.Background()

	_, err := h.ReadTextFile(ctx, ReadTextFileRequest{Path: "/tmp/x"})
	assert.Error(t, err)

	_, err = h.WriteTextFile(ctx, WriteTextFileRequest{Path: "/tmp/x", Content: "y"})
	assert.Error(t, err)

	_, err = h.CreateTerminal(ctx, CreateTerminalRequest{})
	assert.Error(t, err)

	resp, err := h.RequestPermission(ctx, RequestPermissionRequest{
		Options: []PermissionOption{
			{ID: "allow", Kind: PermissionAllowOnce},
			{ID: "reject", Kind: PermissionRejectOnce},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Outcome.Selected)
	assert.Equal(t, "reject", resp.Outcome.Selected.OptionID)
}

func TestClientState(t *testing.T) {
	client := NewClient(CursorAdapter(), "/tmp", nil)
	assert.Equal(t, Disconnected, client.State())
	assert.Equal(t, "disconnected", Disconnected.String())
	assert.Equal(t, "starting", Starting.String())
	assert.Equal(t, "initializing", Initializing.String())
	assert.Equal(t, "ready", Ready.String())
	assert.Equal(t, "closed", Closed.String())

	var stateChanges []State
	client.SetOnStateChange(func(s State) {
		stateChanges = append(stateChanges, s)
	})

	client.setState(Starting)
	client.setState(Ready)
	assert.Equal(t, []State{Starting, Ready}, stateChanges)
	assert.Equal(t, Ready, client.State())
}
