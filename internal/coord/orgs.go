package coord

// org mirrors a row of the orgs table as the wire returns it.
type org struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

// handleListOrgs backs the health probe (fleet calls list_orgs to check the
// relay is up). It MUST return a successful result with a non-empty content[0]
// even against an empty database, so a freshly-migrated coord reports healthy.
func handleListOrgs(s *Server, args map[string]any) (toolResult, error) {
	rows, err := s.store.reader().Query(
		`SELECT id, name, slug, description, created_at FROM orgs ORDER BY name`)
	if err != nil {
		return toolResult{}, err
	}
	defer rows.Close()

	orgs := []org{}
	for rows.Next() {
		var o org
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.Description, &o.CreatedAt); err != nil {
			return toolResult{}, err
		}
		orgs = append(orgs, o)
	}
	if err := rows.Err(); err != nil {
		return toolResult{}, err
	}

	return resultText(map[string]any{"count": len(orgs), "orgs": orgs})
}
