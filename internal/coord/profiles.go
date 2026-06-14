package coord

import "database/sql"

const profileColumns = "id, slug, name, role, context_pack, soul_keys, skills, vault_paths, allowed_tools, pool_size, project, org_id, created_at, updated_at"

func scanProfile(row interface{ Scan(...any) error }) (Profile, error) {
	var p Profile
	err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.Role, &p.ContextPack, &p.SoulKeys, &p.Skills,
		&p.VaultPaths, &p.AllowedTools, &p.PoolSize, &p.Project, &p.OrgID, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// registerProfile upserts on (project, slug). allowedTools/poolSize follow
// wrai.th's option gating: allowed_tools is applied only when non-empty and not
// "[]", pool_size only when > 0 — otherwise a new profile keeps the defaults
// ([], 3) and an existing one keeps its current values. created_at is preserved
// across upserts.
func (s *Store) registerProfile(project, slug, name, role, contextPack, soulKeys, skills, vaultPaths, allowedTools string, poolSize int) (*Profile, error) {
	now := nowMicro()
	if soulKeys == "" {
		soulKeys = "[]"
	}
	if skills == "" {
		skills = "[]"
	}
	if vaultPaths == "" {
		vaultPaths = "[]"
	}
	applyAllowed := allowedTools != "" && allowedTools != "[]"

	var result *Profile
	err := s.write(func(tx *sql.Tx) error {
		existing, err := scanProfile(tx.QueryRow("SELECT "+profileColumns+" FROM profiles WHERE slug = ? AND project = ?", slug, project))
		if err == sql.ErrNoRows {
			p := &Profile{
				ID: newID(), Slug: slug, Name: name, Role: role, ContextPack: contextPack,
				SoulKeys: soulKeys, Skills: skills, VaultPaths: vaultPaths,
				AllowedTools: "[]", PoolSize: 3, Project: project, CreatedAt: now, UpdatedAt: now,
			}
			if applyAllowed {
				p.AllowedTools = allowedTools
			}
			if poolSize > 0 {
				p.PoolSize = poolSize
			}
			if _, err := tx.Exec(
				"INSERT INTO profiles (id, slug, name, role, context_pack, soul_keys, skills, vault_paths, allowed_tools, pool_size, project, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				p.ID, p.Slug, p.Name, p.Role, p.ContextPack, p.SoulKeys, p.Skills, p.VaultPaths, p.AllowedTools, p.PoolSize, p.Project, p.CreatedAt, p.UpdatedAt); err != nil {
				return err
			}
			result = p
			return nil
		}
		if err != nil {
			return err
		}

		existing.Name = name
		existing.Role = role
		existing.ContextPack = contextPack
		existing.SoulKeys = soulKeys
		existing.Skills = skills
		existing.VaultPaths = vaultPaths
		existing.UpdatedAt = now
		if applyAllowed {
			existing.AllowedTools = allowedTools
		}
		if poolSize > 0 {
			existing.PoolSize = poolSize
		}
		if _, err := tx.Exec(
			"UPDATE profiles SET name = ?, role = ?, context_pack = ?, soul_keys = ?, skills = ?, vault_paths = ?, allowed_tools = ?, pool_size = ?, updated_at = ? WHERE slug = ? AND project = ?",
			existing.Name, existing.Role, existing.ContextPack, existing.SoulKeys, existing.Skills, existing.VaultPaths, existing.AllowedTools, existing.PoolSize, now, slug, project); err != nil {
			return err
		}
		result = &existing
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) getProfile(project, slug string) (*Profile, error) {
	p, err := scanProfile(s.reader().QueryRow("SELECT "+profileColumns+" FROM profiles WHERE slug = ? AND project = ?", slug, project))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func handleRegisterProfile(s *Server, args map[string]any) (toolResult, error) {
	slug := argString(args, "slug")
	if slug == "" {
		return resultError("slug is required"), nil
	}
	name := argString(args, "name")
	if name == "" {
		return resultError("name is required"), nil
	}
	p, err := s.store.registerProfile(
		resolveProject(args),
		slug,
		name,
		argString(args, "role"),
		argString(args, "context_pack"),
		argJSONArray(args, "soul_keys"),
		argJSONArray(args, "skills"),
		argJSONArray(args, "vault_paths"),
		argJSONArray(args, "allowed_tools"),
		argInt(args, "pool_size", 0),
	)
	if err != nil {
		return toolResult{}, err
	}
	// Bare profile object (no wrapper), still double-encoded in content[0].text.
	return resultText(p)
}
