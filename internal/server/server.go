// Package server is the HTTP side of the app: the REST API the UI uses,
// the MCP endpoint, and the UI itself.
package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lee/health-trail/internal/store"
)

type Server struct {
	Store *store.Store
	UI    fs.FS        // the built frontend (index.html at its root)
	MCP   http.Handler // served at /mcp
	Info  func(r *http.Request) map[string]any
}

// Handler serves everything with no access checks. Only the desktop
// window's in-process asset server uses it directly; anything listening on
// a socket goes through Guard.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/schema", s.schema)
	mux.HandleFunc("GET /api/info", s.info)
	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("GET /api/timeline", s.timeline)
	mux.HandleFunc("GET /api/search", s.search)
	mux.HandleFunc("GET /api/labs/items", s.labItems)
	mux.HandleFunc("GET /api/labs/trend", s.labTrend)
	mux.HandleFunc("GET /api/metrics/series", s.metricSeries)
	mux.HandleFunc("GET /api/checkins/day", s.checkinDay)
	mux.HandleFunc("GET /api/checkins/stats", s.checkinStats)
	mux.HandleFunc("POST /api/checkins", s.checkin)
	mux.HandleFunc("DELETE /api/checkins", s.uncheck)
	mux.HandleFunc("GET /api/r/{entity}", s.list)
	mux.HandleFunc("POST /api/r/{entity}", s.create)
	mux.HandleFunc("GET /api/r/{entity}/{id}", s.get)
	mux.HandleFunc("PUT /api/r/{entity}/{id}", s.update)
	mux.HandleFunc("DELETE /api/r/{entity}/{id}", s.delete)
	mux.HandleFunc("POST /api/attachments", s.upload)
	mux.HandleFunc("GET /api/attachments/{id}", s.download)
	mux.HandleFunc("DELETE /api/attachments/{id}", s.deleteAttachment)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "没有这个接口："+r.Method+" "+r.URL.Path)
	})
	if s.MCP != nil {
		mux.Handle("/mcp", s.MCP)
	}
	if s.UI != nil {
		mux.Handle("/", spa(s.UI))
	}
	return mux
}

// spa serves the built UI, answering unknown paths with index.html so a
// reload on a client-side route still loads the app.
func spa(ui fs.FS) http.Handler {
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(ui, p); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func fail(w http.ResponseWriter, err error) {
	var ve *store.ValidationError
	switch {
	case errors.As(err, &ve):
		writeErr(w, http.StatusBadRequest, ve.Msg)
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	default:
		log.Printf("api: %v", err)
		writeErr(w, http.StatusInternalServerError, "内部错误："+err.Error())
	}
}

// decode reads a JSON body, keeping numbers exact (json.Number) so the
// store converts them by the field's type.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 5<<20))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return &store.ValidationError{Msg: "请求不是有效的 JSON：" + err.Error()}
	}
	return nil
}

func idParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, &store.ValidationError{Msg: "无效的 id"}
	}
	return id, nil
}

func (s *Server) schema(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"entities":       store.Entities,
		"metric_types":   store.MetricTypes,
		"metric_derived": store.DerivedMetrics,
		"metric_units":   store.MetricUnits,
		"en":             map[string]any{"text": store.English, "nouns": store.EnglishNouns},
	})
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"data_dir": s.Store.Dir}
	if s.Info != nil {
		for k, v := range s.Info(r) {
			out[k] = v
		}
	}
	writeJSON(w, out)
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	o, err := s.Store.Overview()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, o)
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := s.Store.Timeline(n)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	hits, err := s.Store.Search(r.URL.Query().Get("q"), 20)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, hits)
}

func (s *Server) labItems(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.LabItems()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, items)
}

func (s *Server) labTrend(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.LabTrend(r.URL.Query().Get("name"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) metricSeries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rows, err := s.Store.MetricSeries(q.Get("type"), q.Get("from"), q.Get("to"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) checkinDay(w http.ResponseWriter, r *http.Request) {
	d := r.URL.Query().Get("date")
	if d == "" {
		d = "today"
	}
	date, err := store.NormalizeDate(d)
	if err != nil {
		fail(w, &store.ValidationError{Msg: err.Error()})
		return
	}
	days, err := s.Store.HabitDays(date)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"date": date, "habits": days})
}

func (s *Server) checkinStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hid, _ := strconv.ParseInt(q.Get("habit_id"), 10, 64)
	st, err := s.Store.CheckinStats(q.Get("from"), q.Get("to"), hid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, st)
}

func (s *Server) checkin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		HabitID int64    `json:"habit_id"`
		Date    string   `json:"date"`
		Value   *float64 `json:"value"`
		Notes   string   `json:"notes"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	if in.Date == "" {
		in.Date = "today"
	}
	c, err := s.Store.Checkin(in.HabitID, in.Date, in.Value, in.Notes)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, c)
}

func (s *Server) uncheck(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hid, err := strconv.ParseInt(q.Get("habit_id"), 10, 64)
	if err != nil {
		fail(w, &store.ValidationError{Msg: "无效的 habit_id"})
		return
	}
	ok, err := s.Store.Uncheck(hid, q.Get("date"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"removed": ok})
}

var listParams = map[string]bool{"q": true, "from": true, "to": true, "limit": true, "offset": true, "asc": true}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lq := store.ListQuery{Q: q.Get("q"), From: q.Get("from"), To: q.Get("to"), Filters: map[string]any{}}
	lq.Limit, _ = strconv.Atoi(q.Get("limit"))
	lq.Offset, _ = strconv.Atoi(q.Get("offset"))
	lq.Asc = q.Get("asc") == "1" || q.Get("asc") == "true"
	for k, v := range q {
		if !listParams[k] {
			lq.Filters[k] = v[0]
		}
	}
	rows, total, err := s.Store.List(r.PathValue("entity"), lq)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"rows": rows, "total": total})
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var in store.Record
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	rec, err := s.Store.Create(r.PathValue("entity"), in)
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, rec)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, err)
		return
	}
	rec, err := s.Store.Related(r.PathValue("entity"), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, rec)
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in store.Record
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	rec, err := s.Store.Update(r.PathValue("entity"), id, in)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, rec)
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.Delete(r.PathValue("entity"), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	// The Mac window loads through a custom URL scheme. A multipart file
	// arrives on a stream that is not ready yet, and the scheme handler
	// treats that as an empty body ("multipart: NextPart: EOF"). A JSON
	// body, the same way the chat already sends a picture, arrives whole.
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		s.uploadJSON(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, store.MaxAttachment+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		fail(w, &store.ValidationError{Msg: "上传失败：" + err.Error()})
		return
	}
	ownerID, err := strconv.ParseInt(r.FormValue("owner_id"), 10, 64)
	if err != nil {
		fail(w, &store.ValidationError{Msg: "无效的 owner_id"})
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		fail(w, &store.ValidationError{Msg: "缺少文件"})
		return
	}
	defer f.Close()
	ct := hdr.Header.Get("Content-Type")
	if t := mime.TypeByExtension(filepath.Ext(hdr.Filename)); t != "" {
		ct = t
	}
	a, err := s.Store.AddAttachment(r.FormValue("owner_type"), ownerID, hdr.Filename, ct, f)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, a)
}

func (s *Server) uploadJSON(w http.ResponseWriter, r *http.Request) {
	// base64 is about 4/3 of the file, plus the surrounding JSON
	r.Body = http.MaxBytesReader(w, r.Body, store.MaxAttachment*4/3+1<<20)
	var in struct {
		OwnerType string `json:"owner_type"`
		OwnerID   int64  `json:"owner_id"`
		Filename  string `json:"filename"`
		Mime      string `json:"mime"`
		Data      string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, &store.ValidationError{Msg: "上传失败：" + err.Error()})
		return
	}
	data := in.Data
	if i := strings.Index(data, ","); i >= 0 && strings.HasPrefix(data, "data:") {
		data = data[i+1:]
	}
	data = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, data)
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(raw) == 0 {
		fail(w, &store.ValidationError{Msg: "图片数据读不出来"})
		return
	}
	if len(raw) > store.MaxAttachment {
		fail(w, invalidSize())
		return
	}
	ct := in.Mime
	if t := mime.TypeByExtension(filepath.Ext(in.Filename)); t != "" {
		ct = t
	}
	a, err := s.Store.AddAttachment(in.OwnerType, in.OwnerID, in.Filename, ct, bytes.NewReader(raw))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, a)
}

func invalidSize() error {
	return &store.ValidationError{Msg: "文件超过 50 MB"}
}

// inlineTypes may be shown in the window; anything else is downloaded, so
// an uploaded HTML or SVG file never runs as a page of this origin.
var inlineTypes = map[string]bool{
	"application/pdf": true, "image/png": true, "image/jpeg": true, "image/gif": true,
	"image/webp": true, "image/heic": true, "text/plain; charset=utf-8": true,
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, err)
		return
	}
	a, err := s.Store.Attachment(id)
	if err != nil {
		fail(w, err)
		return
	}
	ct, _ := a["mime"].(string)
	disp := "attachment"
	if inlineTypes[ct] {
		disp = "inline"
	} else {
		ct = "application/octet-stream"
	}
	name, _ := a["filename"].(string)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	http.ServeFile(w, r, s.Store.AttachmentPath(a))
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.DeleteAttachment(id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}
