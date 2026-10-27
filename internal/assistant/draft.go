package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lee/health-trail/internal/store"
)

const (
	maxRunes    = 4000
	maxItems    = 30
	maxResponse = 100 << 10
)

// Item is one proposed record. Problem is set when it would not save as it is.
type Item struct {
	Kind           string         `json:"kind"`
	Type           string         `json:"type,omitempty"`
	Fields         map[string]any `json:"fields,omitempty"`
	ConditionName  string         `json:"condition_name,omitempty"`
	MedicationName string         `json:"medication_name,omitempty"`
	Habit          string         `json:"habit,omitempty"`
	Date           string         `json:"date,omitempty"`
	Value          any            `json:"value,omitempty"`
	Notes          string         `json:"notes,omitempty"`
	Report         map[string]any `json:"report,omitempty"`
	Panel          string         `json:"panel,omitempty"`
	Results        []LabResult    `json:"results,omitempty"`
	Summary        string         `json:"summary,omitempty"`
	Problem        string         `json:"problem,omitempty"`
}

// LabResult is one line of a dictated lab report.
type LabResult struct {
	ItemName string `json:"item_name,omitempty"`
	Value    any    `json:"value,omitempty"`
	Unit     string `json:"unit,omitempty"`
	RefRange string `json:"ref_range,omitempty"`
	Flag     string `json:"flag,omitempty"`
	Panel    string `json:"panel,omitempty"`
	TestDate string `json:"test_date,omitempty"`
	Notes    string `json:"notes,omitempty"`
}

// Draft is what the model proposed, checked against the schema but not saved.
type Draft struct {
	Items   []Item `json:"items"`
	Unclear string `json:"unclear,omitempty"`
}

// Propose asks the saved interface to turn text into items.
func Propose(ctx context.Context, st *store.Store, cfg Config, text string, now time.Time) (Draft, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Draft{}, errors.New("先说或写一点内容")
	}
	if utf8.RuneCountInString(text) > maxRunes {
		return Draft{}, fmt.Errorf("一次最多整理 %d 个字", maxRunes)
	}
	if !cfg.Configured() {
		return Draft{}, errors.New("先填写接口地址、密钥和模型")
	}
	content, err := complete(ctx, cfg, Instructions(st, now), "请整理下面这段话：\n---\n"+text+"\n---")
	if err != nil {
		return Draft{}, err
	}
	d, err := parseDraft(content)
	if err != nil {
		return Draft{}, err
	}
	if len(d.Items) > maxItems {
		return Draft{}, fmt.Errorf("一次最多整理 %d 条，请分开说", maxItems)
	}
	for i := range d.Items {
		d.Items[i] = normalize(d.Items[i])
		if p := problem(st, d.Items[i], now); p != "" {
			d.Items[i].Problem = p
		}
	}
	if len(d.Items) == 0 && d.Unclear == "" {
		d.Unclear = "没有从这段话里整理出可以保存的记录"
	}
	return d, nil
}

func problem(st *store.Store, it Item, now time.Time) string {
	switch it.Kind {
	case "record":
		if !recordTypeOK(it.Type) {
			return "不能记这种类型"
		}
		fields := cloneMap(it.Fields)
		delete(fields, "condition_name")
		delete(fields, "medication_name")
		if err := checkNew(st, it.Type, fields); err != nil {
			return err.Error()
		}
	case "checkin":
		if strings.TrimSpace(it.Habit) == "" {
			return "要写打卡项的名称"
		}
		if _, err := st.FindHabit(it.Habit); err != nil {
			return fmt.Sprintf("没有叫「%s」的打卡项", strings.TrimSpace(it.Habit))
		}
		if it.Date != "" && it.Date != "today" && it.Date != "今天" && it.Date != now.Format("2006-01-02") {
			return "打卡只能记在今天"
		}
	case "lab_report":
		if len(it.Results) == 0 {
			return "没有要保存的指标"
		}
		if err := checkNew(st, "reports", it.Report); err != nil {
			return err.Error()
		}
		date, _ := it.Report["report_date"].(string)
		for _, row := range it.Results {
			rec := labRecord(row, it.Panel, date)
			if rec["test_date"] == nil || rec["test_date"] == "" {
				rec["test_date"] = date
			}
			if err := checkNew(st, "lab_results", rec); err != nil {
				return err.Error()
			}
		}
	default:
		return "无法识别这一条"
	}
	return ""
}

func checkNew(st *store.Store, entity string, in map[string]any) error {
	return st.CheckNew(entity, in)
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
