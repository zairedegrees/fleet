package coord

import "testing"

func TestStartConversationMintsRow(t *testing.T) {
	s := New(newTestStore(t))
	res := mustCall(t, s, "start_conversation", map[string]any{
		"project": "p", "as": "dev", "subject": "Review auth PR",
	})
	var got struct {
		Conversation Conversation `json:"conversation"`
	}
	decodePayload(t, res, &got)
	if got.Conversation.ID == "" || got.Conversation.Subject != "Review auth PR" {
		t.Errorf("bad conversation: %+v", got.Conversation)
	}
	if got.Conversation.CreatedBy != "dev" || got.Conversation.Status != "open" {
		t.Errorf("bad created_by/status: %+v", got.Conversation)
	}
	if got.Conversation.CreatedAt == "" || got.Conversation.LastMessageAt != got.Conversation.CreatedAt {
		t.Errorf("timestamps should start equal: %+v", got.Conversation)
	}
}

func TestStartConversationRequiresSubject(t *testing.T) {
	s := New(newTestStore(t))
	res := callTool(t, s, "start_conversation", map[string]any{"project": "p", "as": "dev"})
	if !res.IsError {
		t.Error("expected error when subject missing")
	}
}
