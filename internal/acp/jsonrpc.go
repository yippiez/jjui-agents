package acp

import (
	"encoding/json"
	"fmt"
)

const jsonrpcVersion = "2.0"

// Standard JSON-RPC 2.0 error codes.
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603

	// ACP-specific error codes.
	ErrCodeAuthRequired    = -1
	ErrCodeResourceNotFound = -2
	ErrCodeRequestCancelled = -3
)

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    *json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// rpcRequest is a JSON-RPC 2.0 request (has id + method).
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResponse is a JSON-RPC 2.0 response (has id + result/error).
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// rpcNotification is a JSON-RPC 2.0 notification (has method, no id).
type rpcNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcMessageKind int

const (
	kindResponse rpcMessageKind = iota
	kindRequest
	kindNotification
)

// rpcIncoming is the result of classifying a raw JSON message.
type rpcIncoming struct {
	kind         rpcMessageKind
	request      *rpcRequest
	response     *rpcResponse
	notification *rpcNotification
}

// classifyMessage reads a raw JSON line and determines whether it is a
// request, response, or notification per JSON-RPC 2.0 rules.
func classifyMessage(data []byte) (rpcIncoming, error) {
	var probe struct {
		ID     *json.RawMessage `json:"id"`
		Method *string          `json:"method"`
		Result *json.RawMessage `json:"result"`
		Error  *json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return rpcIncoming{}, fmt.Errorf("invalid JSON-RPC message: %w", err)
	}

	hasID := probe.ID != nil
	hasMethod := probe.Method != nil
	hasResult := probe.Result != nil
	hasError := probe.Error != nil

	switch {
	case hasID && hasMethod:
		var req rpcRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return rpcIncoming{}, err
		}
		return rpcIncoming{kind: kindRequest, request: &req}, nil

	case hasID && (hasResult || hasError):
		var resp rpcResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return rpcIncoming{}, err
		}
		return rpcIncoming{kind: kindResponse, response: &resp}, nil

	case hasMethod && !hasID:
		var notif rpcNotification
		if err := json.Unmarshal(data, &notif); err != nil {
			return rpcIncoming{}, err
		}
		return rpcIncoming{kind: kindNotification, notification: &notif}, nil

	default:
		return rpcIncoming{}, fmt.Errorf("unclassifiable JSON-RPC message: %s", string(data))
	}
}

func marshalRequest(id int64, method string, params any) ([]byte, error) {
	req := rpcRequest{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Method:  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		req.Params = raw
	}
	return json.Marshal(req)
}

func marshalResponse(id int64, result any, rpcErr *RPCError) ([]byte, error) {
	resp := rpcResponse{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Error:   rpcErr,
	}
	if result != nil && rpcErr == nil {
		raw, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		resp.Result = raw
	}
	return json.Marshal(resp)
}

func marshalNotification(method string, params any) ([]byte, error) {
	notif := rpcNotification{
		JSONRPC: jsonrpcVersion,
		Method:  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		notif.Params = raw
	}
	return json.Marshal(notif)
}
