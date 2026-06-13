package coord

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// newTestStore opens a coord store on a throwaway temp database.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := OpenStore(filepath.Join(t.TempDir(), "coord.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestMigrateCreatesAllTables(t *testing.T) {
	st := newTestStore(t)
	for _, tbl := range schemaTables {
		var name string
		err := st.reader().QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Fatalf("table %q missing after migrate: %v", tbl, err)
		}
	}

	// The identity columns the wire contract depends on must exist, or task
	// routing and the register preserve-on-omit silently break.
	cols := tableColumns(t, st, "agents")
	for _, c := range []string{
		"profile_slug", "reports_to", "is_executive", "session_id",
		"interest_tags", "max_context_bytes", "status", "deactivated_at",
	} {
		if !cols[c] {
			t.Errorf("agents table missing column %q", c)
		}
	}

	// teams carries description + parent_team_id: the register_agent is_executive
	// side-effect inserts/selects both (wrai.th CreateTeam), so a missing column
	// would break that later wave silently.
	teamCols := tableColumns(t, st, "teams")
	for _, c := range []string{"description", "parent_team_id", "type", "slug"} {
		if !teamCols[c] {
			t.Errorf("teams table missing column %q", c)
		}
	}
}

// TestReaderPoolPragmas asserts the per-connection pragmas (busy_timeout,
// foreign_keys) are applied on EVERY reader connection, not just the first.
// journal_mode is file-level and can't prove this; busy_timeout is what keeps a
// read during a long write from failing instead of waiting. Holding maxReaders
// connections at once forces distinct pooled conns so a per-conn pragma loss is
// caught.
func TestReaderPoolPragmas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	conns := make([]*sql.Conn, 0, maxReaders)
	for i := 0; i < maxReaders; i++ {
		c, err := st.reader().Conn(ctx)
		if err != nil {
			t.Fatalf("conn %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()

	for i, c := range conns {
		var busy, fk int
		if err := c.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busy); err != nil {
			t.Fatalf("conn %d busy_timeout: %v", i, err)
		}
		if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
			t.Fatalf("conn %d foreign_keys: %v", i, err)
		}
		if busy != 10000 {
			t.Errorf("conn %d busy_timeout = %d, want 10000", i, busy)
		}
		if fk != 1 {
			t.Errorf("conn %d foreign_keys = %d, want 1", i, fk)
		}
	}
}

// TestConcurrentWritesNoBusy validates the store's reason for existing: the
// single-writer-under-Mutex + RO reader pool must not surface "database is
// locked" under concurrent writes and reads, and must not lose updates. This is
// the foundation-level smoke test; the handler-driven dispatch fan-out test
// (T17) comes later.
func TestConcurrentWritesNoBusy(t *testing.T) {
	st := newTestStore(t)
	const n = 20

	var wg sync.WaitGroup
	errs := make(chan error, n+5)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := st.write(func(tx *sql.Tx) error {
				_, e := tx.Exec(
					`INSERT INTO orgs (id, name, slug, description, created_at) VALUES (?,?,?,?,?)`,
					fmt.Sprintf("o%d", i), fmt.Sprintf("org %d", i), fmt.Sprintf("slug-%d", i), "", nowRFC3339())
				return e
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				var c int
				if err := st.reader().QueryRow(`SELECT COUNT(*) FROM orgs`).Scan(&c); err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent op failed: %v", err)
	}

	var count int
	if err := st.reader().QueryRow(`SELECT COUNT(*) FROM orgs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != n {
		t.Errorf("final orgs count = %d, want %d (lost updates)", count, n)
	}
}

func TestWALEnabled(t *testing.T) {
	st := newTestStore(t)
	var mode string
	if err := st.wdb.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func tableColumns(t *testing.T, st *Store, table string) map[string]bool {
	t.Helper()
	rows, err := st.reader().Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer rows.Close()

	cols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		cols[name] = true
	}
	return cols
}
