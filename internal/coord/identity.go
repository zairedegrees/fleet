package coord

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// claudeProjectsDir is the directory whoami scans for transcripts. It is a var so
// tests can point it at a fixture directory.
var claudeProjectsDir = defaultClaudeProjectsDir

func defaultClaudeProjectsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

const priorityOrder = "CASE priority WHEN 'P0' THEN 0 WHEN 'P1' THEN 1 WHEN 'P2' THEN 2 WHEN 'P3' THEN 3 END"

func (s *Store) queryTasks(query string, args ...any) ([]Task, error) {
	rows, err := s.reader().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// getAgentTasks returns the agent's active work: tasks assigned to it plus
// unclaimed pending tasks for its profile (assignedToMe), and tasks it dispatched
// that aren't done (dispatchedByMe).
func (s *Store) getAgentTasks(project, agent string) (assignedToMe, dispatchedByMe []Task, err error) {
	assignedToMe, err = s.queryTasks(
		"SELECT "+taskColumns+" FROM tasks WHERE assigned_to = ? AND project = ? AND archived_at IS NULL AND status IN ('pending','accepted','in-progress') ORDER BY "+priorityOrder,
		agent, project)
	if err != nil {
		return nil, nil, err
	}
	pending, err := s.queryTasks(
		"SELECT "+taskColumns+" FROM tasks WHERE project = ? AND archived_at IS NULL AND status = 'pending' AND assigned_to IS NULL AND profile_slug IN (SELECT profile_slug FROM agents WHERE name = ? AND project = ? AND profile_slug IS NOT NULL) ORDER BY "+priorityOrder,
		project, agent, project)
	if err == nil {
		assignedToMe = append(assignedToMe, pending...)
	}
	dispatchedByMe, err = s.queryTasks(
		"SELECT "+taskColumns+" FROM tasks WHERE dispatched_by = ? AND project = ? AND archived_at IS NULL AND status != 'done' ORDER BY dispatched_at DESC",
		agent, project)
	if err != nil {
		return nil, nil, err
	}
	return assignedToMe, dispatchedByMe, nil
}

// listMemories returns the memories the agent authored in this project (any
// scope), most recently updated first — matching wrai.th's session_context
// ListMemories(project, "", agent, ...) author-filter.
func (s *Store) listMemories(project, agent string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.reader().Query(
		"SELECT "+memoryColumns+" FROM memories WHERE archived_at IS NULL AND project = ? AND agent_name = ? ORDER BY updated_at DESC LIMIT ?",
		project, agent, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	mems := []Memory{}
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		mems = append(mems, m)
	}
	return mems, rows.Err()
}

// sessionContext bundles what an agent needs to resume: its profile, pending
// tasks (assigned + dispatched), unread messages (full content), and relevant
// memories. (Conversations, goals and vault auto-injection are out of scope.)
func (s *Store) sessionContext(project, agent string, profileSlug *string) (map[string]any, error) {
	result := map[string]any{}

	if profileSlug != nil && *profileSlug != "" {
		if p, _ := s.getProfile(project, *profileSlug); p != nil {
			result["profile"] = p
		}
	}

	assigned, dispatched, err := s.getAgentTasks(project, agent)
	if err != nil {
		return nil, err
	}
	result["pending_tasks"] = map[string]any{
		"assigned_to_me":   assigned,
		"dispatched_by_me": dispatched,
	}

	// Full Message objects (not the truncated get_inbox entry shape), matching
	// wrai.th's buildSessionContext.
	unread, err := s.getInbox(project, agent, true, 50)
	if err != nil {
		return nil, err
	}
	result["unread_messages"] = unread

	mems, err := s.listMemories(project, agent, 20)
	if err != nil {
		return nil, err
	}
	result["relevant_memories"] = mems

	return result, nil
}

// --- handlers ---

func handleWhoami(s *Server, args map[string]any) (toolResult, error) {
	salt := argString(args, "salt")
	if salt == "" {
		return resultError("salt is required"), nil
	}
	if len(salt) < 5 {
		return resultError("salt too short — use at least 3 random words"), nil
	}

	dir := claudeProjectsDir()
	if dir == "" {
		return resultError("cannot determine home dir"), nil
	}

	var matchFile string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if matchFile != "" {
			return filepath.SkipAll
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer func() { _ = f.Close() }()

		// Salt is in recent lines — scan the last 64KB.
		if stat, _ := f.Stat(); stat != nil && stat.Size() > 65536 {
			_, _ = f.Seek(stat.Size()-65536, 0)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 256*1024), 256*1024)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), salt) {
				matchFile = path
				return filepath.SkipAll
			}
		}
		return nil
	})

	if matchFile == "" {
		return resultError("salt not found in any transcript — make sure you wrote the salt in your conversation before calling whoami"), nil
	}

	sessionID := strings.TrimSuffix(filepath.Base(matchFile), ".jsonl")
	return resultText(map[string]any{
		"session_id":      sessionID,
		"transcript_path": matchFile,
	})
}

func handleGetSessionContext(s *Server, args map[string]any) (toolResult, error) {
	project := resolveProject(args)
	agent := resolveAgent(args)
	profileSlug := optionalString(argString(args, "profile_slug"))

	// Auto-detect the profile from the registered agent if not provided.
	if profileSlug == nil {
		if a, _ := s.store.getAgent(project, agent); a != nil {
			profileSlug = a.ProfileSlug
		}
	}

	ctx, err := s.store.sessionContext(project, agent, profileSlug)
	if err != nil {
		return toolResult{}, err
	}
	ctx["agent"] = agent
	ctx["project"] = project
	return resultText(ctx)
}
