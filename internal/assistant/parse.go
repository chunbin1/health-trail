package assistant

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/lee/health-trail/internal/store"
)

func parseDraft(content string) (Draft, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(strings.TrimSpace(content), "```")
	content = strings.TrimSpace(content)
	if i := strings.Index(content, "{"); i >= 0 {
		if j := strings.LastIndex(content, "}"); j > i {
			content = content[i : j+1]
		}
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return Draft{}, fmt.Errorf("整理结果不是 JSON")
	}
	itemsRaw, ok := raw["items"]
	if !ok {
		itemsRaw = raw["records"]
	}
	d := Draft{}
	if s, _ := raw["reply"].(string); s != "" {
		d.Reply = strings.TrimSpace(s)
	}
	if s, _ := raw["unclear"].(string); s != "" {
		d.Unclear = strings.TrimSpace(s)
	}
	list, ok := itemsRaw.([]any)
	if itemsRaw != nil && !ok {
		return Draft{}, fmt.Errorf("整理结果不是 JSON")
	}
	for _, one := range list {
		m, ok := one.(map[string]any)
		if !ok {
			return Draft{}, fmt.Errorf("整理结果里有一条无法识别")
		}
		d.Items = append(d.Items, itemFromMap(m))
	}
	if d.Items == nil {
		d.Items = []Item{}
	}
	return d, nil
}

func itemFromMap(m map[string]any) Item {
	it := Item{Kind: strings.TrimSpace(textOf(m["kind"]))}
	it.Type = strings.TrimSpace(textOf(m["type"]))
	it.Habit = strings.TrimSpace(textOf(m["habit"]))
	it.Date = strings.TrimSpace(textOf(m["date"]))
	it.Notes = strings.TrimSpace(textOf(m["notes"]))
	it.Panel = strings.TrimSpace(textOf(m["panel"]))
	it.Summary = strings.TrimSpace(textOf(m["summary"]))
	it.ConditionName = strings.TrimSpace(textOf(m["condition_name"]))
	it.MedicationName = strings.TrimSpace(textOf(m["medication_name"]))
	if it.Value = m["value"]; it.Value == nil {
		it.Value = m["amount"]
	}
	if f, ok := m["fields"].(map[string]any); ok {
		it.Fields = f
	}
	if r, ok := m["report"].(map[string]any); ok {
		it.Report = r
	}
	if rows, ok := m["results"].([]any); ok {
		for _, row := range rows {
			rm, ok := row.(map[string]any)
			if !ok {
				continue
			}
			it.Results = append(it.Results, LabResult{
				ItemName: textOf(rm["item_name"]),
				Value:    rm["value"],
				Unit:     textOf(rm["unit"]),
				RefRange: textOf(rm["ref_range"]),
				Flag:     textOf(rm["flag"]),
				Panel:    textOf(rm["panel"]),
				TestDate: textOf(rm["test_date"]),
				Notes:    textOf(rm["notes"]),
			})
		}
	}
	return normalizeRaw(it, m)
}

// kindAlias is what models write instead of the stored type name.
var kindAlias = map[string]string{
	"appointment": "appointments", "appointments": "appointments", "预约": "appointments",
	"condition": "conditions", "conditions": "conditions", "健康问题": "conditions",
	"symptom": "symptoms", "symptoms": "symptoms", "症状": "symptoms",
	"visit": "visits", "visits": "visits", "就诊": "visits",
	"medication": "medications", "medications": "medications", "用药": "medications",
	"dose": "doses", "doses": "doses", "服药": "doses",
	"report": "reports", "reports": "reports", "检查报告": "reports",
	"metric": "metrics", "metrics": "metrics", "身体数据": "metrics",
	"habit": "habits", "habits": "habits",
	"lab": "lab_report", "labs": "lab_report", "lab_report": "lab_report", "化验单": "lab_report",
	"checkin": "checkin", "checkins": "checkin", "打卡": "checkin",
}

func aliasOf(kind string) (string, bool) {
	s, ok := kindAlias[strings.ToLower(strings.TrimSpace(kind))]
	if ok {
		return s, true
	}
	s, ok = kindAlias[strings.TrimSpace(kind)]
	return s, ok
}

func normalize(it Item) Item { return normalizeRaw(it, nil) }

func normalizeRaw(it Item, raw map[string]any) Item {
	if raw == nil {
		raw = map[string]any{}
	}
	switch it.Kind {
	case "", "record":
		if it.Type != "" {
			it.Kind = "record"
		}
	case "lab", "labs", "lab_report":
		it.Kind = "lab_report"
	case "checkin", "checkins":
		it.Kind = "checkin"
	}
	if typ, ok := aliasOf(it.Kind); ok && it.Kind != "record" {
		switch typ {
		case "checkin", "lab_report":
			it.Kind = typ
		default:
			if it.Type == "" {
				it.Type = typ
			}
			it.Kind = "record"
		}
	}
	if typ, ok := aliasOf(it.Type); ok && typ != "checkin" && typ != "lab_report" {
		it.Type = typ
		it.Kind = "record"
	}
	if it.Fields == nil {
		it.Fields = map[string]any{}
	}
	// models often put the columns next to kind, not inside fields
	if it.Kind == "record" {
		if e := store.EntityByName(it.Type); e != nil {
			for _, f := range e.Fields {
				if _, ok := it.Fields[f.Name]; ok {
					continue
				}
				if v, ok := raw[f.Name]; ok && v != nil && strings.TrimSpace(textOf(v)) != "" {
					it.Fields[f.Name] = v
				}
			}
		}
	}
	if it.ConditionName == "" {
		it.ConditionName = strings.TrimSpace(textOf(it.Fields["condition_name"]))
	}
	delete(it.Fields, "condition_name")
	if it.MedicationName == "" {
		it.MedicationName = strings.TrimSpace(textOf(it.Fields["medication_name"]))
	}
	delete(it.Fields, "medication_name")
	if name, ok := looseName(it.Fields["condition_id"]); ok {
		if it.ConditionName == "" {
			it.ConditionName = name
		}
		delete(it.Fields, "condition_id")
	}
	if name, ok := looseName(it.Fields["medication_id"]); ok {
		if it.MedicationName == "" {
			it.MedicationName = name
		}
		delete(it.Fields, "medication_id")
	}
	return it
}

// looseName is a reference written as a name rather than an id.
func looseName(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(strings.TrimPrefix(s, "#"))
	if s == "" {
		return "", false
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return "", false
	}
	return s, true
}

func textOf(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
}

func labRecord(row LabResult, panel, reportDate string) store.Record {
	rec := store.Record{}
	if row.ItemName != "" {
		rec["item_name"] = row.ItemName
	}
	if row.Unit != "" {
		rec["unit"] = row.Unit
	}
	if row.RefRange != "" {
		rec["ref_range"] = row.RefRange
	}
	if row.Flag != "" {
		rec["flag"] = row.Flag
	}
	if row.Panel != "" {
		rec["panel"] = row.Panel
	} else if panel != "" {
		rec["panel"] = panel
	}
	if row.TestDate != "" {
		rec["test_date"] = row.TestDate
	} else if reportDate != "" {
		rec["test_date"] = reportDate
	}
	if row.Notes != "" {
		rec["notes"] = row.Notes
	}
	switch v := row.Value.(type) {
	case float64:
		rec["value_num"] = v
	case string:
		if s := strings.TrimSpace(v); s != "" {
			rec["value_text"] = s
		}
	case json.Number:
		if s := strings.TrimSpace(v.String()); s != "" {
			rec["value_text"] = s
		}
	}
	return rec
}
