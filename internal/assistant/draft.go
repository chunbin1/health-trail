package assistant

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lee/health-trail/internal/msg"
	"github.com/lee/health-trail/internal/store"
)

const (
	maxRunes    = 4000
	maxItems    = 30
	maxResponse = 100 << 10
)

// Item is one proposed record, or a change to one (kind "update"). Problem is
// set when it would not save as it is.
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
	// kind "update" changes record ID of Type: Fields are the new values,
	// Before what they were when the card was made
	ID     int64          `json:"id,omitempty"`
	Before map[string]any `json:"before,omitempty"`
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

// Usage is what the model service reported, summed over the rounds of one reply.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CachedTokens     int `json:"cached_tokens,omitempty"`
}

func (u *Usage) add(v Usage) {
	u.PromptTokens += v.PromptTokens
	u.CompletionTokens += v.CompletionTokens
	u.TotalTokens += v.TotalTokens
	u.CachedTokens += v.CachedTokens
}

// Draft is one reply: the answer, the records the model put together
// (checked against the schema, not saved), and the tools it called.
type Draft struct {
	Items   []Item `json:"items"`
	Unclear string `json:"unclear,omitempty"`
	Reply   string `json:"reply,omitempty"`
	Steps   []Step `json:"steps"`
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

// a reply may take this many requests: reading, then answering
const maxRounds = 8

// Propose answers the latest turn. The model reads the archive with the MCP
// tools as many rounds as it needs; what it would write comes back as Items.
// onStep, if set, hears each tool call as it happens.
func Propose(ctx context.Context, st *store.Store, cfg Config, turns []Turn, now time.Time, onStep func(Step)) (Draft, error) {
	turns, err := cleanTurns(turns)
	if err != nil {
		return Draft{}, err
	}
	if !cfg.Configured() {
		return Draft{}, msg.New("先填写接口地址、密钥和模型", "Enter the API address, key and model first")
	}
	tb, err := openTools(ctx, st)
	if err != nil {
		return Draft{}, err
	}
	defer tb.close()
	system := Instructions(st, tb.instructions, now)
	msgs := []any{map[string]any{"role": "system", "content": system}}
	x := newExchange(system, tb.defs, turns)
	for _, t := range turns {
		msgs = append(msgs, map[string]any{"role": t.Role, "content": contentOf(t)})
	}
	d := Draft{Items: []Item{}, Steps: []Step{}}
	w := newDrafting(st, turns, now)
	// whether this reply has looked anything up yet
	read := false
	// whether the model was told it claimed something with no card
	nudged := false
	// the failed calls of earlier rounds: the same one failing again ends
	// the reply instead of spending the rest of the rounds on it
	failedBefore := map[string]bool{}
	// tools that failed and have not worked since, like a check-in on a
	// habit that wasn't there before its card was put together
	unresolved := map[string]bool{}
	// whether the model was asked to check in on a habit it just added
	askedCheckin := false
	stopped := ""
	for round := 0; ; round++ {
		last := round == maxRounds-1
		choice := "auto"
		if last {
			choice = "none"
		}
		m, u, err := chat(ctx, cfg, msgs, tb.defs, choice)
		d.Usage.add(u)
		x.request(u, m.Content)
		if err != nil {
			logExchange(st.Dir, cfg, x, d, err)
			return Draft{}, err
		}
		if len(m.Calls) == 0 || last {
			d.Reply = strings.TrimSpace(m.Content)
			if len(m.Calls) > 0 {
				// asked to answer, it called a tool anyway: what it wrote
				// leads into a call that never runs
				x.note("最后一次请求仍然在调用工具：停下")
				stopped = msg.Pick(cfg.Lang, stopRounds, stopRoundsEn)
				d.Reply = ""
			}
			if !last && !nudged && len(w.items) == 0 && claimsDone(d.Reply) {
				nudged = true
				x.note("回复说已经做好了，却没有卡片：提醒它再来一次")
				msgs = append(msgs, m.Raw, map[string]any{"role": "user", "content": claimNudge})
				continue
			}
			break
		}
		msgs = append(msgs, m.Raw)
		// a round that only put cards together, all of them fine, is the
		// end of the reply: asking the model again would resend everything
		// for one sentence. Unless something was looked up and the answer
		// isn't written yet, or a call that failed earlier hasn't worked
		// since: the model may be fixing what stopped it.
		onlyCards := true
		for _, c := range m.Calls {
			if !draftTools[c.Name] {
				onlyCards = false
			}
			if tb.read[c.Name] {
				read = true
			}
		}
		failedAny := false
		repeated := ""
		failedNow := map[string]bool{}
		for _, c := range m.Calls {
			out, failed := "", false
			name, args, err := callOf(c.Name, c.Args)
			c.Name = name
			// the write tools first: they never reach the store from here,
			// whatever their annotations say
			switch {
			case err != nil:
				out, failed = err.Error(), true
			case c.Name == "update_record":
				out, failed = w.update(args)
			case draftTools[c.Name]:
				out, failed = w.draft(c.Name, args)
			case tb.read[c.Name]:
				out, failed = tb.run(ctx, c.Name, args)
			default:
				out, failed = "这里没有 "+c.Name+" 这个工具", true
			}
			step := stepOf(c.Name, args)
			step.Error = failed
			failedAny = failedAny || failed
			if failed {
				key := c.Name + "\x00" + out
				if failedBefore[key] && repeated == "" {
					repeated = out
					if errors.Is(err, errBadArgs) {
						repeated = msg.Pick(cfg.Lang, "模型发来的工具参数格式不对，App 读不懂", "the model sent tool arguments the app can't read")
					} else if cfg.Lang == "en" {
						// what the tool said is written for the model, in Chinese
						repeated = "the tool returned the same error"
					}
				}
				failedNow[key] = true
				unresolved[c.Name] = true
			} else {
				delete(unresolved, c.Name)
			}
			x.call(c.Name, c.Args, out, failed)
			d.Steps = append(d.Steps, step)
			if onStep != nil {
				onStep(step)
			}
			msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": c.ID, "content": out})
		}
		if repeated != "" {
			x.note("同一个调用接连出错：停下，不再重试")
			stopped = msg.Sprintf(cfg.Lang, msg.Pick(cfg.Lang, stopRepeat, stopRepeatEn), repeated)
			break
		}
		for k := range failedNow {
			failedBefore[k] = true
		}
		// a habit added or restored with no check-in on it: "新建喝水，我今天
		// 喝了3L" often stops at the habit. Its tool result asks for the
		// check-in; the model gets one more round to make it.
		waitCheckin := !askedCheckin && w.habitWithoutCheckin()
		askedCheckin = askedCheckin || waitCheckin
		if onlyCards && !failedAny && len(unresolved) == 0 && !waitCheckin && (!read || strings.TrimSpace(m.Content) != "") {
			d.Reply = strings.TrimSpace(m.Content)
			break
		}
	}
	if w.items != nil {
		d.Items = w.items
	}
	d.Reply = plainReply(d.Reply)
	if stopped != "" {
		// the model's own words here lead into a call that never ran
		d.Reply = strings.TrimSpace(stopped)
	}
	if len(d.Items) == 0 && claimsDone(d.Reply) {
		d.Reply += msg.Pick(cfg.Lang, claimNote, claimNoteEn)
	}
	if len(d.Items) == 0 && d.Reply == "" {
		d.Unclear = msg.Pick(cfg.Lang, "没有从这段话里整理出可以保存的记录", "Nothing to save came out of this")
	}
	logExchange(st.Dir, cfg, x, d, nil)
	return d, nil
}

// What the person is told when the reply is cut short. The cards put
// together before that are kept.
const (
	stopRepeat   = "这一步接连两次出错，我先停下了，没有再试：%s。可以换个说法再试一次；每一步的经过在「设置 → AI助手 → 查看日志」里。"
	stopRepeatEn = "The same step failed twice in a row, so I stopped instead of trying again (%s). Try saying it another way; each step is in Settings → AI assistant → View log."
	stopRounds   = "查了好几轮还没做完，我先停下了。可以把要做的事说得更具体一点再试；每一步的经过在「设置 → AI助手 → 查看日志」里。"
	stopRoundsEn = "This took too many rounds, so I stopped. Try asking for something more specific; each step is in Settings → AI assistant → View log."
)

// drafting collects what the write tools were called with. Nothing is
// written here; a call that would not save is told why, so the model can fix
// it in the next round instead of leaving a broken card.
type drafting struct {
	st    *store.Store
	now   time.Time
	items []Item
	// a question about medicine: a diagnosis stays in the reply
	medicine bool
	conds    map[string]int64
	meds     map[string]int64
}

func newDrafting(st *store.Store, turns []Turn, now time.Time) *drafting {
	ask := ""
	for _, t := range turns {
		if t.Role == "user" {
			ask = t.Text
		}
	}
	return &drafting{
		st: st, now: now,
		medicine: medicineAsk(ask) && !diagnosisAsk(ask) && !recordAsk(ask),
		conds:    namesOf(st, "conditions"),
		meds:     namesOf(st, "medications"),
	}
}

func namesOf(st *store.Store, entity string) map[string]int64 {
	out := map[string]int64{}
	rows, _, err := st.List(entity, store.ListQuery{Limit: 500})
	if err != nil {
		return out
	}
	for _, r := range rows {
		if n := sameName(text(r["name"])); n != "" {
			out[n] = idOf(r)
		}
	}
	return out
}

func (w *drafting) draft(tool string, args map[string]any) (string, bool) {
	if len(w.items) >= maxItems {
		return fmt.Sprintf("一次最多整理 %d 条，剩下的请在回复里告诉用户分开说", maxItems), true
	}
	m := cloneMap(args)
	switch tool {
	case "add_record":
		m["kind"] = "record"
	case "add_lab_report":
		if id := idOf(store.Record{"id": args["report_id"]}); id != 0 {
			return "在 App 里只能新建报告：去掉 report_id，用 report 填报告的日期和标题", true
		}
		m["kind"] = "lab_report"
	case "checkin":
		m["kind"] = "checkin"
	}
	it := itemFromMap(m)
	// checkin's create_if_missing, as over MCP: the new habit is a card
	// of its own, before the check-in on it
	habit := Item{}
	if tool == "checkin" && truthy(args["create_if_missing"]) && it.Habit != "" && !w.draftedHabit(it.Habit) {
		if _, err := w.st.FindHabit(it.Habit); errors.Is(err, store.ErrNotFound) {
			habit = Item{Kind: "record", Type: "habits", Fields: map[string]any{"name": it.Habit}}
			for _, k := range []string{"unit", "icon"} {
				if v := text(args[k]); v != "" {
					habit.Fields[k] = v
				}
			}
			if p := w.check(&habit); p != "" {
				return p, true
			}
			w.items = append(w.items, habit)
		}
	}
	if p := w.check(&it); p != "" {
		if habit.Kind != "" {
			w.items = w.items[:len(w.items)-1]
		}
		return p, true
	}
	w.items = append(w.items, it)
	if habit.Kind != "" {
		return fmt.Sprintf("已整理成第 %d 条（新建打卡项「%s」）和第 %d 条（打卡），交给用户确认后写入。",
			len(w.items)-1, it.Habit, len(w.items)), false
	}
	if it.Kind == "record" && it.Type == "habits" {
		return fmt.Sprintf("已整理成第 %d 条，交给用户确认后写入。用户这次如果也说了今天做了（比如喝了多少），"+
			"现在接着调用 checkin 给「%s」打卡；没说就不用打卡，直接回复。", len(w.items), text(it.Fields["name"])), false
	}
	return fmt.Sprintf("已整理成第 %d 条，交给用户确认后写入，现在还没有 id。"+
		"这次回复里别的记录要关联它，在 fields 里填 condition_name 或 medication_name（名称）。", len(w.items)), false
}

func (w *drafting) draftedHabit(name string) bool {
	for _, d := range w.items {
		if d.Kind == "record" && d.Type == "habits" && sameName(text(d.Fields["name"])) == sameName(name) {
			return true
		}
	}
	return false
}

// habitWithoutCheckin tells whether this reply added or restored a habit and
// has no check-in on it.
func (w *drafting) habitWithoutCheckin() bool {
	for _, d := range w.items {
		name := ""
		switch {
		case d.Kind == "record" && d.Type == "habits":
			name = text(d.Fields["name"])
		case d.Kind == "update" && d.Type == "habits":
			if v, ok := d.Fields["archived"]; ok && !truthy(v) {
				name = d.Summary
			}
		}
		if name == "" {
			continue
		}
		done := false
		for _, c := range w.items {
			if c.Kind == "checkin" && sameName(c.Habit) == sameName(name) {
				done = true
			}
		}
		if !done {
			return true
		}
	}
	return false
}

// update turns update_record into a card that shows the change. It is
// checked the way Update would check it, and only saved by the person.
func (w *drafting) update(args map[string]any) (string, bool) {
	if len(w.items) >= maxItems {
		return fmt.Sprintf("一次最多整理 %d 条，剩下的请在回复里告诉用户分开说", maxItems), true
	}
	typ := strings.TrimSpace(text(args["type"]))
	if a, ok := aliasOf(typ); ok && a != "checkin" && a != "lab_report" {
		typ = a
	}
	if !recordTypeOK(typ) && typ != "lab_results" {
		return "这里只能修改这些类型：" + strings.Join(recordTypes, "、") + "、lab_results", true
	}
	id := idOf(store.Record{"id": args["id"]})
	fields, _ := args["fields"].(map[string]any)
	if len(fields) == 0 {
		return "fields 里写要改的字段和新的值", true
	}
	cur, err := w.st.Get(typ, id)
	if err != nil {
		return fmt.Sprintf("没有 %s #%d 这条记录", typ, id), true
	}
	if err := w.st.CheckUpdate(typ, id, store.Record(fields)); err != nil {
		return err.Error(), true
	}
	before := map[string]any{}
	for k := range fields {
		before[k] = cur[k]
	}
	w.items = append(w.items, Item{Kind: "update", Type: typ, ID: id, Fields: fields, Before: before, Summary: titleOf(typ, cur)})
	return fmt.Sprintf("已整理成第 %d 条修改，等用户点保存后才会改，现在还没改。", len(w.items)), false
}

// check fills what the model may leave out and says what would stop the
// item from saving. It also keeps a diagnosis off a medicine question and
// never drafts a second health issue or medicine with a name on file.
func (w *drafting) check(it *Item) string {
	var have map[string]int64
	name := ""
	if it.Kind == "record" && (it.Type == "conditions" || it.Type == "medications") {
		name = sameName(text(it.Fields["name"]))
		label, ref := "健康问题", "condition_id"
		have = w.conds
		if it.Type == "medications" {
			have, label, ref = w.meds, "药", "medication_id"
		}
		if it.Type == "conditions" && w.medicine {
			return "用户这一轮在问药或怎么治：诊断只写在回复里，不要新建健康问题"
		}
		if id, ok := have[name]; ok && name != "" {
			if id == 0 {
				return fmt.Sprintf("这次已经整理过「%s」了，不要再写一条", text(it.Fields["name"]))
			}
			return fmt.Sprintf("档案里已经有这个%s（%s #%d），不要再写一条；要关联它就填 %s %d", label, it.Type, id, ref, id)
		}
	}
	if it.Kind == "record" && it.Type == "habits" {
		if w.draftedHabit(text(it.Fields["name"])) {
			return fmt.Sprintf("这次已经整理过打卡项「%s」了，不要再写一条", text(it.Fields["name"]))
		}
		// the check-in page shows the icon; without one it is a dot
		if it.Fields == nil {
			it.Fields = map[string]any{}
		}
		if strings.TrimSpace(text(it.Fields["icon"])) == "" {
			it.Fields["icon"] = defaultHabitIcon
		}
	}
	*it = fillConditionDates([]Item{*it}, w.now)[0]
	*it = fillMedicationStatus([]Item{*it})[0]
	if p := problem(w.st, *it, w.now); p != "" && !w.newMetric(*it, p) && !w.newHabit(*it) {
		return p
	}
	if n := sameName(it.ConditionName); n != "" {
		if _, ok := w.conds[n]; !ok {
			return fmt.Sprintf("没有叫「%s」的健康问题，档案里没有，这次也没整理", it.ConditionName)
		}
	}
	if n := sameName(it.MedicationName); n != "" {
		if _, ok := w.meds[n]; !ok {
			return fmt.Sprintf("没有叫「%s」的药，档案里没有，这次也没整理", it.MedicationName)
		}
	}
	if name != "" {
		// 0: put together in this reply, not on file yet
		have[name] = 0
	}
	if it.Kind == "record" && it.Type == "doses" {
		// the card's title: a dose's own title is only the time
		it.Summary = w.medName(*it)
	}
	return ""
}

func (w *drafting) medName(it Item) string {
	if it.MedicationName != "" {
		return it.MedicationName
	}
	r, err := w.st.Get("medications", idOf(store.Record{"id": it.Fields["medication_id"]}))
	if err != nil {
		return ""
	}
	return text(r["name"])
}

// newMetric accepts a reading of a metric kind drafted earlier in this reply.
// Checked on its own, that reading's metric_type looks unknown.
func (w *drafting) newMetric(it Item, problem string) bool {
	if it.Kind != "record" || it.Type != "metrics" || !strings.Contains(problem, "不在可选值中") {
		return false
	}
	for _, d := range w.items {
		if d.Kind == "record" && d.Type == "metric_kinds" && text(d.Fields["name"]) == text(it.Fields["metric_type"]) {
			return true
		}
	}
	return false
}

// defaultHabitIcon is the icon of a habit the model added without one.
const defaultHabitIcon = "✅"

// newHabit accepts a check-in on a habit this reply adds, or takes out of
// the archive. Saving writes records and changes before check-ins.
func (w *drafting) newHabit(it Item) bool {
	if it.Kind != "checkin" {
		return false
	}
	name := sameName(it.Habit)
	if name == "" {
		return false
	}
	var id int64
	if h, err := w.st.FindHabit(it.Habit); err == nil {
		id = idOf(h)
	}
	for _, d := range w.items {
		switch {
		case d.Kind == "record" && d.Type == "habits" && sameName(text(d.Fields["name"])) == name:
			return true
		case d.Kind == "update" && d.Type == "habits" && id != 0 && d.ID == id:
			if v, ok := d.Fields["archived"]; ok && !truthy(v) {
				return true
			}
		}
	}
	return false
}

// truthy reads a Bool field the way the model may write it.
func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case int64:
		return x != 0
	case int:
		return x != 0
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		return s == "true" || s == "1" || s == "是"
	}
	return false
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
	return hasAny(s, "药", "怎么治", "怎么吃", "medicine", "medication", "drug", "pill", "treat", "prescri")
}

// hasAny tells whether s has one of the words; English in any case.
func hasAny(s string, words ...string) bool {
	s = strings.ToLower(s)
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func diagnosisAsk(s string) bool {
	return hasAny(s, "什么问题", "什么病", "是不是", "诊断", "可能是", "what's wrong", "what is wrong", "diagnos", "could it be", "do i have")
}

func recordAsk(s string) bool {
	return hasAny(s, "记下", "记一下", "记录", "帮我记", "保存", "record", "log ", "note down", "write down", "save")
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
		h, err := st.FindHabit(it.Habit)
		if err != nil {
			return fmt.Sprintf("没有叫「%s」的打卡项。要新建就在 checkin 里设 create_if_missing=true（可以带 icon、unit），或者先用 add_record 写一条 habits（还能填 target），在同一次回复里再调用 checkin", strings.TrimSpace(it.Habit))
		}
		if truthy(h["archived"]) {
			return fmt.Sprintf("打卡项「%s」（habits #%d）已归档。用户要接着用它，先用 update_record 把它的 archived 改成 false，在同一次回复里再调用 checkin", text(h["name"]), idOf(h))
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
		return nil, msg.New("先说或写一点内容", "Say or type something first")
	}
	if len(in) > maxTurns {
		in = in[len(in)-maxTurns:]
	}
	out := make([]Turn, 0, len(in))
	images := 0
	for _, t := range in {
		role := strings.TrimSpace(t.Role)
		if role != "user" && role != "assistant" {
			return nil, msg.New("对话里有无法识别的一方", "The chat has a speaker it doesn't know")
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
			return nil, msg.Errorf("一次对话最多附 %d 张图片", "A chat can have at most %d pictures", maxImages*2)
		}
		if text == "" && len(pics) == 0 {
			continue
		}
		if utf8.RuneCountInString(text) > maxRunes {
			return nil, msg.Errorf("一次最多整理 %d 个字", "At most %d characters at a time", maxRunes)
		}
		out = append(out, Turn{Role: "user", Text: text, Images: pics})
	}
	if len(out) == 0 || out[len(out)-1].Role != "user" {
		return nil, msg.New("先说或写一点内容", "Say or type something first")
	}
	return out, nil
}

func cleanImages(in []Image) ([]Image, error) {
	if len(in) > maxImages {
		return nil, msg.Errorf("一次最多 %d 张图片", "At most %d pictures at a time", maxImages)
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
			return nil, msg.New("只能发送图片（JPG、PNG、GIF、WebP）", "Only pictures can be sent (JPG, PNG, GIF, WebP)")
		}
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil || len(raw) == 0 {
			return nil, msg.New("这张图片读不出来", "Couldn't read this picture")
		}
		if len(raw) > maxImageBytes {
			return nil, msg.New("这张图片太大", "This picture is too big")
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
