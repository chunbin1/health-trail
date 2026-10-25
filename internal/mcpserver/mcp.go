// Package mcpserver exposes the health records to Claude as MCP tools.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lee/health-trail/internal/store"
)

const instructions = `这是用户本人的个人健康档案（单人）。所有数据存在用户自己的电脑上。

使用约定：
- 先调用 health_overview 了解现状，再回答问题或写入。
- 日期用 YYYY-MM-DD，时间用 YYYY-MM-DD HH:MM（本地时间）；"today"/"今天" 也可以。
- 写入前先查重：新增健康问题、就诊、报告之前，用 search_records 或 list_records 看看是否已经有了，有就用 update_record 补充。
- 新的症状、就诊、用药如果属于已有的健康问题，填上 condition_id 关联起来。
- 用户说吃了某个药，用 add_record 记一条 doses：medication_id 填那个药的 id，taken_at 填服用时间（没说就是现在），amount 填颗数（默认 1）。health_overview 里的在用药物带有 last_taken（上次服用）和 taken_total（累计颗数）。
- 要按时吃的药，把每天吃的时刻填进 medications 的 remind_times（如 "08:00, 20:00"）：到点还没记吃药，用户的电脑会提醒。只说了"一天两次"没说几点时，问用户平时几点吃，不要自己定。health_overview 的 meds_today 是今天每个时刻吃了没有。
- 用户说要去做检查、复诊、体检等（以后的安排），用 add_record 记一条 appointments：scheduled_at 填日期和时刻；只说了"下午""明早"时问清几点，不要猜。地点填 place，空腹、带资料等要求填 preparation。会在前一天晚上 8 点和开始前（remind_before，默认 1 小时）提醒。health_overview 的 upcoming_appointments 是还没去的预约。
- 预约过后用户讲了就诊或检查结果：照常新建 visits 或 reports（报告用 add_lab_report），再用 update_record 把那条预约的 status 改成"已完成"，visit_id 或 report_id 填新记录的 id。取消了就把 status 改成"已取消"。
- 化验/体检报告（包括用户发来的截图或 PDF）用 add_lab_report 一次写入：每个指标一行，结果原样填（如 "111.87↑"），参考范围原样填（如 "0-6"），偏高偏低会自动判断。写入后把识别出的条目复述给用户核对。
- 不要编造数据；看不清的数值问用户，不要猜。
- 你可以帮助整理、对比、发现趋势、准备就医问题，但不能替代医生诊断；涉及用药调整或明显异常时，建议用户咨询医生。
- delete_record 会永久删除，只在用户明确要求时使用。`

// New builds the MCP server over st. local adds add_attachment, which
// reads files on this computer: pass true only for a client on this
// computer (stdio, or HTTP from loopback), never for another device.
func New(st *store.Store, version string, local bool) *mcp.Server {
	ins := instructions
	if local {
		ins = strings.Replace(ins, "\n- 不要编造数据", attachInstruction+"\n- 不要编造数据", 1)
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "health-trail", Title: "健康足迹", Version: version},
		&mcp.ServerOptions{Instructions: ins})
	t := &tools{st}
	ro := &mcp.ToolAnnotations{ReadOnlyHint: true}
	no := false
	yes := true

	mcp.AddTool(s, &mcp.Tool{Name: "health_overview", Annotations: ro,
		Description: "健康现状总览：进行中的健康问题、在用药物、今天各时刻的吃药情况、还没去的预约、最近就诊和待复诊、近 30 天症状、各指标最近一次结果中的异常项、各身体数据最新值、用户自定义的身体数据指标、今天的打卡情况、各类记录数量。回答任何健康问题前先调用。"},
		t.overview)
	mcp.AddTool(s, &mcp.Tool{Name: "search_records", Annotations: ro,
		Description: "在所有记录（健康问题、症状、就诊、用药、预约、报告、检验指标、身体数据、打卡项）中搜索文字。"},
		t.search)
	mcp.AddTool(s, &mcp.Tool{Name: "list_records", Annotations: ro,
		Description: "按类型列出记录，可按日期范围、关键字、字段值筛选，默认按日期从新到旧。\n\n" + typesDoc()},
		t.list)
	mcp.AddTool(s, &mcp.Tool{Name: "get_record", Annotations: ro,
		Description: "读取一条记录及其关联内容：健康问题会带上相关症状/就诊/用药/预约；就诊带上开的药和报告；报告带上全部检验指标；打卡项带上打卡记录。附件列表也会一起返回。"},
		t.get)
	mcp.AddTool(s, &mcp.Tool{Name: "add_record",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &no},
		Description: "新增一条记录。fields 的键用下面列出的字段名。化验/体检报告请用 add_lab_report，打卡请用 checkin。\n\n" + typesDoc()},
		t.add)
	mcp.AddTool(s, &mcp.Tool{Name: "update_record",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &no, IdempotentHint: true},
		Description: "修改一条记录：只需给出要改的字段；把字段设为空字符串或 null 即清空。字段名见 add_record。"},
		t.update)
	mcp.AddTool(s, &mcp.Tool{Name: "delete_record",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &yes},
		Description: "永久删除一条记录（删除报告会连同其检验指标一起删除）。打卡项不能删除：用户不再打卡时，用 update_record 把它的 archived 设为 true（归档），打卡记录会保留。只在用户明确要求删除时使用。"},
		t.delete)
	mcp.AddTool(s, &mcp.Tool{Name: "add_lab_report",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &no},
		Description: "录入一份化验/体检报告及其全部指标（全部成功或全部不写入）。用 report 新建报告，或用 report_id 往已有报告里追加指标。" +
			"results 每项：item_name 项目名；value 结果（原样，如 \"111.87↑\"、\"14.1↓\"、\"阴性\"、\"<0.5\"，也可以是数字）；unit 单位；ref_range 参考范围（原样，如 \"3.5-9.5\"、\"<5\"）；可选 flag（偏高/偏低/正常/异常，不填则按箭头或参考范围自动判断）、panel 分组（如 血常规）。"},
		t.addLabReport)
	mcp.AddTool(s, &mcp.Tool{Name: "list_lab_items", Annotations: ro,
		Description: "列出做过的所有检验项目，每项有检测次数和最近一次的结果、单位、标记。"},
		t.labItems)
	mcp.AddTool(s, &mcp.Tool{Name: "get_lab_trend", Annotations: ro,
		Description: "某个检验项目的历次结果（从旧到新），含参考范围和标记。项目名先精确匹配，没有再模糊匹配（如 \"反应蛋白\"、\"白细胞\"）。"},
		t.labTrend)
	mcp.AddTool(s, &mcp.Tool{Name: "get_metric_trend", Annotations: ro,
		Description: "某项身体数据在一段时间内的全部读数（从旧到新）。metric_type 可选：" + strings.Join(store.MetricTypes, "、") + "，以及用户自定义的指标（health_overview 的 metric_kinds）。血压的 value 是收缩压、value2 是舒张压。每条读数的 source 说明来源：metrics 是身体数据记录；lab_results 是同名、同单位的检验指标（化验单上的结果，只是一起列出，id 是检验指标的 id，要改就改那条检验指标）。"},
		t.metricTrend)
	mcp.AddTool(s, &mcp.Tool{Name: "checkin",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &no, IdempotentHint: true},
		Description: "给打卡项打卡（如 吃药、运动、早睡）。同一天再打一次会更新数量和备注。打卡项按名称或 id 找；不存在时设 create_if_missing=true 会新建。"},
		t.checkin)
	mcp.AddTool(s, &mcp.Tool{Name: "undo_checkin",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &yes, IdempotentHint: true},
		Description: "取消某天的打卡。"},
		t.undoCheckin)
	mcp.AddTool(s, &mcp.Tool{Name: "get_checkin_stats", Annotations: ro,
		Description: "打卡统计：每个打卡项在一段时间内（默认最近 30 天）的完成天数、完成率、当前连续天数、区间内最长连续、数量合计和打卡日期。已归档的打卡项只要在这段时间里打过卡也会列出（habit.archived 为 1）。"},
		t.checkinStats)
	if local {
		mcp.AddTool(s, &mcp.Tool{Name: "add_attachment",
			Annotations: &mcp.ToolAnnotations{DestructiveHint: &no},
			Description: "把这台电脑上的一个文件（报告单截图、PDF、片子录屏、处方照片等）存成附件，挂到一条记录下。文件会复制进档案，原文件不动；最大 " + fmt.Sprint(store.MaxAttachment>>20) + " MB。get_record 会列出一条记录的附件。"},
			t.attach)
	}
	addPrompts(s)
	return s
}

// typesDoc describes every record type and its fields, from the schema.
func typesDoc() string {
	var b strings.Builder
	b.WriteString("记录类型（type）及字段：\n")
	for _, e := range store.Entities {
		if e.Name == "checkins" {
			continue
		}
		fmt.Fprintf(&b, "- %s（%s）：", e.Name, e.Label)
		parts := make([]string, 0, len(e.Fields))
		for _, f := range e.Fields {
			p := f.Name + " " + f.Label
			var extra []string
			if f.Required {
				extra = append(extra, "必填")
			}
			switch f.Type {
			case store.Date:
				extra = append(extra, "日期")
			case store.DateTime:
				extra = append(extra, "时间")
			case store.Time:
				extra = append(extra, "每天的时刻 HH:MM")
			case store.Times:
				extra = append(extra, "每天的时刻，逗号分开，如 08:00, 20:00")
			case store.Int, store.Real:
				extra = append(extra, "数字")
			case store.Bool:
				extra = append(extra, "true/false")
			case store.Ref:
				extra = append(extra, f.Ref+" 的 id")
			case store.Enum:
				extra = append(extra, strings.Join(f.Options, "/"))
				if f.Custom != "" {
					extra = append(extra, "或 "+f.Custom+" 里的名称")
				}
			}
			if len(extra) > 0 {
				p += "（" + strings.Join(extra, "，") + "）"
			}
			parts = append(parts, p)
		}
		b.WriteString(strings.Join(parts, "；") + "\n")
	}
	return b.String()
}

type tools struct{ st *store.Store }

func result(v any) (*mcp.CallToolResult, any, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: b.String()}}}, nil, nil
}

// failed reports an error to Claude as a tool result, so it can read the
// message (which field, which values are allowed) and try again.
func failed(err error) (*mcp.CallToolResult, any, error) {
	r := &mcp.CallToolResult{}
	r.SetError(err)
	return r, nil, nil
}

func reply(v any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return failed(err)
	}
	return result(v)
}

type empty struct{}

func (t *tools) overview(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
	return reply(t.st.Overview())
}

type searchIn struct {
	Query string `json:"query" jsonschema:"要搜索的文字，如 胃、头孢、协和"`
	Limit int    `json:"limit,omitempty" jsonschema:"每种记录最多返回几条，默认 20"`
}

func (t *tools) search(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
	return reply(t.st.Search(in.Query, in.Limit))
}

type listIn struct {
	Type    string         `json:"type" jsonschema:"记录类型，如 visits、lab_results、metrics"`
	From    string         `json:"from,omitempty" jsonschema:"起始日期（含），YYYY-MM-DD"`
	To      string         `json:"to,omitempty" jsonschema:"结束日期（含），YYYY-MM-DD"`
	Query   string         `json:"query,omitempty" jsonschema:"关键字"`
	Filters map[string]any `json:"filters,omitempty" jsonschema:"按字段值精确筛选，如 {\"status\":\"进行中\"} 或 {\"condition_id\":3}"`
	Limit   int            `json:"limit,omitempty" jsonschema:"最多返回几条，默认 50"`
	Offset  int            `json:"offset,omitempty"`
	Oldest  bool           `json:"oldest_first,omitempty" jsonschema:"true 则从旧到新"`
}

func (t *tools) list(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, any, error) {
	if in.Limit == 0 {
		in.Limit = 50
	}
	rows, total, err := t.st.List(in.Type, store.ListQuery{Q: in.Query, From: in.From, To: in.To,
		Filters: in.Filters, Limit: in.Limit, Offset: in.Offset, Asc: in.Oldest})
	if err != nil {
		return failed(err)
	}
	return result(map[string]any{"total": total, "returned": len(rows), "rows": rows})
}

type refIn struct {
	Type string `json:"type" jsonschema:"记录类型"`
	ID   int64  `json:"id" jsonschema:"记录 id"`
}

func (t *tools) get(ctx context.Context, _ *mcp.CallToolRequest, in refIn) (*mcp.CallToolResult, any, error) {
	return reply(t.st.Related(in.Type, in.ID))
}

type addIn struct {
	Type   string         `json:"type" jsonschema:"记录类型：conditions、symptoms、visits、medications、doses、appointments、reports、metrics、metric_kinds、habits"`
	Fields map[string]any `json:"fields" jsonschema:"字段名到值"`
}

func (t *tools) add(ctx context.Context, _ *mcp.CallToolRequest, in addIn) (*mcp.CallToolResult, any, error) {
	return reply(t.st.Create(in.Type, in.Fields))
}

type updateIn struct {
	Type   string         `json:"type" jsonschema:"记录类型"`
	ID     int64          `json:"id" jsonschema:"记录 id"`
	Fields map[string]any `json:"fields" jsonschema:"要修改的字段名到新值"`
}

func (t *tools) update(ctx context.Context, _ *mcp.CallToolRequest, in updateIn) (*mcp.CallToolResult, any, error) {
	return reply(t.st.Update(in.Type, in.ID, in.Fields))
}

func (t *tools) delete(ctx context.Context, _ *mcp.CallToolRequest, in refIn) (*mcp.CallToolResult, any, error) {
	if err := t.st.Delete(in.Type, in.ID); err != nil {
		return failed(err)
	}
	return result(map[string]any{"deleted": true, "type": in.Type, "id": in.ID})
}

type labResult struct {
	ItemName string `json:"item_name" jsonschema:"项目名，照报告原样，如 超敏C-反应蛋白"`
	Value    any    `json:"value" jsonschema:"结果，照报告原样，如 \"111.87↑\"、\"阴性\"，或数字"`
	Unit     string `json:"unit,omitempty" jsonschema:"单位，如 mg/L、10^9/L"`
	RefRange string `json:"ref_range,omitempty" jsonschema:"参考范围，照报告原样，如 0-6"`
	Flag     string `json:"flag,omitempty" jsonschema:"偏高/偏低/正常/异常；不填则自动判断"`
	Panel    string `json:"panel,omitempty" jsonschema:"分组，如 血常规"`
	TestDate string `json:"test_date,omitempty" jsonschema:"检测日期，不填则用报告日期"`
	Notes    string `json:"notes,omitempty"`
}

type labReportIn struct {
	Report   map[string]any `json:"report,omitempty" jsonschema:"新建报告的字段：report_date（必填）、title（必填）、report_type（体检/化验/影像/病理/其他）、institution、conclusion、content、visit_id、notes"`
	ReportID int64          `json:"report_id,omitempty" jsonschema:"往已有报告追加时填报告 id"`
	Panel    string         `json:"panel,omitempty" jsonschema:"所有指标的默认分组，如 血常规"`
	Results  []labResult    `json:"results" jsonschema:"指标列表"`
}

func (t *tools) addLabReport(ctx context.Context, _ *mcp.CallToolRequest, in labReportIn) (*mcp.CallToolResult, any, error) {
	rows := make([]store.Record, len(in.Results))
	for i, r := range in.Results {
		row := store.Record{"item_name": r.ItemName, "unit": r.Unit, "ref_range": r.RefRange,
			"flag": r.Flag, "panel": r.Panel, "test_date": r.TestDate, "notes": r.Notes}
		switch v := r.Value.(type) {
		case float64:
			row["value_num"] = v
		case string:
			row["value_text"] = v
		case nil:
		default:
			row["value_text"] = fmt.Sprint(v)
		}
		rows[i] = row
	}
	id := ""
	if in.ReportID > 0 {
		id = fmt.Sprint(in.ReportID)
	}
	res, err := importLabs(t.st, id, in.Report, in.Panel, rows)
	if err != nil {
		return failed(err)
	}
	// a compact echo for Claude to read back to the person
	lines := []string{}
	abnormal := 0
	for _, r := range res["lab_results"].([]store.Record) {
		v := r["value_text"]
		if v == nil {
			v = r["value_num"]
		}
		flag, _ := r["flag"].(string)
		if flag == "偏高" || flag == "偏低" || flag == "异常" {
			abnormal++
		}
		lines = append(lines, fmt.Sprintf("%v %v %v（参考 %v）%s", r["item_name"], v, orEmpty(r["unit"]), orEmpty(r["ref_range"]), flag))
	}
	rep := res["report"].(store.Record)
	return result(map[string]any{
		"report":         rep,
		"saved":          len(lines),
		"abnormal_count": abnormal,
		"results":        lines,
	})
}

func orEmpty(v any) any {
	if v == nil {
		return "-"
	}
	return v
}

func (t *tools) labItems(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, any, error) {
	return reply(t.st.LabItems())
}

type trendIn struct {
	ItemName string `json:"item_name" jsonschema:"检验项目名，可以是部分名称"`
}

func (t *tools) labTrend(ctx context.Context, _ *mcp.CallToolRequest, in trendIn) (*mcp.CallToolResult, any, error) {
	rows, err := t.st.LabTrend(in.ItemName)
	if err != nil {
		return failed(err)
	}
	if len(rows) == 0 {
		return failed(fmt.Errorf("没有找到检验项目 %q，可以先用 list_lab_items 查看有哪些", in.ItemName))
	}
	return result(rows)
}

type metricIn struct {
	MetricType string `json:"metric_type" jsonschema:"指标，如 体重、血压"`
	From       string `json:"from,omitempty" jsonschema:"起始日期（含）"`
	To         string `json:"to,omitempty" jsonschema:"结束日期（含）"`
}

func (t *tools) metricTrend(ctx context.Context, _ *mcp.CallToolRequest, in metricIn) (*mcp.CallToolResult, any, error) {
	return reply(t.st.MetricSeries(in.MetricType, in.From, in.To))
}

type checkinIn struct {
	Habit           string   `json:"habit" jsonschema:"打卡项名称或 id，如 吃药、跑步"`
	Date            string   `json:"date,omitempty" jsonschema:"日期，只能是今天，可不填；过去的打卡不能补记、修改或撤销"`
	Value           *float64 `json:"value,omitempty" jsonschema:"数量（打卡项有单位时），如 跑了 5 公里填 5。打卡项设了 metric_type（如 体重）时填测量值，会同时记一条身体数据"`
	Notes           string   `json:"notes,omitempty"`
	CreateIfMissing bool     `json:"create_if_missing,omitempty" jsonschema:"打卡项不存在时新建"`
	Unit            string   `json:"unit,omitempty" jsonschema:"新建打卡项时的单位"`
	Icon            string   `json:"icon,omitempty" jsonschema:"新建打卡项时的 emoji 图标"`
}

func (t *tools) checkin(ctx context.Context, _ *mcp.CallToolRequest, in checkinIn) (*mcp.CallToolResult, any, error) {
	h, err := t.st.FindHabit(in.Habit)
	if err == store.ErrNotFound && in.CreateIfMissing {
		h, err = t.st.Create("habits", store.Record{"name": in.Habit, "unit": in.Unit, "icon": in.Icon})
	}
	if err == store.ErrNotFound {
		return failed(fmt.Errorf("没有叫 %q 的打卡项。现有的：%s。要新建请设 create_if_missing=true", in.Habit, t.habitNames()))
	}
	if err != nil {
		return failed(err)
	}
	if in.Date == "" {
		in.Date = "today"
	}
	c, err := t.st.Checkin(h["id"].(int64), in.Date, in.Value, in.Notes)
	if err != nil {
		return failed(err)
	}
	return result(map[string]any{"habit": h, "checkin": c})
}

func (t *tools) habitNames() string {
	rows, _, _ := t.st.List("habits", store.ListQuery{Limit: 100})
	names := []string{}
	for _, r := range rows {
		names = append(names, fmt.Sprint(r["name"]))
	}
	if len(names) == 0 {
		return "（还没有）"
	}
	return strings.Join(names, "、")
}

type undoIn struct {
	Habit string `json:"habit" jsonschema:"打卡项名称或 id"`
	Date  string `json:"date,omitempty" jsonschema:"日期，只能是今天，可不填；过去的打卡不能补记、修改或撤销"`
}

func (t *tools) undoCheckin(ctx context.Context, _ *mcp.CallToolRequest, in undoIn) (*mcp.CallToolResult, any, error) {
	h, err := t.st.FindHabit(in.Habit)
	if err != nil {
		return failed(fmt.Errorf("没有叫 %q 的打卡项", in.Habit))
	}
	if in.Date == "" {
		in.Date = "today"
	}
	ok, err := t.st.Uncheck(h["id"].(int64), in.Date)
	if err != nil {
		return failed(err)
	}
	return result(map[string]any{"removed": ok, "habit": h["name"]})
}

type statsIn struct {
	Habit string `json:"habit,omitempty" jsonschema:"只看某个打卡项（名称或 id），不填则全部"`
	From  string `json:"from,omitempty" jsonschema:"起始日期，默认 30 天前"`
	To    string `json:"to,omitempty" jsonschema:"结束日期，默认今天"`
}

func (t *tools) checkinStats(ctx context.Context, _ *mcp.CallToolRequest, in statsIn) (*mcp.CallToolResult, any, error) {
	var id int64
	if in.Habit != "" {
		h, err := t.st.FindHabit(in.Habit)
		if err != nil {
			return failed(fmt.Errorf("没有叫 %q 的打卡项。现有的：%s", in.Habit, t.habitNames()))
		}
		id = h["id"].(int64)
	}
	return reply(t.st.CheckinStats(in.From, in.To, id))
}

// importLabs saves a set of lab results under a report: an existing one
// (reportID), or a new one made from `report`. A report it creates is
// removed again if the results can't be saved, so a failed import leaves
// nothing behind.
func importLabs(st *store.Store, reportID string, report store.Record, panel string, rows []store.Record) (store.Record, error) {
	if len(rows) == 0 {
		return nil, &store.ValidationError{Msg: "没有要保存的指标"}
	}
	var rep store.Record
	var err error
	created := false
	if reportID != "" {
		id, perr := strconv.ParseInt(reportID, 10, 64)
		if perr != nil {
			return nil, &store.ValidationError{Msg: "无效的 report_id"}
		}
		if rep, err = st.Get("reports", id); err != nil {
			return nil, err
		}
	} else {
		if report == nil {
			return nil, &store.ValidationError{Msg: "需要 report_id，或用 report 新建一份报告"}
		}
		if rep, err = st.Create("reports", report); err != nil {
			return nil, err
		}
		created = true
	}
	for _, row := range rows {
		row["report_id"] = rep["id"]
		if row["test_date"] == nil || row["test_date"] == "" {
			row["test_date"] = rep["report_date"]
		}
		if panel != "" && (row["panel"] == nil || row["panel"] == "") {
			row["panel"] = panel
		}
	}
	results, err := st.CreateMany("lab_results", rows)
	if err != nil {
		if created {
			st.Delete("reports", rep["id"].(int64))
		}
		return nil, err
	}
	return store.Record{"report": rep, "lab_results": results}, nil
}
