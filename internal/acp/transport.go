package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Transport manages JSON-RPC 2.0 communication over stdin/stdout pipes.
// It handles request/response correlation and routes incoming notifications
// and agent-initiated requests to separate channels.
type Transport struct {
	writer io.Writer
	reader io.Reader

	writeMu sync.Mutex
	nextID  atomic.Int64

	pendingMu sync.Mutex
	pending   map[int64]chan *rpcResponse

	// Notifications receives agent-initiated notifications (e.g. session/update).
	Notifications chan *rpcNotification

	// Requests receives agent-initiated requests (e.g. file reads, terminal, permissions).
	Requests chan *rpcRequest

	done chan struct{}
	err  error
}

// NewTransport creates a transport over the given reader/writer pair.
// Call Run to start the read loop.
func NewTransport(r io.Reader, w io.Writer) *Transport {
	return &Transport{
		writer:        w,
		reader:        r,
		pending:       make(map[int64]chan *rpcResponse),
		Notifications: make(chan *rpcNotification, 64),
		Requests:      make(chan *rpcRequest, 16),
		done:          make(chan struct{}),
	}
}

// Run starts the read loop. It blocks until the reader is closed or an error
// occurs. Call this in a goroutine.
func (t *Transport) Run() {
	defer close(t.done)
	scanner := bufio.NewScanner(t.reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		msg, err := classifyMessage(line)
		if err != nil {
			continue
		}

		switch msg.kind {
		case kindResponse:
			t.pendingMu.Lock()
			ch, ok := t.pending[msg.response.ID]
			if ok {
				delete(t.pending, msg.response.ID)
			}
			t.pendingMu.Unlock()
			if ok {
				ch <- msg.response
			}

		case kindRequest:
			select {
			case t.Requests <- msg.request:
			default:
			}

		case kindNotification:
			select {
			case t.Notifications <- msg.notification:
			default:
			}
		}
	}

	if err := scanner.Err(); err != nil {
		t.err = err
	}
}

// SendRequest sends a JSON-RPC request and waits for the correlated response.
func (t *Transport) SendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := t.nextID.Add(1)
	ch := make(chan *rpcResponse, 1)

	t.pendingMu.Lock()
	t.pending[id] = ch
	t.pendingMu.Unlock()

	data, err := marshalRequest(id, method, params)
	if err != nil {
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	if err := t.writeLine(data); err != nil {
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-t.done:
		return nil, fmt.Errorf("transport closed")
	}
}

// SendResponse sends a JSON-RPC response for an agent-initiated request.
func (t *Transport) SendResponse(id int64, result any, rpcErr *RPCError) error {
	data, err := marshalResponse(id, result, rpcErr)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	return t.writeLine(data)
}

// SendNotification sends a JSON-RPC notification (no response expected).
func (t *Transport) SendNotification(method string, params any) error {
	data, err := marshalNotification(method, params)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	return t.writeLine(data)
}

// Done returns a channel that is closed when the read loop exits.
func (t *Transport) Done() <-chan struct{} {
	return t.done
}

// Err returns any error from the read loop after Done is closed.
func (t *Transport) Err() error {
	return t.err
}

func (t *Transport) writeLine(data []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	data = append(data, '\n')
	_, err := t.writer.Write(data)
	return err
}
