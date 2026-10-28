package store

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
)

// Groundwork for syncing between devices (CloudKit, later). Every record has
// a uid besides its id: the id is this database's own number, which MCP, the
// API and the window use; the uid names the same record on every device.
// Each change to a record can be noted in sync_outbox, by triggers in the
// database itself, so a change made by any process (the window, `serve`,
// the stdio MCP server, an older version of the app) is caught. Nothing is
// noted until sync is turned on (sync_state "enabled" = "1"): turning it on
// will send everything once.
//
// The uid is kept out of the records the store returns (see query): the
// API, MCP and the window have the id.

// syncTables are the tables whose rows sync: every kind of record and the
// attachments.
func syncTables() []string {
	out := make([]string, 0, len(Entities)+1)
	for _, e := range Entities {
		out = append(out, e.Name)
	}
	return append(out, "attachments")
}

// a uid is 32 random hex digits, made by the database when a row goes in
const uidSQL = "lower(hex(randomblob(16)))"

var uidRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ValidUID tells whether s has the shape of a uid.
func ValidUID(s string) bool { return uidRe.MatchString(s) }

// triggerVersion is in the triggers' names. Changing what a trigger does
// means a new version: the old ones are dropped by name.
const triggerVersion = 1

// migrateSync adds the uid to tables made before it, gives every row one,
// and (re)creates the sync tables and triggers. Run after the tables exist.
func (s *Store) migrateSync() error {
	if _, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS sync_outbox (
		entity TEXT NOT NULL,
		uid TEXT NOT NULL,
		op TEXT NOT NULL,
		at TEXT NOT NULL,
		PRIMARY KEY (entity, uid)
	);
	CREATE TABLE IF NOT EXISTS sync_state (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`); err != nil {
		return err
	}
	for _, t := range syncTables() {
		have, err := s.columns(t)
		if err != nil {
			return err
		}
		if !have["uid"] {
			if _, err := s.DB.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN uid TEXT", t)); err != nil {
				return fmt.Errorf("%s.uid: %w", t, err)
			}
		}
		if _, err := s.DB.Exec(fmt.Sprintf("UPDATE %s SET uid = %s WHERE uid IS NULL", t, uidSQL)); err != nil {
			return fmt.Errorf("%s.uid: %w", t, err)
		}
		if _, err := s.DB.Exec(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS idx_%s_uid ON %s(uid)", t, t)); err != nil {
			return fmt.Errorf("%s.uid: %w", t, err)
		}
		if err := s.triggers(t); err != nil {
			return fmt.Errorf("%s triggers: %w", t, err)
		}
	}
	return nil
}

// triggers makes t's triggers, unless this version of them is there already
// (each process opening the database would otherwise rewrite the schema).
func (s *Store) triggers(t string) error {
	name := func(kind string) string { return fmt.Sprintf("sync%d_%s_%s", triggerVersion, t, kind) }
	var n int
	if err := s.DB.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name = ?", name("del")).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// older versions' triggers
	for v := 0; v < triggerVersion; v++ {
		for _, kind := range []string{"uid", "ins", "upd", "del"} {
			if _, err := tx.Exec(fmt.Sprintf("DROP TRIGGER IF EXISTS sync%d_%s_%s", v, t, kind)); err != nil {
				return err
			}
		}
	}
	on := `(SELECT value FROM sync_state WHERE key = 'enabled') = '1'`
	note := func(op, row string) string {
		return fmt.Sprintf(`INSERT OR REPLACE INTO sync_outbox (entity, uid, op, at)
			VALUES ('%s', %s.uid, '%s', strftime('%%Y-%%m-%%d %%H:%%M:%%S', 'now', 'localtime'));`, t, row, op)
	}
	for _, q := range []string{
		// a row written without a uid (an older app, a restored backup) gets
		// one; the update below notes it
		fmt.Sprintf(`CREATE TRIGGER %s AFTER INSERT ON %s WHEN NEW.uid IS NULL
			BEGIN UPDATE %s SET uid = %s WHERE id = NEW.id; END`, name("uid"), t, t, uidSQL),
		fmt.Sprintf(`CREATE TRIGGER %s AFTER INSERT ON %s WHEN NEW.uid IS NOT NULL AND %s
			BEGIN %s END`, name("ins"), t, on, note("save", "NEW")),
		fmt.Sprintf(`CREATE TRIGGER %s AFTER UPDATE ON %s WHEN NEW.uid IS NOT NULL AND %s
			BEGIN %s END`, name("upd"), t, on, note("save", "NEW")),
		fmt.Sprintf(`CREATE TRIGGER %s AFTER DELETE ON %s WHEN OLD.uid IS NOT NULL AND %s
			BEGIN %s END`, name("del"), t, on, note("delete", "OLD")),
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UID is the uid of record id of entity.
func (s *Store) UID(entityName string, id int64) (string, error) {
	t, err := syncTable(entityName)
	if err != nil {
		return "", err
	}
	var uid sql.NullString
	err = s.DB.QueryRow("SELECT uid FROM "+t+" WHERE id = ?", id).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return uid.String, err
}

// IDByUID is this database's id of the record of entity with uid.
func (s *Store) IDByUID(entityName, uid string) (int64, error) {
	t, err := syncTable(entityName)
	if err != nil {
		return 0, err
	}
	if !ValidUID(uid) {
		return 0, ErrNotFound
	}
	var id int64
	err = s.DB.QueryRow("SELECT id FROM "+t+" WHERE uid = ?", uid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

func syncTable(name string) (string, error) {
	if name == "attachments" {
		return name, nil
	}
	e, err := entity(name)
	if err != nil {
		return "", err
	}
	return e.Name, nil
}

// IsSyncTable tells whether a table holds sync's own state, which a backup
// doesn't restore: it describes this device and the cloud now, not then.
func IsSyncTable(name string) bool { return name == "sync_outbox" || name == "sync_state" }

// MarkRestored is for the end of a restore, in its transaction: the records
// were all replaced, so what was waiting to be sent no longer says what
// changed, and the next sync has to start over from this data ("reset").
func MarkRestored(tx *sql.Tx) error {
	if _, err := tx.Exec("DELETE FROM main.sync_outbox"); err != nil {
		return err
	}
	_, err := tx.Exec("INSERT OR REPLACE INTO main.sync_state (key, value) VALUES ('reset', '1')")
	return err
}
