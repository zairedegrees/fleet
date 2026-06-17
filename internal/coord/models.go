package coord

// The model structs mirror the wrai.th wire shapes exactly (same JSON tags,
// same omitempty), so a response coord marshals is byte-comparable with what the
// relay returns and the fleet client / agent skill decode unchanged.

type Agent struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Role            string  `json:"role"`
	Description     string  `json:"description"`
	RegisteredAt    string  `json:"registered_at"`
	LastSeen        string  `json:"last_seen"`
	Project         string  `json:"project"`
	ReportsTo       *string `json:"reports_to,omitempty"`
	ProfileSlug     *string `json:"profile_slug,omitempty"`
	Status          string  `json:"status"`
	DeactivatedAt   *string `json:"deactivated_at,omitempty"`
	IsExecutive     bool    `json:"is_executive"`
	SessionID       *string `json:"session_id,omitempty"`
	InterestTags    string  `json:"interest_tags"`
	MaxContextBytes int     `json:"max_context_bytes"`
}

type Profile struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	Role         string  `json:"role"`
	ContextPack  string  `json:"context_pack"`
	SoulKeys     string  `json:"soul_keys"`
	Skills       string  `json:"skills"`
	VaultPaths   string  `json:"vault_paths"`
	AllowedTools string  `json:"allowed_tools"`
	PoolSize     int     `json:"pool_size"`
	Project      string  `json:"project"`
	OrgID        *string `json:"org_id,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type Memory struct {
	ID           string  `json:"id"`
	Key          string  `json:"key"`
	Value        string  `json:"value"`
	Tags         string  `json:"tags"`
	Scope        string  `json:"scope"`
	Project      string  `json:"project"`
	AgentName    string  `json:"agent_name"`
	Confidence   string  `json:"confidence"`
	Version      int     `json:"version"`
	Supersedes   *string `json:"supersedes,omitempty"`
	ConflictWith *string `json:"conflict_with,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	ArchivedAt   *string `json:"archived_at,omitempty"`
	ArchivedBy   *string `json:"archived_by,omitempty"`
	Layer        string  `json:"layer"`
}

type Message struct {
	ID             string  `json:"id"`
	From           string  `json:"from"`
	To             string  `json:"to"`
	ReplyTo        *string `json:"reply_to"`
	Type           string  `json:"type"`
	Subject        string  `json:"subject"`
	Content        string  `json:"content"`
	Metadata       string  `json:"metadata"`
	CreatedAt      string  `json:"created_at"`
	ReadAt         *string `json:"read_at"`
	ConversationID *string `json:"conversation_id,omitempty"`
	Project        string  `json:"project"`
	TaskID         *string `json:"task_id,omitempty"`
	Priority       string  `json:"priority"`
	TTLSeconds     int     `json:"ttl_seconds"`
	ExpiredAt      *string `json:"expired_at,omitempty"`
	DeliveryID     *string `json:"delivery_id,omitempty"`
	DeliveryState  *string `json:"delivery_state,omitempty"`
}

type Task struct {
	ID             string  `json:"id"`
	ProfileSlug    string  `json:"profile_slug"`
	AssignedTo     *string `json:"assigned_to,omitempty"`
	DispatchedBy   string  `json:"dispatched_by"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Priority       string  `json:"priority"`
	Status         string  `json:"status"`
	Result         *string `json:"result,omitempty"`
	BlockedReason  *string `json:"blocked_reason,omitempty"`
	Project        string  `json:"project"`
	DispatchedAt   string  `json:"dispatched_at"`
	AcceptedAt     *string `json:"accepted_at,omitempty"`
	StartedAt      *string `json:"started_at,omitempty"`
	CompletedAt    *string `json:"completed_at,omitempty"`
	ParentTaskID   *string `json:"parent_task_id,omitempty"`
	AckNotifiedAt  *string `json:"ack_notified_at,omitempty"`
	AckEscalatedAt *string `json:"ack_escalated_at,omitempty"`
	BoardID        *string `json:"board_id,omitempty"`
	GoalID         *string `json:"goal_id,omitempty"`
	ArchivedAt     *string `json:"archived_at,omitempty"`
}

// Conversation is a named agent-to-agent thread. Messages link to it via their
// conversation_id; last_message_at tracks activity for ordering.
type Conversation struct {
	ID            string `json:"id"`
	Project       string `json:"project"`
	Subject       string `json:"subject"`
	CreatedBy     string `json:"created_by"`
	CreatedAt     string `json:"created_at"`
	LastMessageAt string `json:"last_message_at"`
	Status        string `json:"status"`
}

// Goal is a high-level objective that groups tasks (via tasks.goal_id).
type Goal struct {
	ID          string `json:"id"`
	Project     string `json:"project"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	Status      string `json:"status"`
}
