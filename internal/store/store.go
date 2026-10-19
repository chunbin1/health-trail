// Package store keeps every health record in one SQLite file.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Record = map[string]any

// ErrNotFound is returned when a record id doesn't exist.
var ErrNotFound = errors.New("记录不存在")

// ValidationError is a problem with the input, worded for whoever typed it
// (a person in the UI, or Claude through MCP).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return &ValidationError{fmt.Sprintf(format, a...)} }

type Store struct {
	DB      *sql.DB
	Dir     string // data directory: health.db and files/
	FileDir string
}

// Open opens (and creates or migrates) the database in dir. The desktop app,
// `serve` and the stdio MCP server can each have it open at once: WAL mode
// lets them read while one writes, and busy_timeout waits out the writer.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o700); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.Join(dir, "health.db") +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	s := &Store{DB: db, Dir: dir, FileDir: filepath.Join(dir, "files")}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func sqlType(f Field) string {
	switch f.Type {
	case Int, Ref, Bool:
		return "INTEGER"
	case Real:
		return "REAL"
	default:
		return "TEXT"
	}
}

func columnDef(f Field) string {
	def := f.Name + " " + sqlType(f)
	if f.Type == Ref {
		action := "SET NULL"
		if f.Cascade {
			action = "CASCADE"
		}
		def += fmt.Sprintf(" REFERENCES %s(id) ON DELETE %s", f.Ref, action)
	}
	return def
}

// migrate creates missing tables and adds missing columns. Columns are never
// dropped or retyped here: a field removed from the schema keeps its data.
func (s *Store) migrate() error {
	for _, e := range Entities {
		cols := []string{"id INTEGER PRIMARY KEY AUTOINCREMENT"}
		for _, f := range e.Fields {
			cols = append(cols, columnDef(f))
		}
		cols = append(cols, "created_at TEXT NOT NULL", "updated_at TEXT NOT NULL")
		if len(e.Unique) > 0 {
			cols = append(cols, "UNIQUE("+strings.Join(e.Unique, ",")+")")
		}
		if _, err := s.DB.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", e.Name, strings.Join(cols, ", "))); err != nil {
			return fmt.Errorf("%s: %w", e.Name, err)
		}
		have, err := s.columns(e.Name)
		if err != nil {
			return err
		}
		for _, f := range e.Fields {
			if !have[f.Name] {
				if _, err := s.DB.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", e.Name, columnDef(f))); err != nil {
					return fmt.Errorf("%s.%s: %w", e.Name, f.Name, err)
				}
			}
		}
		if e.DateKey != "" {
			s.DB.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s)", e.Name, e.DateKey, e.Name, e.DateKey))
		}
		for _, f := range e.Fields {
			if f.Type == Ref {
				s.DB.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s)", e.Name, f.Name, e.Name, f.Name))
			}
		}
	}
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS attachments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner_type TEXT NOT NULL,
		owner_id INTEGER NOT NULL,
		filename TEXT NOT NULL,
		mime TEXT,
		size INTEGER,
		stored_name TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_attachments_owner ON attachments(owner_type, owner_id);
	CREATE INDEX IF NOT EXISTS idx_lab_item ON lab_results(item_name);
	CREATE INDEX IF NOT EXISTS idx_metrics_type ON metrics(metric_type, measured_at);`)
	return err
}

func (s *Store) columns(table string) (map[string]bool, error) {
	rows, err := s.DB.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		have[n] = true
	}
	return have, rows.Err()
}

func now() string { return time.Now().Format("2006-01-02 15:04:05") }

func entity(name string) (*Entity, error) {
	e := EntityByName(name)
	if e == nil {
		return nil, invalid("没有 %q 这种记录类型，可用的有：%s", name, strings.Join(EntityNames(), ", "))
	}
	return e, nil
}

// clean turns raw input into column values: unknown keys are rejected,
// values are converted to the field's type, and blank strings become NULL.
func (s *Store) clean(e *Entity, in Record) (Record, error) {
	out := Record{}
	for k, v := range in {
		if k == "id" || k == "created_at" || k == "updated_at" {
			continue
		}
		f := e.Field(k)
		if f == nil {
			names := make([]string, len(e.Fields))
			for i, f := range e.Fields {
				names[i] = f.Name
			}
			return nil, invalid("%s 没有字段 %q，可用字段：%s", e.Label, k, strings.Join(names, ", "))
		}
		cv, err := convert(*f, v)
		if err != nil {
			return nil, invalid("%s「%s」：%v", e.Label, f.Label, err)
		}
		if cv != nil && f.Type == Ref {
			if err := s.mustExist(f.Ref, cv.(int64)); err != nil {
				return nil, invalid("%s「%s」：%s #%d 不存在", e.Label, f.Label, EntityByName(f.Ref).Label, cv)
			}
		}
		out[k] = cv
	}
	return out, nil
}

func (s *Store) mustExist(table string, id int64) error {
	var one int
	return s.DB.QueryRow("SELECT 1 FROM "+table+" WHERE id = ?", id).Scan(&one)
}

func required(e *Entity, r Record) error {
	var missing []string
	for _, f := range e.Fields {
		if f.Required && r[f.Name] == nil {
			missing = append(missing, f.Label+"("+f.Name+")")
		}
	}
	if len(missing) > 0 {
		return invalid("%s缺少必填项：%s", e.Label, strings.Join(missing, "、"))
	}
	return nil
}

// prepare fills in what can be derived from the other fields.
func prepare(e *Entity, r Record) {
	switch e.Name {
	case "lab_results":
		prepareLab(r)
	case "habits":
		// a habit that logs a metric asks for it in the metric's unit
		if r["unit"] == nil {
			if t, ok := r["metric_type"].(string); ok && MetricUnits[t] != "" {
				r["unit"] = MetricUnits[t]
			}
		}
	case "metrics":
		if r["unit"] == nil {
			if t, ok := r["metric_type"].(string); ok && MetricUnits[t] != "" {
				r["unit"] = MetricUnits[t]
			}
		}
	}
}

func (s *Store) build(e *Entity, in Record) (Record, error) {
	r, err := s.clean(e, in)
	if err != nil {
		return nil, err
	}
	prepare(e, r)
	if err := required(e, r); err != nil {
		return nil, err
	}
	ts := now()
	r["created_at"], r["updated_at"] = ts, ts
	return r, nil
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func insert(ex execer, e *Entity, r Record) (int64, error) {
	keys := sortedKeys(r)
	vals := make([]any, len(keys))
	for i, k := range keys {
		vals[i] = r[k]
	}
	res, err := ex.Exec(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", e.Name,
		strings.Join(keys, ","), strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")), vals...)
	if err != nil {
		return 0, dbErr(e, err)
	}
	return res.LastInsertId()
}

func (s *Store) Create(entityName string, in Record) (Record, error) {
	e, err := entity(entityName)
	if err != nil {
		return nil, err
	}
	r, err := s.build(e, in)
	if err != nil {
		return nil, err
	}
	id, err := insert(s.DB, e, r)
	if err != nil {
		return nil, err
	}
	return s.Get(e.Name, id)
}

// CreateMany adds all the records or, if any is invalid, none of them. An
// error names the 1-based position of the record it is about.
func (s *Store) CreateMany(entityName string, ins []Record) ([]Record, error) {
	e, err := entity(entityName)
	if err != nil {
		return nil, err
	}
	built := make([]Record, len(ins))
	for i, in := range ins {
		if built[i], err = s.build(e, in); err != nil {
			return nil, prefixErr(fmt.Sprintf("第 %d 条：", i+1), err)
		}
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ids := make([]int64, len(built))
	for i, r := range built {
		if ids[i], err = insert(tx, e, r); err != nil {
			return nil, prefixErr(fmt.Sprintf("第 %d 条：", i+1), err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	out := make([]Record, len(ids))
	for i, id := range ids {
		if out[i], err = s.Get(e.Name, id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func prefixErr(p string, err error) error {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return invalid("%s%s", p, ve.Msg)
	}
	return err
}

func (s *Store) Update(entityName string, id int64, patch Record) (Record, error) {
	e, err := entity(entityName)
	if err != nil {
		return nil, err
	}
	cur, err := s.Get(e.Name, id)
	if err != nil {
		return nil, err
	}
	p, err := s.clean(e, patch)
	if err != nil {
		return nil, err
	}
	if len(p) == 0 {
		return cur, nil
	}
	merged := Record{}
	for _, f := range e.Fields {
		merged[f.Name] = cur[f.Name]
	}
	for k, v := range p {
		merged[k] = v
	}
	if e.Name == "lab_results" {
		// a changed value or range re-derives what came from it, unless the
		// patch sets that itself
		_, valueChanged := p["value_num"]
		_, textChanged := p["value_text"]
		_, rangeChanged := p["ref_range"]
		if _, ok := p["flag"]; !ok && (valueChanged || textChanged || rangeChanged) {
			merged["flag"] = nil
		}
		if rangeChanged {
			if _, ok := p["ref_low"]; !ok {
				merged["ref_low"] = nil
			}
			if _, ok := p["ref_high"]; !ok {
				merged["ref_high"] = nil
			}
		}
	}
	prepare(e, merged)
	if err := required(e, merged); err != nil {
		return nil, err
	}
	merged["updated_at"] = now()
	keys := sortedKeys(merged)
	sets := make([]string, len(keys))
	vals := make([]any, 0, len(keys)+1)
	for i, k := range keys {
		sets[i] = k + " = ?"
		vals = append(vals, merged[k])
	}
	vals = append(vals, id)
	if _, err := s.DB.Exec(fmt.Sprintf("UPDATE %s SET %s WHERE id = ?", e.Name, strings.Join(sets, ", ")), vals...); err != nil {
		return nil, dbErr(e, err)
	}
	return s.Get(e.Name, id)
}

func (s *Store) Delete(entityName string, id int64) error {
	e, err := entity(entityName)
	if err != nil {
		return err
	}
	// attachments only hang off records a person opens (reports, visits…),
	// not off the rows a cascade removes, so the record's own are enough
	att, err := s.Attachments(e.Name, id)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec("DELETE FROM "+e.Name+" WHERE id = ?", id)
	if err != nil {
		return dbErr(e, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec("DELETE FROM attachments WHERE owner_type = ? AND owner_id = ?", e.Name, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, a := range att {
		os.Remove(s.AttachmentPath(a))
	}
	return nil
}

func (s *Store) Get(entityName string, id int64) (Record, error) {
	e, err := entity(entityName)
	if err != nil {
		return nil, err
	}
	rows, err := s.query("SELECT * FROM "+e.Name+" WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return rows[0], nil
}

type ListQuery struct {
	Q       string         // text search over text fields
	From    string         // inclusive, on the entity's date field
	To      string         // inclusive
	Filters map[string]any // field = value
	Limit   int
	Offset  int
	Asc     bool
}

func (s *Store) List(entityName string, q ListQuery) ([]Record, int, error) {
	e, err := entity(entityName)
	if err != nil {
		return nil, 0, err
	}
	var where []string
	var args []any
	if q.Q != "" {
		var ors []string
		for _, f := range e.Fields {
			if f.Type == Text || f.Type == LongText || f.Type == Enum {
				ors = append(ors, f.Name+" LIKE ?")
				args = append(args, "%"+q.Q+"%")
			}
		}
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}
	if e.DateKey != "" {
		if q.From != "" {
			d, err := normalizeDate(q.From)
			if err != nil {
				return nil, 0, invalid("from：%v", err)
			}
			where = append(where, "substr("+e.DateKey+",1,10) >= ?")
			args = append(args, d)
		}
		if q.To != "" {
			d, err := normalizeDate(q.To)
			if err != nil {
				return nil, 0, invalid("to：%v", err)
			}
			where = append(where, "substr("+e.DateKey+",1,10) <= ?")
			args = append(args, d)
		}
	}
	for k, v := range q.Filters {
		f := e.Field(k)
		if f == nil {
			return nil, 0, invalid("%s 没有字段 %q", e.Label, k)
		}
		cv, err := convert(*f, v)
		if err != nil {
			return nil, 0, invalid("%s：%v", f.Label, err)
		}
		if cv == nil {
			where = append(where, k+" IS NULL")
		} else {
			where = append(where, k+" = ?")
			args = append(args, cv)
		}
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM "+e.Name+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := "id"
	if e.DateKey != "" {
		order = e.DateKey
	}
	dir := "DESC"
	if q.Asc {
		dir = "ASC"
	}
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 200
	}
	rows, err := s.query(fmt.Sprintf("SELECT * FROM %s%s ORDER BY %s %s, id %s LIMIT %d OFFSET %d",
		e.Name, cond, order, dir, dir, q.Limit, q.Offset), args...)
	return rows, total, err
}

// query runs a SELECT and returns rows as maps, with SQLite's []byte text
// turned into strings so the rows encode as JSON text.
func (s *Store) query(sqlText string, args ...any) ([]Record, error) {
	rows, err := s.DB.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []Record{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := Record{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				r[c] = string(b)
			} else {
				r[c] = vals[i]
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func dbErr(e *Entity, err error) error {
	msg := err.Error()
	if strings.Contains(msg, "UNIQUE constraint failed") {
		if e.Name == "checkins" {
			return invalid("这个打卡项当天已经打过卡了")
		}
		return invalid("%s已存在同名记录", e.Label)
	}
	if strings.Contains(msg, "FOREIGN KEY constraint failed") {
		return invalid("关联的记录不存在")
	}
	return err
}

func sortedKeys(r Record) []string {
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
