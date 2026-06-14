package coord

// schemaDDL is the full SQLite schema coord creates on first open. It mirrors
// the wrai.th relay tables coord must serve, so the wire contract fleet and the
// agents speak stays byte-identical regardless of which backend answers.
//
// Timestamp formats are deliberate and must be preserved: agents and
// message_reads use RFC3339; messages, deliveries, tasks and memories use the
// microsecond format (2006-01-02T15:04:05.000000Z). See timeRFC3339 / timeMicro
// in store.go.
const schemaDDL = `
CREATE TABLE IF NOT EXISTS agents (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  registered_at TEXT NOT NULL,
  last_seen TEXT NOT NULL,
  project TEXT NOT NULL DEFAULT 'default',
  reports_to TEXT,
  profile_slug TEXT,
  status TEXT NOT NULL DEFAULT 'active',
  deactivated_at TEXT,
  is_executive INTEGER NOT NULL DEFAULT 0,
  session_id TEXT,
  org_id TEXT,
  interest_tags TEXT NOT NULL DEFAULT '[]',
  max_context_bytes INTEGER NOT NULL DEFAULT 16384
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_project_name ON agents(project, name);

CREATE TABLE IF NOT EXISTS profiles (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT '',
  context_pack TEXT NOT NULL DEFAULT '',
  soul_keys TEXT NOT NULL DEFAULT '[]',
  skills TEXT NOT NULL DEFAULT '[]',
  vault_paths TEXT NOT NULL DEFAULT '[]',
  allowed_tools TEXT NOT NULL DEFAULT '[]',
  pool_size INTEGER NOT NULL DEFAULT 3,
  project TEXT NOT NULL DEFAULT 'default',
  org_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_project_slug ON profiles(project, slug);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  profile_slug TEXT NOT NULL,
  assigned_to TEXT,
  dispatched_by TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  priority TEXT NOT NULL DEFAULT 'P2',
  status TEXT NOT NULL DEFAULT 'pending',
  result TEXT,
  blocked_reason TEXT,
  project TEXT NOT NULL DEFAULT 'default',
  dispatched_at TEXT NOT NULL,
  accepted_at TEXT,
  started_at TEXT,
  completed_at TEXT,
  parent_task_id TEXT,
  board_id TEXT,
  goal_id TEXT,
  archived_at TEXT,
  ack_notified_at TEXT,
  ack_escalated_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_tasks_profile ON tasks(project, profile_slug);
CREATE INDEX IF NOT EXISTS idx_tasks_project_status ON tasks(project, status);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  from_agent TEXT NOT NULL,
  to_agent TEXT NOT NULL,
  reply_to TEXT,
  type TEXT NOT NULL DEFAULT 'notification',
  subject TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  read_at TEXT,
  project TEXT NOT NULL DEFAULT 'default',
  conversation_id TEXT,
  task_id TEXT,
  priority TEXT NOT NULL DEFAULT 'P2',
  ttl_seconds INTEGER NOT NULL DEFAULT 14400,
  expired_at TEXT
);

CREATE TABLE IF NOT EXISTS deliveries (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL,
  to_agent TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'queued',
  sequence_number INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  surfaced_at TEXT,
  acknowledged_at TEXT,
  expired_at TEXT,
  project TEXT NOT NULL DEFAULT 'default',
  FOREIGN KEY (message_id) REFERENCES messages(id)
);
CREATE INDEX IF NOT EXISTS idx_deliveries_agent_state ON deliveries(to_agent, project, state);

CREATE TABLE IF NOT EXISTS message_reads (
  message_id TEXT NOT NULL,
  agent_name TEXT NOT NULL,
  project TEXT NOT NULL DEFAULT 'default',
  read_at TEXT NOT NULL,
  UNIQUE(message_id, agent_name)
);

CREATE TABLE IF NOT EXISTS memories (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL,
  value TEXT NOT NULL,
  tags TEXT NOT NULL DEFAULT '[]',
  scope TEXT NOT NULL,
  project TEXT NOT NULL DEFAULT 'default',
  agent_name TEXT NOT NULL,
  confidence TEXT NOT NULL DEFAULT 'stated',
  version INTEGER NOT NULL DEFAULT 1,
  supersedes TEXT,
  conflict_with TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  archived_at TEXT,
  archived_by TEXT,
  layer TEXT NOT NULL DEFAULT 'behavior'
);
CREATE INDEX IF NOT EXISTS idx_memories_key_scope ON memories(project, scope, key) WHERE archived_at IS NULL;

CREATE TABLE IF NOT EXISTS orgs (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT UNIQUE NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
  name TEXT PRIMARY KEY,
  planet_type TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS teams (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  org_id TEXT,
  project TEXT NOT NULL DEFAULT 'default',
  description TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT 'regular',
  parent_team_id TEXT,
  created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_teams_project_slug ON teams(project, slug);

CREATE TABLE IF NOT EXISTS team_members (
  team_id TEXT NOT NULL,
  agent_name TEXT NOT NULL,
  project TEXT NOT NULL DEFAULT 'default',
  role TEXT NOT NULL DEFAULT 'member',
  joined_at TEXT NOT NULL,
  left_at TEXT,
  PRIMARY KEY (team_id, agent_name)
);

CREATE TABLE IF NOT EXISTS agent_notify_channels (
  agent_name TEXT NOT NULL,
  project TEXT NOT NULL DEFAULT 'default',
  target TEXT NOT NULL,
  PRIMARY KEY (agent_name, project, target)
);
`

// schemaTables is the set of tables migrate must create. The store's migration
// test asserts every one exists, guarding against a partial-DDL regression.
var schemaTables = []string{
	"agents", "profiles", "tasks", "messages", "deliveries",
	"message_reads", "memories", "orgs", "projects", "teams",
	"team_members", "agent_notify_channels",
}
