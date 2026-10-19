package store

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// LabRow is one line of a lab report, as parsed from pasted text.
type LabRow struct {
	Name      string   `json:"item_name"`
	Value     *float64 `json:"value_num"`
	ValueText string   `json:"value_text,omitempty"`
	Flag      string   `json:"flag,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	RefRange  string   `json:"ref_range,omitempty"`
}

var (
	headerWords = map[string]bool{
		"检查项目": true, "项目": true, "项目名称": true, "检验项目": true, "结果": true, "检验结果": true,
		"单位": true, "参考值": true, "参考范围": true, "参考区间": true, "提示": true, "状态": true,
	}
	textResults = map[string]bool{
		"阴性": true, "阳性": true, "弱阳性": true, "未见异常": true, "正常": true, "(-)": true, "(+)": true,
		"阴性(-)": true, "阳性(+)": true, "-": true, "+": true,
	}
	unitRe  = regexp.MustCompile(`^[A-Za-zμµ%^/.*×·0-9()（）\-]+$`)
	cnUnits = map[string]bool{"秒": true, "倍": true, "次": true, "个": true, "比值": true}
	arrowRe = regexp.MustCompile(`^[↑↓]+$|^[HL]$`)
	splitRe = regexp.MustCompile(`[\t\n\r ]+|　`)
	numRe   = regexp.MustCompile(`^-?\d+(?:\.\d+)?$`)
	dashRe  = regexp.MustCompile(`^(?:-|~|～|－|–|—|至|到)$`)
)

func isUnit(t string) bool {
	if cnUnits[t] {
		return true
	}
	if utf8.RuneCountInString(t) > 14 || !unitRe.MatchString(t) {
		return false
	}
	// a bare number is a value, not a unit
	return !numRe.MatchString(t)
}

// ParseLabText reads lab results pasted as text. It copes with table rows
// (cells split by tabs or spaces) and with one cell per line, where a long
// item name may itself be broken over two lines — the way a report copied
// from a phone screen usually comes out. Each row is: name, result (an
// arrow may follow, as its own cell or not), unit (optional), range
// (optional).
func ParseLabText(text string) []LabRow {
	var raw []string
	for _, t := range splitRe.Split(text, -1) {
		if t = strings.TrimSpace(t); t != "" {
			raw = append(raw, t)
		}
	}
	// "3.5 - 9.5" → "3.5-9.5"
	var toks []string
	for i := 0; i < len(raw); i++ {
		if i+2 < len(raw) && numRe.MatchString(raw[i]) && dashRe.MatchString(raw[i+1]) && numRe.MatchString(raw[i+2]) {
			toks = append(toks, raw[i]+"-"+raw[i+2])
			i += 2
			continue
		}
		toks = append(toks, raw[i])
	}

	var rows []LabRow
	var name []string
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if headerWords[t] && len(name) == 0 {
			continue
		}
		num, flag, isNum := ParseValue(t)
		_, _, isRange := ParseRange(t)
		isText := textResults[t]
		if len(name) == 0 || (!isNum && !isText) || (isRange && !isNum) {
			name = append(name, t)
			continue
		}
		row := LabRow{Name: strings.Join(name, ""), Flag: flag}
		name = nil
		if isNum {
			row.Value = num
			if strings.ContainsAny(t, "<>＜＞≤≥") {
				row.ValueText = strings.TrimRight(t, "↑↓HL ")
			}
		} else {
			row.ValueText = t
		}
		if i+1 < len(toks) && arrowRe.MatchString(toks[i+1]) {
			row.Flag = normalizeFlag(string([]rune(toks[i+1])[0:1]))
			i++
		}
		if i+1 < len(toks) && isUnit(toks[i+1]) {
			if _, _, r := ParseRange(toks[i+1]); !r {
				row.Unit = toks[i+1]
				i++
			}
		}
		if i+1 < len(toks) {
			if _, _, r := ParseRange(toks[i+1]); r || textResults[toks[i+1]] {
				row.RefRange = toks[i+1]
				i++
			}
		}
		rows = append(rows, row)
	}
	return rows
}
