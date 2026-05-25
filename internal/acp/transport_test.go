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

func TestClassifyMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantKind rpcMessageKind
		wantErr  bool
	}{
		{
			name:     "request",
			input:    `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
			wantKind: kindRequest,
		},
		{
			name:     "response with result",
			input:    `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"1.0"}}`,
			wantKind: kindResponse,
		},
		{
			name:     "response with error",
			input:    `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"not found"}}`,
			wantKind: kindResponse,
		},
		{
			name:     "notification",
			input:    `{"jsonrpc":"2.0","method":"session/update","params":{}}`,
			wantKind: kindNotification,
		},
		{
			name:    "invalid json",
			input:   `not json`,
			wantErr: true,
		},
		{
			name:    "empty object",
			input:   `{}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := classifyMessage([]byte(tt.input))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKind, msg.kind)
		})
	}
}

func TestTransportRequestResponse(t *testing.T) {
	clientRead, agentWrite := io.Pipe()
	agentRead, clientWrite := io.Pipe()

	transport := NewTransport(clientRead, clientWrite)
	go transport.Run()

	go func() {
		buf := make([]byte, 4096)
		n, err := agentRead.Read(buf)
		if err != nil {
			return
		}

		var req rpcRequest
		if err := json.Unmarshal(buf[:n], &req); err != nil {
			return
		}

		resp, _ := marshalResponse(req.ID, map[string]string{"status": "ok"}, nil)
		resp = append(resp, '\n')
		_, _ = agentWrite.Write(resp)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := transport.SendRequest(ctx, "test/method", map[string]string{"key": "value"})
	require.NoError(t, err)

	var parsed map[string]string
	require.NoError(t, json.Unmarshal(result, &parsed))
	assert.Equal(t, "ok", parsed["status"])

	_ = clientWrite.Close()
	_ = agentWrite.Close()
}

func TestTransportNotifications(t *testing.T) {
	clientRead, agentWrite := io.Pipe()
	_, clientWrite := io.Pipe()

	transport := NewTransport(clientRead, clientWrite)
	go transport.Run()

	notif, _ := marshalNotification("session/update", map[string]string{"sessionId": "s1"})
	notif = append(notif, '\n')
	_, err := agentWrite.Write(notif)
	require.NoError(t, err)

	select {
	case n := <-transport.Notifications:
		assert.Equal(t, "session/update", n.Method)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification")
	}

	_ = clientWrite.Close()
	_ = agentWrite.Close()
}

func TestTransportAgentRequests(t *testing.T) {
	clientRead, agentWrite := io.Pipe()
	_, clientWrite := io.Pipe()

	transport := NewTransport(clientRead, clientWrite)
	go transport.Run()

	req, _ := marshalRequest(42, "textFile/read", map[string]string{"path": "/tmp/test.go"})
	req = append(req, '\n')
	_, err := agentWrite.Write(req)
	require.NoError(t, err)

	select {
	case r := <-transport.Requests:
		assert.Equal(t, "textFile/read", r.Method)
		assert.Equal(t, int64(42), r.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for agent request")
	}

	_ = clientWrite.Close()
	_ = agentWrite.Close()
}

func TestTransportContextCancellation(t *testing.T) {
	clientRead, agentWrite := io.Pipe()
	agentRead, clientWrite := io.Pipe()

	transport := NewTransport(clientRead, clientWrite)
	go transport.Run()
	// drain writes so they don't block
	go func() { _, _ = io.Copy(io.Discard, agentRead) }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := transport.SendRequest(ctx, "test/method", nil)
	assert.ErrorIs(t, err, context.Canceled)

	_ = clientWrite.Close()
	_ = agentWrite.Close()
}
