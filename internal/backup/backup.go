// Package backup copies the data folder (the database and the attachments)
// into one zip file in a folder the person picks, keeps the latest few, and
// restores from one.
package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lee/health-vault/internal/store"
)

// Prefix starts every backup's file name; only files with it are listed,
// pruned or restored, so other files in the folder are left alone. Names are
// plain ASCII: cloud drives, Windows and terminals all take them as they are.
const Prefix = "health-vault-"

// BeforeRestore marks the backup made of the current data before a restore.
const BeforeRestore = "-before-restore"

// legacyPrefix is what the first backups were called; they still list and
// restore.
const legacyPrefix = "健康档案-"

const dbName = "health.db"

// Entry is one backup file in the folder.
type Entry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Time string `json:"time"` // YYYY-MM-DD HH:MM:SS, from the name
}

// CheckDir refuses a folder a backup can't go in: it must be an absolute
// path, and outside the data folder, which a backup copies.
func CheckDir(st *store.Store, dir string) error {
	if dir == "" {
		return errors.New("还没有设置备份位置")
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("备份位置要写完整路径（以 / 开头），收到 %q", dir)
	}
	data, _ := filepath.Abs(st.Dir)
	if rel, err := filepath.Rel(data, filepath.Clean(dir)); err == nil && !strings.HasPrefix(rel, "..") {
		return errors.New("备份位置不能放在数据目录里面，换一个文件夹")
	}
	return nil
}

// Create writes a backup of st into dir as health-vault-<time><suffix>.zip and
// returns its name. The database is copied with VACUUM INTO, a consistent
// snapshot even while the app or Claude is writing.
func Create(st *store.Store, dir, suffix string, now time.Time) (string, error) {
	if err := CheckDir(st, dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("无法创建备份文件夹：%w", err)
	}
	tmp, err := os.MkdirTemp("", "health-vault-backup-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	snap := filepath.Join(tmp, dbName)
	if _, err := st.DB.Exec("VACUUM INTO ?", snap); err != nil {
		return "", fmt.Errorf("复制数据库失败：%w", err)
	}

	name := Prefix + now.Format("2006-01-02-150405") + suffix + ".zip"
	final := filepath.Join(dir, name)
	part := final + ".part" // a half-written backup never looks like a real one
	f, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("无法写入备份位置：%w", err)
	}
	err = writeZip(f, snap, st.FileDir)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(part, final)
	}
	if err != nil {
		os.Remove(part)
		return "", fmt.Errorf("写入备份失败：%w", err)
	}
	return name, nil
}

func writeZip(w io.Writer, db, files string) error {
	z := zip.NewWriter(w)
	if err := addFile(z, db, dbName); err != nil {
		return err
	}
	err := filepath.WalkDir(files, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(files, p)
		return addFile(z, p, "files/"+filepath.ToSlash(rel))
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return z.Close()
}

func addFile(z *zip.Writer, path, name string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	h, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	h.Name, h.Method = name, zip.Deflate
	dst, err := z.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}

// List returns the backups in dir, newest first.
func List(dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Entry{}
	for _, d := range des {
		n := d.Name()
		prefix := Prefix
		if strings.HasPrefix(n, legacyPrefix) {
			prefix = legacyPrefix
		}
		if d.IsDir() || !strings.HasPrefix(n, prefix) || !strings.HasSuffix(n, ".zip") {
			continue
		}
		info, err := d.Info()
		if err != nil {
			continue
		}
		e := Entry{Name: n, Size: info.Size(), Time: info.ModTime().Format("2006-01-02 15:04:05")}
		stamp := strings.TrimPrefix(n, prefix)
		if len(stamp) >= 17 {
			if t, err := time.ParseInLocation("2006-01-02-150405", stamp[:17], time.Local); err == nil {
				e.Time = t.Format("2006-01-02 15:04:05")
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Time != out[j].Time {
			return out[i].Time > out[j].Time
		}
		return out[i].Name > out[j].Name // a fixed order within one second
	})
	return out, nil
}

// Prune deletes all but the newest keep backups in dir.
func Prune(dir string, keep int) error {
	if keep < 1 {
		return nil
	}
	es, err := List(dir)
	if err != nil {
		return err
	}
	for _, e := range es[min(keep, len(es)):] {
		if err := os.Remove(filepath.Join(dir, e.Name)); err != nil {
			return err
		}
	}
	return nil
}

// Restore replaces st's records with those in the backup at path, and puts
// back its attachment files. The database file stays where it is and is
// rewritten in one transaction, so the app, Claude and anything else with it
// open simply see the restored data, and a failure changes nothing. A backup
// from an older version, with fewer columns, restores into the columns it
// has. The caller backs up the current data first.
func Restore(st *store.Store, path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("打不开这个备份：%w", err)
	}
	defer zr.Close()
	tmp, err := os.MkdirTemp("", "health-vault-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	var db *zip.File
	var files []*zip.File
	for _, f := range zr.File {
		switch {
		case f.Name == dbName:
			db = f
		case strings.HasPrefix(f.Name, "files/") && !f.FileInfo().IsDir():
			if _, err := safeRel(strings.TrimPrefix(f.Name, "files/")); err != nil {
				return err
			}
			files = append(files, f)
		}
	}
	if db == nil {
		return errors.New("这不是健康档案的备份：里面没有 health.db")
	}
	snap := filepath.Join(tmp, dbName)
	if err := extract(db, snap); err != nil {
		return err
	}
	if err := check(snap); err != nil {
		return err
	}
	if err := copyRecords(st, snap); err != nil {
		return err
	}
	for _, f := range files {
		rel, _ := safeRel(strings.TrimPrefix(f.Name, "files/"))
		if err := extract(f, filepath.Join(st.FileDir, rel)); err != nil {
			return fmt.Errorf("恢复附件 %s 失败：%w", rel, err)
		}
	}
	return nil
}

// safeRel keeps an attachment path inside the files folder.
func safeRel(name string) (string, error) {
	rel := filepath.Clean(filepath.FromSlash(name))
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("备份里有不安全的文件路径：%q", name)
	}
	return rel, nil
}

func extract(f *zip.File, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return err
	}
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

func check(path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var res string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&res); err != nil || res != "ok" {
		return fmt.Errorf("备份里的数据库已损坏，没有恢复（%v %s）", err, res)
	}
	return nil
}

// copyRecords empties every table and fills it from the snapshot, on one
// connection (ATTACH is per connection) and in one transaction.
func copyRecords(st *store.Store, snap string) (err error) {
	ctx := context.Background()
	c, err := st.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err := c.ExecContext(ctx, "ATTACH DATABASE ? AS r", snap); err != nil {
		return err
	}
	defer c.ExecContext(ctx, "DETACH DATABASE r")
	tx, err := c.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	// rows go in in any order; the foreign keys are checked at commit
	if _, err = tx.Exec("PRAGMA defer_foreign_keys = ON"); err != nil {
		return err
	}
	tables, err := names(tx, "SELECT name FROM main.sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return err
	}
	theirs, err := names(tx, "SELECT name FROM r.sqlite_master WHERE type = 'table'")
	if err != nil {
		return err
	}
	has := map[string]bool{}
	for _, t := range theirs {
		has[t] = true
	}
	// every delete before any insert: a cascade from a later delete would
	// otherwise take restored rows with it
	for _, t := range tables {
		if _, err = tx.Exec(fmt.Sprintf("DELETE FROM main.%q", t)); err != nil {
			return err
		}
	}
	for _, t := range tables {
		if !has[t] {
			continue
		}
		cols, err := shared(tx, t)
		if err != nil {
			return err
		}
		if len(cols) == 0 {
			continue
		}
		list := strings.Join(cols, ", ")
		if _, err = tx.Exec(fmt.Sprintf("INSERT INTO main.%q (%s) SELECT %s FROM r.%q", t, list, list, t)); err != nil {
			return fmt.Errorf("恢复 %s 失败：%w", t, err)
		}
	}
	// the AUTOINCREMENT counters, so new records don't reuse restored ids
	if has["sqlite_sequence"] {
		if _, err = tx.Exec("DELETE FROM main.sqlite_sequence"); err == nil {
			_, err = tx.Exec("INSERT INTO main.sqlite_sequence (name, seq) SELECT name, seq FROM r.sqlite_sequence")
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func names(tx *sql.Tx, q string) ([]string, error) {
	rows, err := tx.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// shared lists the columns table t has in both databases, quoted.
func shared(tx *sql.Tx, t string) ([]string, error) {
	cols := func(schema string) (map[string]bool, []string, error) {
		rows, err := tx.Query(fmt.Sprintf("PRAGMA %s.table_info(%q)", schema, t))
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		set, order := map[string]bool{}, []string{}
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				return nil, nil, err
			}
			set[name] = true
			order = append(order, name)
		}
		return set, order, rows.Err()
	}
	_, mine, err := cols("main")
	if err != nil {
		return nil, err
	}
	theirs, _, err := cols("r")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range mine {
		if theirs[c] {
			out = append(out, fmt.Sprintf("%q", c))
		}
	}
	return out, nil
}
