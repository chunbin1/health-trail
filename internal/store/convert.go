package store

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// convert turns an input value (from JSON: string, float64, bool, nil, or
// json.Number) into the value stored for f. Blank input becomes nil.
func convert(f Field, v any) (any, error) {
	if n, ok := v.(json.Number); ok {
		v = string(n)
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		v = s
	}
	if v == nil {
		return nil, nil
	}
	switch f.Type {
	case Text, LongText:
		switch x := v.(type) {
		case string:
			return x, nil
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64), nil
		case int, int64, bool:
			return fmt.Sprint(x), nil
		}
	case Int, Ref:
		switch x := v.(type) {
		case float64:
			if x != math.Trunc(x) {
				return nil, fmt.Errorf("需要整数，收到 %v", x)
			}
			return int64(x), nil
		case int:
			return int64(x), nil
		case int64:
			return x, nil
		case string:
			n, err := strconv.ParseInt(strings.TrimPrefix(x, "#"), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("需要整数，收到 %q", x)
			}
			return n, nil
		}
	case Real:
		switch x := v.(type) {
		case float64:
			return x, nil
		case int:
			return float64(x), nil
		case int64:
			return float64(x), nil
		case string:
			n, err := strconv.ParseFloat(strings.ReplaceAll(x, ",", ""), 64)
			if err != nil {
				return nil, fmt.Errorf("需要数字，收到 %q", x)
			}
			return n, nil
		}
	case Bool:
		switch x := v.(type) {
		case bool:
			if x {
				return int64(1), nil
			}
			return int64(0), nil
		case float64:
			if x != 0 {
				return int64(1), nil
			}
			return int64(0), nil
		case int64:
			if x != 0 {
				return int64(1), nil
			}
			return int64(0), nil
		case string:
			switch strings.ToLower(x) {
			case "1", "true", "yes", "是":
				return int64(1), nil
			case "0", "false", "no", "否":
				return int64(0), nil
			}
		}
		return nil, fmt.Errorf("需要 true/false，收到 %v", v)
	case Date:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("需要日期（YYYY-MM-DD），收到 %v", v)
		}
		return normalizeDate(s)
	case DateTime:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("需要时间（YYYY-MM-DD HH:MM），收到 %v", v)
		}
		return normalizeDateTime(s)
	case Enum:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("需要以下之一：%s", strings.Join(f.Options, "、"))
		}
		if f.Name == "flag" {
			if fl := normalizeFlag(s); fl != "" {
				return fl, nil
			}
		}
		for _, o := range f.Options {
			if o == s {
				return s, nil
			}
		}
		return nil, fmt.Errorf("%q 不在可选值中：%s", s, strings.Join(f.Options, "、"))
	}
	return nil, fmt.Errorf("无法识别的值 %v", v)
}

var (
	dateRe = regexp.MustCompile(`^(\d{4})\s*[-/.年]\s*(\d{1,2})\s*[-/.月]\s*(\d{1,2})\s*日?`)
	compactDateRe = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})`)
	timeRe = regexp.MustCompile(`(\d{1,2})\s*[:：时]\s*(\d{1,2})`)
)

// normalizeDate accepts 2026-3-5, 2026/03/05, 2026.3.5, 20260305,
// 2026年3月5日, "today"/"今天" and a date with a time after it.
func normalizeDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "today", "今天":
		return time.Now().Format("2006-01-02"), nil
	case "yesterday", "昨天":
		return time.Now().AddDate(0, 0, -1).Format("2006-01-02"), nil
	}
	m := dateRe.FindStringSubmatch(s)
	if m == nil {
		m = compactDateRe.FindStringSubmatch(s)
	}
	if m == nil {
		return "", fmt.Errorf("无法识别日期 %q，请用 YYYY-MM-DD", s)
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.Local)
	if t.Year() != y || int(t.Month()) != mo || t.Day() != d {
		return "", fmt.Errorf("日期 %q 不存在", s)
	}
	return t.Format("2006-01-02"), nil
}

// normalizeDateTime returns "YYYY-MM-DD HH:MM", or "YYYY-MM-DD" when the
// input has no time (a date alone still sorts before that day's times).
func normalizeDateTime(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "now") || s == "现在" {
		return time.Now().Format("2006-01-02 15:04"), nil
	}
	d, err := normalizeDate(s)
	if err != nil {
		return "", err
	}
	rest := s
	if m := dateRe.FindStringIndex(s); m != nil {
		rest = s[m[1]:]
	} else if m := compactDateRe.FindStringIndex(s); m != nil {
		rest = s[m[1]:]
	}
	rest = strings.TrimLeft(rest, "T ")
	if t := timeRe.FindStringSubmatch(rest); t != nil {
		h, _ := strconv.Atoi(t[1])
		mi, _ := strconv.Atoi(t[2])
		if h > 23 || mi > 59 {
			return "", fmt.Errorf("时间 %q 不存在", s)
		}
		return fmt.Sprintf("%s %02d:%02d", d, h, mi), nil
	}
	return d, nil
}

func normalizeFlag(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "↑", "H", "HIGH", "高", "偏高", "升高", "↑↑":
		return "偏高"
	case "↓", "L", "LOW", "低", "偏低", "降低", "↓↓":
		return "偏低"
	case "N", "NORMAL", "正常", "-", "阴性":
		return "正常"
	case "ABNORMAL", "A", "异常", "阳性", "+":
		return "异常"
	}
	return ""
}

var (
	rangeRe = regexp.MustCompile(`^\s*(-?\d+(?:\.\d+)?)\s*(?:-|~|～|－|–|—|至|到)\s*(-?\d+(?:\.\d+)?)\s*$`)
	ltRe    = regexp.MustCompile(`^\s*(?:<|＜|≤|<=|小于)\s*(\d+(?:\.\d+)?)\s*$`)
	gtRe    = regexp.MustCompile(`^\s*(?:>|＞|≥|>=|大于)\s*(\d+(?:\.\d+)?)\s*$`)
	valueRe = regexp.MustCompile(`^\s*([<>＜＞≤≥]?)\s*(-?\d+(?:\.\d+)?)\s*([↑↓]*|[HL])?\s*$`)
)

// ParseRange reads a reference range: "3.5-9.5", "0~6", "<5", ">60".
// A bound that doesn't exist is nil.
func ParseRange(s string) (low, high *float64, ok bool) {
	if m := rangeRe.FindStringSubmatch(s); m != nil {
		l, _ := strconv.ParseFloat(m[1], 64)
		h, _ := strconv.ParseFloat(m[2], 64)
		return &l, &h, true
	}
	if m := ltRe.FindStringSubmatch(s); m != nil {
		h, _ := strconv.ParseFloat(m[1], 64)
		return nil, &h, true
	}
	if m := gtRe.FindStringSubmatch(s); m != nil {
		l, _ := strconv.ParseFloat(m[1], 64)
		return &l, nil, true
	}
	return nil, nil, false
}

// ParseValue reads a result such as "111.87↑", "14.1 ↓" or "<0.5" into its
// number and the flag the report printed next to it ("" if none).
func ParseValue(s string) (num *float64, flag string, ok bool) {
	m := valueRe.FindStringSubmatch(s)
	if m == nil {
		return nil, "", false
	}
	n, _ := strconv.ParseFloat(m[2], 64)
	if m[3] != "" {
		flag = normalizeFlag(string([]rune(m[3])[0:1]))
	}
	return &n, flag, true
}

// prepareLab derives a lab result's number, bounds and flag from what was
// given: the flag printed on the report wins over the one computed from the
// range, since the lab may use rules the range alone doesn't show.
func prepareLab(r Record) {
	if r["value_num"] == nil {
		if t, ok := r["value_text"].(string); ok {
			if n, fl, ok := ParseValue(t); ok {
				r["value_num"] = *n
				if r["flag"] == nil && fl != "" {
					r["flag"] = fl
				}
				if !strings.ContainsAny(t, "<>＜＞≤≥") {
					r["value_text"] = nil
				}
			}
		}
	}
	if rr, ok := r["ref_range"].(string); ok && r["ref_low"] == nil && r["ref_high"] == nil {
		if l, h, ok := ParseRange(rr); ok {
			if l != nil {
				r["ref_low"] = *l
			}
			if h != nil {
				r["ref_high"] = *h
			}
		}
	}
	if r["flag"] == nil {
		if v, ok := r["value_num"].(float64); ok {
			lo, hasLo := r["ref_low"].(float64)
			hi, hasHi := r["ref_high"].(float64)
			switch {
			case hasHi && v > hi:
				r["flag"] = "偏高"
			case hasLo && v < lo:
				r["flag"] = "偏低"
			case hasLo || hasHi:
				r["flag"] = "正常"
			}
		}
	}
}

// NormalizeDate is normalizeDate for callers outside the package.
func NormalizeDate(s string) (string, error) { return normalizeDate(s) }
