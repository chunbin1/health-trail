package store

// The schema is declared once, here. The SQLite tables, the REST API's
// accepted fields, the MCP tools' record types and the UI's forms are all
// derived from it, so a field added here shows up everywhere.

type FieldType string

const (
	Text     FieldType = "text"     // single line
	LongText FieldType = "longtext" // multi line
	Int      FieldType = "int"
	Real     FieldType = "real"
	Date     FieldType = "date"     // YYYY-MM-DD
	DateTime FieldType = "datetime" // YYYY-MM-DD HH:MM
	Time     FieldType = "time"     // HH:MM, a time of day
	Enum     FieldType = "enum"
	Ref      FieldType = "ref" // id of a row in another entity
	Bool     FieldType = "bool"
)

type Field struct {
	Name     string    `json:"name"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required,omitempty"`
	Options  []string  `json:"options,omitempty"` // Enum values
	Ref      string    `json:"ref,omitempty"`     // Ref target entity
	Cascade  bool      `json:"-"`                 // Ref: delete this row with its target (else set null)
	Unit     string    `json:"unit,omitempty"`
	Hint     string    `json:"hint,omitempty"`
	List     bool      `json:"list,omitempty"` // shown as a column in list views
}

type Entity struct {
	Name     string  `json:"name"`
	Label    string  `json:"label"`
	DateKey  string  `json:"dateKey"`  // field records are ordered and filtered by
	TitleKey string  `json:"titleKey"` // field that names a record
	Fields   []Field `json:"fields"`
	Unique   []string `json:"-"`
}

func (e *Entity) Field(name string) *Field {
	for i := range e.Fields {
		if e.Fields[i].Name == name {
			return &e.Fields[i]
		}
	}
	return nil
}

var conditionRef = Field{Name: "condition_id", Label: "相关健康问题", Type: Ref, Ref: "conditions"}

var Entities = []*Entity{
	{
		Name: "conditions", Label: "健康问题", DateKey: "onset_date", TitleKey: "name",
		Fields: []Field{
			{Name: "name", Label: "名称", Type: Text, Required: true, List: true, Hint: "如：慢性胃炎、过敏性鼻炎"},
			{Name: "category", Label: "类别", Type: Text, List: true, Hint: "如：消化、呼吸、骨科"},
			{Name: "status", Label: "状态", Type: Enum, Options: []string{"进行中", "慢性", "观察中", "已痊愈"}, List: true},
			{Name: "onset_date", Label: "开始日期", Type: Date, List: true},
			{Name: "resolved_date", Label: "痊愈日期", Type: Date},
			{Name: "notes", Label: "备注", Type: LongText},
		},
	},
	{
		Name: "symptoms", Label: "症状日志", DateKey: "occurred_at", TitleKey: "description",
		Fields: []Field{
			{Name: "occurred_at", Label: "发生时间", Type: DateTime, Required: true, List: true},
			{Name: "description", Label: "症状", Type: Text, Required: true, List: true, Hint: "如：上腹隐痛、头痛"},
			{Name: "severity", Label: "严重程度", Type: Int, List: true, Hint: "1（轻）- 10（重）"},
			{Name: "duration", Label: "持续时间", Type: Text, Hint: "如：2 小时"},
			{Name: "trigger", Label: "诱因", Type: Text, Hint: "如：空腹、受凉、熬夜"},
			conditionRef,
			{Name: "notes", Label: "备注", Type: LongText},
		},
	},
	{
		Name: "visits", Label: "就诊记录", DateKey: "visit_date", TitleKey: "diagnosis",
		Fields: []Field{
			{Name: "visit_date", Label: "就诊日期", Type: Date, Required: true, List: true},
			{Name: "hospital", Label: "医院", Type: Text, List: true},
			{Name: "department", Label: "科室", Type: Text, List: true},
			{Name: "doctor", Label: "医生", Type: Text},
			{Name: "visit_type", Label: "类型", Type: Enum, Options: []string{"门诊", "复诊", "急诊", "住院", "体检", "线上问诊"}},
			{Name: "chief_complaint", Label: "主诉", Type: LongText, Hint: "因为什么去看"},
			{Name: "diagnosis", Label: "诊断", Type: Text, List: true},
			{Name: "treatment", Label: "处理", Type: LongText, Hint: "检查、处方、操作"},
			{Name: "advice", Label: "医嘱", Type: LongText},
			{Name: "follow_up_date", Label: "复诊日期", Type: Date},
			{Name: "cost", Label: "费用", Type: Real, Unit: "元"},
			conditionRef,
			{Name: "notes", Label: "备注", Type: LongText},
		},
	},
	{
		Name: "medications", Label: "用药", DateKey: "start_date", TitleKey: "name",
		Fields: []Field{
			{Name: "name", Label: "药名", Type: Text, Required: true, List: true},
			{Name: "dosage", Label: "剂量", Type: Text, List: true, Hint: "如：20mg"},
			{Name: "frequency", Label: "频次", Type: Text, List: true, Hint: "如：每日 2 次，饭前"},
			{Name: "status", Label: "状态", Type: Enum, Options: []string{"在用", "按需", "计划中", "已停用"}, List: true},
			{Name: "start_date", Label: "开始日期", Type: Date, List: true},
			{Name: "end_date", Label: "结束日期", Type: Date},
			{Name: "purpose", Label: "用途", Type: Text},
			{Name: "side_effects", Label: "不良反应", Type: Text},
			conditionRef,
			{Name: "visit_id", Label: "开药就诊", Type: Ref, Ref: "visits"},
			{Name: "notes", Label: "备注", Type: LongText},
		},
	},
	{
		Name: "doses", Label: "服药记录", DateKey: "taken_at", TitleKey: "taken_at",
		Fields: []Field{
			{Name: "medication_id", Label: "药物", Type: Ref, Ref: "medications", Required: true, Cascade: true, List: true},
			{Name: "taken_at", Label: "服用时间", Type: DateTime, Required: true, List: true},
			{Name: "amount", Label: "数量", Type: Real, Unit: "颗", List: true, Hint: "不填按 1 颗算"},
			{Name: "notes", Label: "备注", Type: Text},
		},
	},
	{
		Name: "reports", Label: "检查报告", DateKey: "report_date", TitleKey: "title",
		Fields: []Field{
			{Name: "report_date", Label: "报告日期", Type: Date, Required: true, List: true},
			{Name: "title", Label: "标题", Type: Text, Required: true, List: true, Hint: "如：2026 年度体检、血常规+CRP"},
			{Name: "report_type", Label: "类型", Type: Enum, Options: []string{"体检", "化验", "影像", "病理", "其他"}, List: true},
			{Name: "institution", Label: "机构", Type: Text, List: true},
			{Name: "conclusion", Label: "结论", Type: LongText, Hint: "报告上的结论或医生意见"},
			{Name: "content", Label: "报告全文", Type: LongText, Hint: "影像描述等不适合拆成指标的内容"},
			{Name: "visit_id", Label: "关联就诊", Type: Ref, Ref: "visits"},
			{Name: "notes", Label: "备注", Type: LongText},
		},
	},
	{
		Name: "lab_results", Label: "检验指标", DateKey: "test_date", TitleKey: "item_name",
		Fields: []Field{
			{Name: "report_id", Label: "所属报告", Type: Ref, Ref: "reports", Cascade: true},
			{Name: "test_date", Label: "检测日期", Type: Date, Required: true, List: true},
			{Name: "panel", Label: "分组", Type: Text, List: true, Hint: "如：血常规、肝功能"},
			{Name: "item_name", Label: "项目", Type: Text, Required: true, List: true},
			{Name: "value_num", Label: "数值", Type: Real, List: true},
			{Name: "value_text", Label: "结果（文字）", Type: Text, Hint: "阴性/阳性等非数值结果"},
			{Name: "unit", Label: "单位", Type: Text, List: true},
			{Name: "ref_range", Label: "参考范围", Type: Text, List: true, Hint: "如：3.5-9.5"},
			{Name: "ref_low", Label: "参考下限", Type: Real},
			{Name: "ref_high", Label: "参考上限", Type: Real},
			{Name: "flag", Label: "标记", Type: Enum, Options: []string{"正常", "偏高", "偏低", "异常"}, List: true},
			{Name: "notes", Label: "备注", Type: Text},
		},
	},
	{
		Name: "metrics", Label: "身体数据", DateKey: "measured_at", TitleKey: "metric_type",
		Fields: []Field{
			{Name: "measured_at", Label: "测量时间", Type: DateTime, Required: true, List: true},
			{Name: "metric_type", Label: "指标", Type: Enum, Required: true, List: true, Options: MetricTypes},
			{Name: "value", Label: "数值", Type: Real, Required: true, List: true, Hint: "血压填收缩压"},
			{Name: "value2", Label: "数值 2", Type: Real, List: true, Hint: "血压填舒张压"},
			{Name: "unit", Label: "单位", Type: Text, List: true},
			{Name: "context", Label: "情境", Type: Text, Hint: "如：空腹、餐后 2 小时、晨起"},
			{Name: "notes", Label: "备注", Type: Text},
		},
	},
	{
		Name: "habits", Label: "打卡项", DateKey: "", TitleKey: "name",
		Fields: []Field{
			{Name: "name", Label: "名称", Type: Text, Required: true, List: true, Hint: "如：吃药、运动、早睡"},
			{Name: "icon", Label: "图标", Type: Text, List: true, Hint: "一个 emoji，如 💊"},
			{Name: "unit", Label: "单位", Type: Text, List: true, Hint: "需要记数量时填，如：分钟、杯"},
			{Name: "target", Label: "每日目标", Type: Real, List: true},
			{Name: "metric_type", Label: "同时记入身体数据", Type: Enum, Options: HabitMetricTypes, Hint: "如 晨起称体重 选 体重：打卡时填数值，同时记一条身体数据"},
			{Name: "remind_at", Label: "提醒时间", Type: Time, List: true, Hint: "如 23:00：每天到这个时间还没打卡，桌面 App 会发系统通知"},
			{Name: "archived", Label: "已归档", Type: Bool},
			{Name: "notes", Label: "备注", Type: Text},
		},
		Unique: []string{"name"},
	},
	{
		Name: "checkins", Label: "打卡记录", DateKey: "date", TitleKey: "date",
		Fields: []Field{
			{Name: "habit_id", Label: "打卡项", Type: Ref, Ref: "habits", Required: true, Cascade: true, List: true},
			{Name: "date", Label: "日期", Type: Date, Required: true, List: true},
			{Name: "value", Label: "数量", Type: Real, List: true},
			{Name: "metric_id", Label: "身体数据记录", Type: Ref, Ref: "metrics"},
			{Name: "notes", Label: "备注", Type: Text},
		},
		Unique: []string{"habit_id", "date"},
	},
}

// MetricTypes are the body metrics the app knows a default unit for.
var MetricTypes = []string{"体重", "血压", "心率", "血糖", "体温", "血氧", "腰围", "睡眠时长", "步数"}

// HabitMetricTypes are the metrics a habit can log with its check-in: those
// measured as a single number (not 血压, which needs two).
var HabitMetricTypes = []string{"体重", "心率", "血糖", "体温", "血氧", "腰围", "睡眠时长", "步数"}

var MetricUnits = map[string]string{
	"体重": "kg", "血压": "mmHg", "心率": "次/分", "血糖": "mmol/L", "体温": "°C",
	"血氧": "%", "腰围": "cm", "睡眠时长": "小时", "步数": "步",
}

func EntityByName(name string) *Entity {
	for _, e := range Entities {
		if e.Name == name {
			return e
		}
	}
	return nil
}

func EntityNames() []string {
	names := make([]string, len(Entities))
	for i, e := range Entities {
		names[i] = e.Name
	}
	return names
}
