package store

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MaxAttachment is the largest file an attachment may be.
const MaxAttachment = 50 << 20

// AddAttachment stores a file (a report's PDF, a photo of a prescription)
// under the data directory and links it to a record. The stored name is
// random, so a file's original name never becomes a path.
func (s *Store) AddAttachment(ownerType string, ownerID int64, filename, mime string, r io.Reader) (Record, error) {
	e, err := entity(ownerType)
	if err != nil {
		return nil, err
	}
	if err := s.mustExist(e.Name, ownerID); err != nil {
		return nil, ErrNotFound
	}
	b := make([]byte, 16)
	rand.Read(b)
	ext := strings.ToLower(filepath.Ext(filename))
	if len(ext) > 10 || strings.ContainsAny(ext, `/\`) {
		ext = ""
	}
	stored := hex.EncodeToString(b) + ext
	path := filepath.Join(s.FileDir, stored)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(f, io.LimitReader(r, MaxAttachment+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > MaxAttachment {
		err = invalid("文件超过 %d MB", "The file is over %d MB", MaxAttachment>>20)
	}
	if err != nil {
		os.Remove(path)
		return nil, err
	}
	res, err := s.DB.Exec(`INSERT INTO attachments (owner_type, owner_id, filename, mime, size, stored_name, created_at) VALUES (?,?,?,?,?,?,?)`,
		e.Name, ownerID, filepath.Base(filename), mime, n, stored, now())
	if err != nil {
		os.Remove(path)
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.Attachment(id)
}

func (s *Store) Attachment(id int64) (Record, error) {
	rows, err := s.query(`SELECT * FROM attachments WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return rows[0], nil
}

func (s *Store) Attachments(ownerType string, ownerID int64) ([]Record, error) {
	return s.query(`SELECT * FROM attachments WHERE owner_type = ? AND owner_id = ? ORDER BY id`, ownerType, ownerID)
}

// AttachmentPath is where an attachment's bytes are on disk.
func (s *Store) AttachmentPath(a Record) string {
	return filepath.Join(s.FileDir, filepath.Base(a["stored_name"].(string)))
}

func (s *Store) DeleteAttachment(id int64) error {
	a, err := s.Attachment(id)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM attachments WHERE id = ?`, id); err != nil {
		return err
	}
	os.Remove(s.AttachmentPath(a))
	return nil
}
