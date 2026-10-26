package store

import (
	"fmt"
	"math"
	"strings"
)

// BMI isn't recorded: it is worked out from each weight and the height of
// the time (the latest one measured by then, or else the first one), so it
// follows the scale without being typed in.
const BMI = "BMI"

// DerivedMetrics are the metrics worked out from others, read only.
var DerivedMetrics = []string{BMI}

// bmiCategory is the band a BMI is in, by the Chinese standard for adults
// (WS/T 428—2013).
func bmiCategory(v float64) string {
	switch {
	case v < 18.5:
		return "偏瘦"
	case v < 24:
		return "正常"
	case v < 28:
		return "超重"
	}
	return "肥胖"
}

func unitWord(r Record) string {
	u, _ := r["unit"].(string)
	return strings.ToLower(strings.TrimSpace(u))
}

// kilograms is a weight reading in kg; ok is false for a unit it can't be
// turned from.
func kilograms(r Record) (float64, bool) {
	v, ok := r["value"].(float64)
	if !ok {
		return 0, false
	}
	switch unitWord(r) {
	case "", "kg", "公斤", "千克":
		return v, true
	case "斤":
		return v / 2, true
	case "lb", "lbs", "磅":
		return v * 0.45359237, true
	}
	return 0, false
}

// metres is a height reading in m. 1.79 written in cm is taken as metres:
// no adult is 1.79 cm tall.
func metres(r Record) (float64, bool) {
	v, ok := r["value"].(float64)
	if !ok {
		return 0, false
	}
	switch unitWord(r) {
	case "", "cm", "厘米":
		if v >= 3 {
			v /= 100
		}
	case "m", "米":
	default:
		return 0, false
	}
	return v, v >= 0.5 && v <= 2.5
}

// bmiOf is the BMI of one weight reading with heights (oldest first), or
// nil when there is no height or a unit it can't read.
func bmiOf(w Record, heights []Record) Record {
	kg, ok := kilograms(w)
	if !ok || len(heights) == 0 {
		return nil
	}
	h := heights[0]
	at := fmt.Sprint(w["measured_at"])
	for _, r := range heights {
		if fmt.Sprint(r["measured_at"]) <= at {
			h = r
		}
	}
	m, ok := metres(h)
	if !ok {
		return nil
	}
	v := math.Round(kg/(m*m)*10) / 10
	// the weight's source and id: opening a BMI opens the weight it's from
	return Record{
		"source": w["source"], "id": w["id"], "metric_type": BMI, "measured_at": w["measured_at"],
		"value": v, "value2": nil, "unit": "kg/m²", "category": bmiCategory(v),
		"weight_kg": math.Round(kg*10) / 10, "height_cm": math.Round(m * 100), "context": w["context"], "report_title": w["report_title"],
	}
}

func (s *Store) heights() ([]Record, error) {
	return s.MetricSeries("身高", "", "")
}

// bmiSeries is MetricSeries for BMI: one for each weight in [from, to].
func (s *Store) bmiSeries(from, to string) ([]Record, error) {
	hs, err := s.heights()
	if err != nil || len(hs) == 0 {
		return []Record{}, err
	}
	ws, err := s.MetricSeries("体重", from, to)
	if err != nil {
		return nil, err
	}
	out := []Record{}
	for _, w := range ws {
		if b := bmiOf(w, hs); b != nil {
			out = append(out, b)
		}
	}
	return out, nil
}
