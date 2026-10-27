package assistant

import (
	"context"
	"encoding/base64"
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
// Usage is what the model service reported for this reply.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CachedTokens     int `json:"cached_tokens,omitempty"`
}

type Draft struct {
	Items   []Item `json:"items"`
	Unclear string `json:"unclear,omitempty"`
	Reply   string `json:"reply,omitempty"`
	Raw     string `json:"raw,omitempty"`
	Usage   Usage  `json:"usage"`
}

// Turn is one side of the capture chat. Images are raw base64, jpeg/png/gif/webp only.
type Turn struct {
	Role   string  `json:"role"`
	Text   string  `json:"text"`
	Images []Image `json:"images,omitempty"`
}

// Image is one picture the person attached.
type Image struct {
	MIME string `json:"mime"`
	Data string `json:"data"`
}

// Propose asks the saved interface to turn the latest turn into items.
// turns is the conversation so far, ending with the user.
func Propose(ctx context.Context, st *store.Store, cfg Config, turns []Turn, now time.Time) (Draft, error) {
	turns, err := cleanTurns(turns)
	if err != nil {
		return Draft{}, err
	}
	if !cfg.Configured() {
		return Draft{}, errors.New("先填写接口地址、密钥和模型")
	}
	system := Instructions(st, now)
	content, usage, err := complete(ctx, cfg, system, turns)
	logExchange(st.Dir, cfg, system, turns, content, usage, err)
	if err != nil {
		return Draft{}, err
	}
	d, err := parseDraft(content)
	if err != nil {
		if strings.TrimSpace(content) == "" {
			return Draft{}, err
		}
		// A plain answer, not a record, is still an answer.
		return Draft{Reply: strings.TrimSpace(content), Items: []Item{}, Raw: content, Usage: usage}, nil
	}
	d.Raw, d.Usage = content, usage
	if len(d.Items) > maxItems {
		return Draft{}, fmt.Errorf("一次最多整理 %d 条，请分开说", maxItems)
	}
	for i := range d.Items {
		d.Items[i] = normalize(d.Items[i])
	}
	d.Items = dropConditions(st, turns, d.Items)
	d.Items = dropDuplicateMeds(st, d.Items)
	d.Items = fillConditionDates(d.Items, now)
	d.Items = fillMedicationStatus(d.Items)
	for i := range d.Items {
		if p := problem(st, d.Items[i], now); p != "" {
			d.Items[i].Problem = p
		}
	}
	if len(d.Items) == 0 && d.Unclear == "" && d.Reply == "" {
		d.Unclear = "没有从这段话里整理出可以保存的记录"
	}
	return d, nil
}

// dropConditions keeps a diagnosis off a medicine question, and never writes
// a health issue whose name is already on file.
func dropConditions(st *store.Store, turns []Turn, items []Item) []Item {
	ask := ""
	for _, t := range turns {
		if t.Role == "user" || t.Role == "" {
			ask = t.Text
		}
	}
	skipNew := medicineAsk(ask) && !diagnosisAsk(ask) && !recordAsk(ask)
	have := map[string]bool{}
	if rows, _, err := st.List("conditions", store.ListQuery{Limit: 200}); err == nil {
		for _, r := range rows {
			have[sameName(text(r["name"]))] = true
		}
	}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if it.Kind == "record" && it.Type == "conditions" {
			if skipNew {
				continue
			}
			name := sameName(text(it.Fields["name"]))
			if name != "" && have[name] {
				continue
			}
			if name != "" {
				have[name] = true
			}
		}
		out = append(out, it)
	}
	return out
}

// fillConditionDates gives a health issue a start date. The model is asked
// to use the date of the lab or symptom it relied on; if it leaves the
// date out, the day of this save is used so the list is not blank.
func fillConditionDates(items []Item, now time.Time) []Item {
	day := now.Format("2006-01-02")
	for i, it := range items {
		if it.Kind != "record" || it.Type != "conditions" {
			continue
		}
		if it.Fields == nil {
			it.Fields = map[string]any{}
		}
		if strings.TrimSpace(text(it.Fields["onset_date"])) == "" {
			it.Fields["onset_date"] = day
		}
		items[i] = it
	}
	return items
}

// dropDuplicateMeds drops a medicine whose name is already on file, and a
// second copy of the same name in this reply.
func dropDuplicateMeds(st *store.Store, items []Item) []Item {
	have := map[string]bool{}
	if rows, _, err := st.List("medications", store.ListQuery{Limit: 200}); err == nil {
		for _, r := range rows {
			have[sameName(text(r["name"]))] = true
		}
	}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if it.Kind == "record" && it.Type == "medications" {
			name := sameName(text(it.Fields["name"]))
			if name != "" && have[name] {
				continue
			}
			if name != "" {
				have[name] = true
			}
		}
		out = append(out, it)
	}
	return out
}

// fillMedicationStatus marks a suggested medicine as not yet started when the
// model leaves the status out. A medicine the person says they take should
// already say 在用.
func fillMedicationStatus(items []Item) []Item {
	for i, it := range items {
		if it.Kind != "record" || it.Type != "medications" {
			continue
		}
		if it.Fields == nil {
			it.Fields = map[string]any{}
		}
		if strings.TrimSpace(text(it.Fields["status"])) == "" {
			it.Fields["status"] = "计划中"
		}
		items[i] = it
	}
	return items
}

func medicineAsk(s string) bool {
	for _, w := range []string{"药", "怎么治", "怎么吃"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func diagnosisAsk(s string) bool {
	for _, w := range []string{"什么问题", "什么病", "是不是", "诊断", "可能是"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func recordAsk(s string) bool {
	for _, w := range []string{"记下", "记一下", "记录", "帮我记", "保存"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func sameName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), ""))
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

const (
	maxImages     = 4
	maxImageBytes = 2 << 20
	maxTurns      = 12
)

func cleanTurns(in []Turn) ([]Turn, error) {
	if len(in) == 0 {
		return nil, errors.New("先说或写一点内容")
	}
	if len(in) > maxTurns {
		in = in[len(in)-maxTurns:]
	}
	out := make([]Turn, 0, len(in))
	images := 0
	for _, t := range in {
		role := strings.TrimSpace(t.Role)
		if role != "user" && role != "assistant" {
			return nil, errors.New("对话里有无法识别的一方")
		}
		text := strings.TrimSpace(t.Text)
		if role == "assistant" {
			if text == "" {
				continue
			}
			out = append(out, Turn{Role: role, Text: text})
			continue
		}
		pics, err := cleanImages(t.Images)
		if err != nil {
			return nil, err
		}
		images += len(pics)
		if images > maxImages*2 {
			return nil, fmt.Errorf("一次对话最多附 %d 张图片", maxImages*2)
		}
		if text == "" && len(pics) == 0 {
			continue
		}
		if utf8.RuneCountInString(text) > maxRunes {
			return nil, fmt.Errorf("一次最多整理 %d 个字", maxRunes)
		}
		out = append(out, Turn{Role: "user", Text: text, Images: pics})
	}
	if len(out) == 0 || out[len(out)-1].Role != "user" {
		return nil, errors.New("先说或写一点内容")
	}
	return out, nil
}

func cleanImages(in []Image) ([]Image, error) {
	if len(in) > maxImages {
		return nil, fmt.Errorf("一次最多 %d 张图片", maxImages)
	}
	out := make([]Image, 0, len(in))
	for _, im := range in {
		mime := strings.ToLower(strings.TrimSpace(im.MIME))
		data := strings.TrimSpace(im.Data)
		if i := strings.Index(data, "base64,"); i >= 0 {
			data = data[i+len("base64,"):]
		}
		data = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == ' ' {
				return -1
			}
			return r
		}, data)
		switch mime {
		case "image/jpeg", "image/png", "image/gif", "image/webp":
		default:
			return nil, errors.New("只能发送图片（JPG、PNG、GIF、WebP）")
		}
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil || len(raw) == 0 {
			return nil, errors.New("这张图片读不出来")
		}
		if len(raw) > maxImageBytes {
			return nil, errors.New("这张图片太大")
		}
		out = append(out, Image{MIME: mime, Data: base64.StdEncoding.EncodeToString(raw)})
	}
	return out, nil
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
