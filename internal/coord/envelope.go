package coord

import "encoding/json"

// JSON-RPC 2.0 error codes coord uses for protocol-level failures (a malformed
// body or a non-tools/call method). Tool-level problems never use these — they
// come back as a normal result with isError=true, mirroring wrai.th.
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
)

type rpcRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// toolResult is the MCP tools/call result. content[0].text carries the payload;
// for success it is a JSON STRING (the payload marshaled, i.e. double-encoded),
// for errors it is a plain human string. The fleet client unmarshals
// content[0].text back into the payload object, so this double-encoding is part
// of the wire contract, not an implementation detail.
type toolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  *toolResult     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// resultText wraps a payload as a successful tools/call result. The payload is
// marshaled to JSON and that JSON is stored AS A STRING in content[0].text
// (double-encoding), exactly as wrai.th and the fleet client expect.
func resultText(payload any) (toolResult, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return toolResult{}, err
	}
	return toolResult{
		Content: []contentItem{{Type: "text", Text: string(b)}},
		IsError: false,
	}, nil
}

// resultError wraps a plain message as a tool-level error result (isError=true,
// content[0].text is the raw message, not JSON).
func resultError(msg string) toolResult {
	return toolResult{
		Content: []contentItem{{Type: "text", Text: msg}},
		IsError: true,
	}
}

func idOrNull(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}

func marshalResult(id json.RawMessage, res toolResult) ([]byte, error) {
	return json.Marshal(rpcResponse{Jsonrpc: "2.0", ID: idOrNull(id), Result: &res})
}

func marshalRPCError(id json.RawMessage, code int, msg string) ([]byte, error) {
	return json.Marshal(rpcResponse{Jsonrpc: "2.0", ID: idOrNull(id), Error: &rpcError{Code: code, Message: msg}})
}
