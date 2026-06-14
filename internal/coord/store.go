package coord

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// maxReaders bounds the read-only connection pool. Writes are serialized
// through a single connection under writeMu; reads run concurrently against the
// WAL.
const maxReaders = 10

// Timestamp layouts the relay wire depends on. agents.registered_at/last_seen
// and message_reads.read_at are RFC3339; messages/deliveries/tasks/memories use
// the microsecond layout. Reproducing these exactly keeps responses
// byte-comparable with wrai.th.
const (
	timeRFC3339 = time.RFC3339
	timeMicro   = "2006-01-02T15:04:05.000000Z"
)

func nowRFC3339() string { return time.Now().UTC().Format(timeRFC3339) }
func nowMicro() string   { return time.Now().UTC().Format(timeMicro) }

// Store owns the SQLite database backing coord. It enforces the single-writer
// discipline modernc.org/sqlite needs: every write goes through wdb (capped at
// one open connection) while writeMu is held for the whole transaction, so a
// multi-statement write (e.g. register_agent's read-then-update preserve, or
// dispatch's insert-task-then-fan-out-notifications) can never interleave with
// another writer. Reads use a separate pool against the WAL.
type Store struct {
	path    string
	wdb     *sql.DB
	rdb     *sql.DB
	writeMu sync.Mutex
}

func writerDSN(path string) string {
	// mode=rwc is explicit: the read-only reader pool hard-depends on the file
	// already existing, so the writer must be the one that creates it.
	return "file:" + path +
		"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&mode=rwc"
}

func readerDSN(path string) string {
	return "file:" + path + "?mode=ro&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)"
}

// OpenStore opens (creating if needed) the database at path, applies the schema,
// and returns a ready Store. The writer is opened and migrated first so the
// read-only pool always attaches to an existing, fully-migrated file.
func OpenStore(path string) (*Store, error) {
	wdb, err := sql.Open("sqlite", writerDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	wdb.SetMaxOpenConns(1)

	if _, err := wdb.Exec(schemaDDL); err != nil {
		wdb.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	rdb, err := sql.Open("sqlite", readerDSN(path))
	if err != nil {
		wdb.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}
	rdb.SetMaxOpenConns(maxReaders)

	return &Store{path: path, wdb: wdb, rdb: rdb}, nil
}

// Close releases both connection pools.
func (s *Store) Close() error {
	var firstErr error
	if s.rdb != nil {
		if err := s.rdb.Close(); err != nil {
			firstErr = err
		}
	}
	if s.wdb != nil {
		if err := s.wdb.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// reader returns the read-only connection pool for queries.
func (s *Store) reader() *sql.DB { return s.rdb }

// write runs fn inside a single serialized write transaction. writeMu is held
// for the entire BEGIN..COMMIT so multi-statement writes stay atomic against
// other writers; fn's error rolls back.
func (s *Store) write(fn func(*sql.Tx) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.wdb.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
