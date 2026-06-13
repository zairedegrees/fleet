package coord

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResultTextIsDoubleEncoded(t *testing.T) {
	payload := map[string]any{"count": 0, "orgs": []any{}}

	res, err := resultText(payload)
	if err != nil {
		t.Fatalf("resultText: %v", err)
	}
	if res.IsError {
		t.Error("resultText set isError true")
	}
	if len(res.Content) != 1 || res.Content[0].Type != "text" {
		t.Fatalf("unexpected content: %+v", res.Content)
	}

	// content[0].text is a JSON STRING that re-parses to the payload, and equals
	// the payload's own marshaling byte-for-byte (the double-encoding contract).
	var back map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].Text), &back); err != nil {
		t.Fatalf("content text is not valid JSON: %v", err)
	}
	want, _ := json.Marshal(payload)
	if res.Content[0].Text != string(want) {
		t.Errorf("text = %s, want %s", res.Content[0].Text, want)
	}

	// Prove the WIRE form, not just the in-memory struct: once the full result is
	// serialized, text must be an ESCAPED JSON string ("text":"{\"count\":...}"),
	// never a nested object. This is the two-stage decode the fleet client does
	// (client.go:102 unmarshals result, :122 unmarshals content[0].text again).
	wire, err := marshalResult(json.RawMessage("1"), res)
	if err != nil {
		t.Fatalf("marshalResult: %v", err)
	}
	if !strings.Contains(string(wire), `"text":"{`) {
		t.Errorf("serialized text is not an escaped JSON string (nested object?): %s", wire)
	}
	var stage1 struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(wire, &stage1); err != nil {
		t.Fatalf("stage-1 decode: %v", err)
	}
	var stage2 map[string]any
	if err := json.Unmarshal([]byte(stage1.Result.Content[0].Text), &stage2); err != nil {
		t.Fatalf("stage-2 decode of content text failed: %v", err)
	}
}

func TestResultErrorIsPlainString(t *testing.T) {
	res := resultError("boom")
	if !res.IsError {
		t.Error("resultError did not set isError true")
	}
	if got := res.Content[0].Text; got != "boom" {
		t.Errorf("text = %q, want %q", got, "boom")
	}
	// The error text must NOT be JSON-wrapped — a plain string.
	if res.Content[0].Text[0] == '"' || res.Content[0].Text[0] == '{' {
		t.Errorf("error text looks JSON-encoded: %q", res.Content[0].Text)
	}
}

func TestMarshalResultEchoesID(t *testing.T) {
	res, _ := resultText(map[string]any{"ok": true})
	b, err := marshalResult(json.RawMessage("7"), res)
	if err != nil {
		t.Fatalf("marshalResult: %v", err)
	}
	var resp rpcResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(resp.ID) != "7" {
		t.Errorf("id = %s, want 7", resp.ID)
	}
	if resp.Jsonrpc != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", resp.Jsonrpc)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error field: %v", resp.Error)
	}
}
