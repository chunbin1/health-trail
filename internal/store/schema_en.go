package store

// English text for the UI's English mode. The schema's labels, hints and
// options stay Chinese — enum options are the values stored in the database,
// and the MCP tools describe fields in Chinese — so the UI looks up what it
// shows here instead. TestEnglishCoversSchema fails when a field added to
// schema.go has no English yet.

// English maps each Chinese label, hint, enum option and unit in the schema
// to the English the UI shows for it.
var English = map[string]string{
	// entities
	"健康问题": "Health issues",
	"症状日志": "Symptom log",
	"就诊记录": "Visits",
	"用药":   "Medications",
	"服药记录": "Doses taken",
	"检查报告": "Reports",
	"检验指标": "Lab results",
	"身体数据": "Body metrics",
	"打卡项":  "Check-in items",
	"打卡记录": "Check-ins",

	// custom metrics
	"自定义指标":               "Custom metrics",
	"如：尿酸、体脂率、视力":         "e.g. Uric acid, Body fat, Vision",
	"如：μmol/L、%；没有单位可以不填": "e.g. μmol/L, %; leave blank if it has none",

	// shared fields
	"名称":     "Name",
	"类别":     "Category",
	"状态":     "Status",
	"开始日期":   "Start date",
	"备注":     "Notes",
	"类型":     "Type",
	"单位":     "Unit",
	"数值":     "Value",
	"数量":     "Amount",
	"日期":     "Date",
	"相关健康问题": "Related health issue",

	// conditions
	"痊愈日期":         "Resolved date",
	"如：慢性胃炎、过敏性鼻炎": "e.g. chronic gastritis, allergic rhinitis",
	"如：消化、呼吸、骨科":   "e.g. digestive, respiratory, orthopedic",
	"进行中":          "Active",
	"慢性":           "Chronic",
	"观察中":          "Monitoring",
	"已痊愈":          "Resolved",

	// symptoms
	"发生时间":        "When",
	"症状":          "Symptom",
	"如：上腹隐痛、头痛":   "e.g. dull upper abdominal pain, headache",
	"严重程度":        "Severity",
	"1（轻）- 10（重）": "1 (mild) – 10 (severe)",
	"持续时间":        "Duration",
	"如：2 小时":      "e.g. 2 hours",
	"诱因":          "Trigger",
	"如：空腹、受凉、熬夜":  "e.g. empty stomach, caught a chill, stayed up late",

	// visits
	"就诊日期":     "Visit date",
	"医院":       "Hospital",
	"科室":       "Department",
	"医生":       "Doctor",
	"门诊":       "Outpatient",
	"复诊":       "Follow-up",
	"急诊":       "Emergency",
	"住院":       "Inpatient",
	"体检":       "Checkup",
	"线上问诊":     "Online consult",
	"主诉":       "Chief complaint",
	"因为什么去看":   "Why you went",
	"诊断":       "Diagnosis",
	"处理":       "Treatment",
	"检查、处方、操作": "Tests, prescriptions, procedures",
	"医嘱":       "Doctor's advice",
	"复诊日期":     "Follow-up date",
	"费用":       "Cost",
	"元":        "CNY",
	"就诊":       "Visit",

	// medications
	"药名":          "Drug",
	"剂量":          "Dosage",
	"如：20mg":      "e.g. 20mg",
	"频次":          "Frequency",
	"如：每日 2 次，饭前": "e.g. twice daily, before meals",
	"在用":          "Taking",
	"按需":          "As needed",
	"计划中":         "Planned",
	"已停用":         "Stopped",
	"结束日期":        "End date",
	"用途":          "Purpose",
	"不良反应":        "Side effects",
	"开药就诊":        "Prescribed at visit",

	// doses
	"药物":       "Medication",
	"服用时间":     "Taken at",
	"颗":        "pill(s)",
	"不填按 1 颗算": "Leave blank for 1 pill",

	// reports
	"报告日期": "Report date",
	"标题":   "Title",
	"如：2026 年度体检、血常规+CRP": "e.g. 2026 annual checkup, CBC + CRP",
	"化验":          "Lab test",
	"影像":          "Imaging",
	"病理":          "Pathology",
	"其他":          "Other",
	"机构":          "Institution",
	"结论":          "Conclusion",
	"报告上的结论或医生意见": "The report's conclusion or the doctor's opinion",
	"报告全文":        "Full text",
	"影像描述等不适合拆成指标的内容": "Imaging findings and other text that isn't a list of results",
	"关联就诊": "Related visit",

	// lab results
	"所属报告":        "Report",
	"检测日期":        "Test date",
	"分组":          "Panel",
	"如：血常规、肝功能":   "e.g. CBC, liver function",
	"项目":          "Test",
	"结果（文字）":      "Result (text)",
	"阴性/阳性等非数值结果": "Non-numeric results such as negative/positive",
	"参考范围":        "Reference range",
	"如：3.5-9.5":   "e.g. 3.5-9.5",
	"参考下限":        "Reference low",
	"参考上限":        "Reference high",
	"标记":          "Flag",
	"正常":          "Normal",
	"偏高":          "High",
	"偏低":          "Low",
	"异常":          "Abnormal",

	// metrics
	"测量时间":   "Measured at",
	"指标":     "Metric",
	"血压填收缩压": "For blood pressure, the systolic",
	"数值 2":   "Value 2",
	"血压填舒张压": "For blood pressure, the diastolic",
	"情境":     "Context",
	"如：空腹、餐后 2 小时、晨起": "e.g. fasting, 2 h after a meal, on waking",
	"体重":   "Weight",
	"身高":   "Height",
	"血压":   "Blood pressure",
	"心率":   "Heart rate",
	"血糖":   "Blood glucose",
	"体温":   "Temperature",
	"血氧":   "Blood oxygen",
	"腰围":   "Waist",
	"睡眠时长": "Sleep",
	"步数":   "Steps",
	"次/分":  "bpm",
	"小时":   "h",
	"步":    "steps",
	// BMI's bands (bmiCategory)
	"偏瘦": "Underweight",
	"超重": "Overweight",
	"肥胖": "Obese",

	// habits
	"如：吃药、运动、早睡":     "e.g. take meds, exercise, sleep early",
	"图标":             "Icon",
	"一个 emoji，如 💊":   "An emoji, e.g. 💊",
	"需要记数量时填，如：分钟、杯": "Fill in to log an amount, e.g. minutes, cups",
	"每日目标":           "Daily goal",
	"同时记入身体数据":       "Also log as body metric",
	"如 晨起称体重 选 体重：打卡时填数值，同时记一条身体数据": "e.g. choose Weight for a morning weigh-in: the check-in asks for the reading and logs it as a body metric too",
	"提醒时间": "Reminder",
	"如 23:00：每天到这个时间还没打卡，桌面 App 会发系统通知": "e.g. 23:00: if it isn't checked off by then, the desktop app sends a notification",
	"已归档": "Archived",

	// checkins
	"身体数据记录": "Body metric record",

	// medications' reminders
	"每天几点吃：到点还没记吃药，会发提醒": "When to take it each day: if no dose is logged by then, a reminder comes",

	// appointments
	"预约":           "Appointments",
	"时间":           "Time",
	"事项":           "What",
	"如：胃镜检查、消化科复诊": "e.g. Gastroscopy, GI follow-up",
	"检查":           "Exam",
	"地点":           "Place",
	"医院、科室":        "Hospital, department",
	"待去":           "Upcoming",
	"已完成":          "Done",
	"已取消":          "Cancelled",
	"提前提醒":         "Remind me",
	"不选是提前 1 小时；另外前一天晚上 8 点也会提醒": "1 hour before if not set; there is also a reminder at 8 pm the evening before",
	"15 分钟": "15 minutes before",
	"30 分钟": "30 minutes before",
	"1 小时":  "1 hour before",
	"2 小时":  "2 hours before",
	"1 天":   "1 day before",
	"不提醒":   "No reminder",
	"注意事项":  "Preparation",
	"如：空腹 8 小时、带上次的片子": "e.g. Fast for 8 hours, bring the last scans",
}

// EnglishNouns names one record of each entity in English, for "Add a …",
// "Delete this …" and the like.
var EnglishNouns = map[string]string{
	"conditions":   "health issue",
	"symptoms":     "symptom",
	"visits":       "visit",
	"medications":  "medication",
	"doses":        "dose",
	"reports":      "report",
	"lab_results":  "lab result",
	"metrics":      "body metric",
	"metric_kinds": "custom metric",
	"habits":       "check-in item",
	"checkins":     "check-in",
	"appointments": "appointment",
}
