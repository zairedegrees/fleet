package coord

import "database/sql"

const profileColumns = "id, slug, name, role, context_pack, soul_keys, skills, vault_paths, allowed_tools, pool_size, project, org_id, created_at, updated_at"

func scanProfile(row interface{ Scan(...any) error }) (Profile, error) {
	var p Profile
	err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.Role, &p.ContextPack, &p.SoulKeys, &p.Skills,
		&p.VaultPaths, &p.AllowedTools, &p.PoolSize, &p.Project, &p.OrgID, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// registerProfile upserts on (project, slug): a new profile seeds allowed_tools
// [] and pool_size 3; an existing one updates its fields while preserving
// created_at.
func (s *Store) registerProfile(project, slug, name, role, contextPack, soulKeys, skills, vaultPaths string) (*Profile, error) {
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

	var result *Profile
	err := s.write(func(tx *sql.Tx) error {
		existing, err := scanProfile(tx.QueryRow("SELECT "+profileColumns+" FROM profiles WHERE slug = ? AND project = ?", slug, project))
		if err == sql.ErrNoRows {
			p := &Profile{
				ID: newID(), Slug: slug, Name: name, Role: role, ContextPack: contextPack,
				SoulKeys: soulKeys, Skills: skills, VaultPaths: vaultPaths,
				AllowedTools: "[]", PoolSize: 3, Project: project, CreatedAt: now, UpdatedAt: now,
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
		if _, err := tx.Exec(
			"UPDATE profiles SET name = ?, role = ?, context_pack = ?, soul_keys = ?, skills = ?, vault_paths = ?, updated_at = ? WHERE slug = ? AND project = ?",
			existing.Name, existing.Role, existing.ContextPack, existing.SoulKeys, existing.Skills, existing.VaultPaths, now, slug, project); err != nil {
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

func handleRegisterProfile(s *Server, args map[string]any) (toolResult, error) {
	slug := argString(args, "slug")
	if slug == "" {
		return resultError("slug is required"), nil
	}
	p, err := s.store.registerProfile(
		resolveProject(args),
		slug,
		argString(args, "name"),
		argString(args, "role"),
		argString(args, "context_pack"),
		argStringDefault(args, "soul_keys", "[]"),
		argStringDefault(args, "skills", "[]"),
		argStringDefault(args, "vault_paths", "[]"),
	)
	if err != nil {
		return toolResult{}, err
	}
	// Bare profile object (no wrapper), still double-encoded in content[0].text.
	return resultText(p)
}
