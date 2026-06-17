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

func TestSendMessageRejectsUnknownConversation(t *testing.T) {
	s := New(newTestStore(t))
	res := callTool(t, s, "send_message", map[string]any{
		"project": "p", "as": "dev", "to": "auditor",
		"content": "hi", "conversation_id": "does-not-exist",
	})
	if !res.IsError {
		t.Fatal("expected error for unknown conversation_id")
	}
}

func TestSendMessageBumpsConversation(t *testing.T) {
	s := New(newTestStore(t))
	start := mustCall(t, s, "start_conversation", map[string]any{"project": "p", "as": "dev", "subject": "T"})
	var sc struct {
		Conversation Conversation `json:"conversation"`
	}
	decodePayload(t, start, &sc)
	cid := sc.Conversation.ID

	mustCall(t, s, "send_message", map[string]any{
		"project": "p", "as": "auditor", "to": "dev",
		"content": "reply", "conversation_id": cid,
	})

	// The bump sets last_message_at to the posted message's created_at exactly.
	var lastAt, msgAt string
	if err := s.store.reader().QueryRow(
		"SELECT last_message_at FROM conversations WHERE id = ?", cid).Scan(&lastAt); err != nil {
		t.Fatal(err)
	}
	if err := s.store.reader().QueryRow(
		"SELECT created_at FROM messages WHERE conversation_id = ?", cid).Scan(&msgAt); err != nil {
		t.Fatal(err)
	}
	if lastAt != msgAt {
		t.Errorf("last_message_at %q should equal posted message created_at %q", lastAt, msgAt)
	}
}

func TestSendMessageNoConversationUnchanged(t *testing.T) {
	s := New(newTestStore(t))
	res := mustCall(t, s, "send_message", map[string]any{
		"project": "p", "as": "dev", "to": "auditor", "content": "plain",
	})
	if res.IsError {
		t.Errorf("plain message (no conversation_id) must still work: %s", res.Content[0].Text)
	}
}

func TestStartConversationPostsOpeningMessage(t *testing.T) {
	s := New(newTestStore(t))
	res := mustCall(t, s, "start_conversation", map[string]any{
		"project": "p", "as": "dev", "subject": "Review", "to": "auditor", "content": "please review",
	})
	var got struct {
		Conversation Conversation `json:"conversation"`
		Message      *Message     `json:"message"`
	}
	decodePayload(t, res, &got)
	if got.Message == nil || got.Message.Content != "please review" {
		t.Fatalf("opening message not posted: %+v", got.Message)
	}
	if got.Message.ConversationID == nil || *got.Message.ConversationID != got.Conversation.ID {
		t.Errorf("opening message not linked to conversation: %+v", got.Message)
	}
	if got.Conversation.LastMessageAt != got.Message.CreatedAt {
		t.Errorf("conversation last_message_at should track the opening message")
	}
	inbox := mustCall(t, s, "get_inbox", map[string]any{"project": "p", "as": "auditor"})
	var ib struct {
		Count int `json:"count"`
	}
	decodePayload(t, inbox, &ib)
	if ib.Count != 1 {
		t.Errorf("auditor inbox should have the opening message, got count %d", ib.Count)
	}
}

func TestStartConversationNoMessageWithoutBoth(t *testing.T) {
	s := New(newTestStore(t))
	res := mustCall(t, s, "start_conversation", map[string]any{
		"project": "p", "as": "dev", "subject": "S", "to": "auditor", // no content
	})
	var got struct {
		Message *Message `json:"message"`
	}
	decodePayload(t, res, &got)
	if got.Message != nil {
		t.Errorf("no message should post without content, got %+v", got.Message)
	}
}
