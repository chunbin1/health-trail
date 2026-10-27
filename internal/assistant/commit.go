package assistant

import (
	"strconv"
	"strings"

	"github.com/lee/health-trail/internal/mcpserver"
	"github.com/lee/health-trail/internal/msg"
	"github.com/lee/health-trail/internal/store"
)

// Saved is one row that was written.
type Saved struct {
	Index int    `json:"index"`
	Kind  string `json:"kind"`
	Type  string `json:"type,omitempty"`
	ID    int64  `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

// Result is the outcome of a confirmed save. When Error is set after some
// rows were saved, those rows are in Saved and nothing further was written.
type Result struct {
	OK          bool    `json:"ok"`
	Error       string  `json:"error,omitempty"`
	Saved       []Saved `json:"saved"`
	FailedIndex *int    `json:"failed_index,omitempty"`
}

// Commit writes items. Ordinary records share one transaction. Changes,
// check-ins and lab reports run after it; if one of those fails, earlier
// rows stay saved. Errors are in lang, the UI's language.
func Commit(st *store.Store, items []Item, lang string) Result {
	out := Result{Saved: []Saved{}}
	if len(items) == 0 {
		out.Error = msg.Pick(lang, "没有要保存的记录", "Nothing to save")
		return out
	}
	if len(items) > maxItems {
		out.Error = msg.Sprintf(lang, msg.Pick(lang, "一次最多保存 %d 条", "At most %d can be saved at once"), maxItems)
		return out
	}
	for i := range items {
		items[i] = normalize(items[i])
	}
	for i, it := range items {
		switch it.Kind {
		case "record", "checkin", "lab_report", "update":
		default:
			return fail(out, i, msg.Sprintf(lang, msg.Pick(lang, "第 %d 条：不能记这种类型", "Item %d: this kind of record can't be saved"), i+1))
		}
	}
	var rows []store.CaptureRow
	for i, it := range items {
		if it.Kind != "record" {
			continue
		}
		if !recordTypeOK(it.Type) {
			out.Error = msg.Sprintf(lang, msg.Pick(lang, "第 %d 条：不能记这种类型", "Item %d: this kind of record can't be saved"), i+1)
			n := i
			out.FailedIndex = &n
			return out
		}
		rows = append(rows, store.CaptureRow{
			Entity: it.Type, Fields: cloneMap(it.Fields),
			ConditionName: it.ConditionName, MedicationName: it.MedicationName,
			Index: i + 1,
		})
	}
	if len(rows) > 0 {
		saved, err := st.CreateInOrder(rows)
		if err != nil {
			out.Error = msg.In(err, lang)
			return out
		}
		j := 0
		for i, it := range items {
			if it.Kind != "record" {
				continue
			}
			out.Saved = append(out.Saved, Saved{
				Index: i, Kind: "record", Type: it.Type,
				ID: idOf(saved[j]), Title: titleOf(it.Type, saved[j]),
			})
			j++
		}
	}
	for i, it := range items {
		if it.Kind != "update" {
			continue
		}
		row, err := st.Update(it.Type, it.ID, store.Record(it.Fields))
		if err != nil {
			return fail(out, i, msg.Sprintf(lang, msg.Pick(lang, "第 %d 条：%v", "Item %d: %v"), i+1, err))
		}
		out.Saved = append(out.Saved, Saved{
			Index: i, Kind: "update", Type: it.Type,
			ID: idOf(row), Title: titleOf(it.Type, row),
		})
	}
	for i, it := range items {
		if it.Kind != "checkin" {
			continue
		}
		h, err := st.FindHabit(it.Habit)
		if err != nil {
			return fail(out, i, msg.Sprintf(lang, msg.Pick(lang, "没有叫「%s」的打卡项", "There's no check-in item called %q"), strings.TrimSpace(it.Habit)))
		}
		date := it.Date
		if date == "" {
			date = "today"
		}
		val, err := floatPtr(it.Value)
		if err != nil {
			return fail(out, i, msg.Pick(lang, "数量需要数字", "The amount must be a number"))
		}
		row, err := st.Checkin(idOf(h), date, val, it.Notes)
		if err != nil {
			return fail(out, i, msg.In(err, lang))
		}
		out.Saved = append(out.Saved, Saved{
			Index: i, Kind: "checkin", Type: "checkins",
			ID: idOf(row), Title: text(h["name"]),
		})
	}
	for i, it := range items {
		if it.Kind != "lab_report" {
			continue
		}
		labs := make([]store.Record, len(it.Results))
		date, _ := it.Report["report_date"].(string)
		for k, row := range it.Results {
			labs[k] = labRecord(row, it.Panel, date)
		}
		res, err := mcpserver.ImportLabs(st, "", it.Report, it.Panel, labs)
		if err != nil {
			return fail(out, i, msg.In(err, lang))
		}
		rep, _ := res["report"].(store.Record)
		out.Saved = append(out.Saved, Saved{
			Index: i, Kind: "lab_report", Type: "reports",
			ID: idOf(rep), Title: text(rep["title"]),
		})
	}
	out.OK = true
	return out
}

func fail(out Result, index int, text string) Result {
	out.OK = false
	out.Error = text
	out.FailedIndex = &index
	return out
}

func titleOf(entity string, r store.Record) string {
	e := store.EntityByName(entity)
	if e == nil {
		return ""
	}
	return text(r[e.TitleKey])
}

func floatPtr(v any) (*float64, error) {
	if v == nil {
		return nil, nil
	}
	switch x := v.(type) {
	case float64:
		return &x, nil
	case int:
		f := float64(x)
		return &f, nil
	case int64:
		f := float64(x)
		return &f, nil
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil, nil
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, err
		}
		return &n, nil
	case jsonNumber:
		n, err := x.Float64()
		if err != nil {
			return nil, err
		}
		return &n, nil
	default:
		return nil, msg.New("数量需要数字", "The amount must be a number")
	}
}

// jsonNumber matches encoding/json.Number without importing it twice in the signature.
type jsonNumber interface {
	Float64() (float64, error)
}
