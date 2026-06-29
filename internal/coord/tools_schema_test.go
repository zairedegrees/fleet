package coord

import "testing"

// send_message must advertise conversation_id: handleSendMessage reads it and
// start_conversation instructs agents to use it, so a schema-validating client
// would otherwise reject the threading reply.
func TestSendMessageAdvertisesConversationID(t *testing.T) {
	var sm *toolDef
	for i := range toolDefs {
		if toolDefs[i].Name == "send_message" {
			sm = &toolDefs[i]
		}
	}
	if sm == nil {
		t.Fatal("send_message not found in toolDefs")
	}
	props, ok := sm.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("send_message inputSchema has no properties map: %v", sm.InputSchema)
	}
	if _, ok := props["conversation_id"]; !ok {
		t.Errorf("send_message must advertise conversation_id; properties = %v", props)
	}
}
