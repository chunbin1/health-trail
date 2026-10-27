package store

import (
	"strings"
	"testing"
)

func TestCreateInOrderMetricKindAndReading(t *testing.T) {
	s := open(t)
	rows, err := s.CreateInOrder([]CaptureRow{
		{Entity: "metrics", Fields: Record{"measured_at": "2026-10-27 08:00", "metric_type": "尿酸", "value": 420}, Index: 1},
		{Entity: "metric_kinds", Fields: Record{"name": "尿酸", "unit": "μmol/L"}, Index: 2},
	})
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %+v", err, rows)
	}
	if rows[0]["metric_type"] != "尿酸" || rows[0]["unit"] != "μmol/L" {
		t.Fatalf("%+v", rows[0])
	}
}

func TestCreateInOrderRollsBack(t *testing.T) {
	s := open(t)
	_, err := s.CreateInOrder([]CaptureRow{
		{Entity: "conditions", Fields: Record{"name": "胃炎"}, Index: 1},
		{Entity: "visits", Fields: Record{"visit_date": "2026-10-27", "visit_type": "很高"}, Index: 2},
	})
	if err == nil || !strings.Contains(err.Error(), "第 2 条") {
		t.Fatalf("err = %v", err)
	}
	if _, n, _ := s.List("conditions", ListQuery{Limit: 10}); n != 0 {
		t.Fatalf("condition was kept, n=%d", n)
	}
	if _, n, _ := s.List("visits", ListQuery{Limit: 10}); n != 0 {
		t.Fatalf("visit was kept, n=%d", n)
	}
}

func TestCreateInOrderLinksNames(t *testing.T) {
	s := open(t)
	old, err := s.Create("conditions", Record{"name": "鼻炎", "status": "进行中"})
	if err != nil {
		t.Fatal(err)
	}
	med, err := s.Create("medications", Record{"name": "氯雷他定", "status": "在用"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.CreateInOrder([]CaptureRow{
		{Entity: "symptoms", Fields: Record{"occurred_at": "2026-10-27 08:00", "description": "打喷嚏"}, ConditionName: "鼻炎", Index: 1},
		{Entity: "conditions", Fields: Record{"name": "胃炎"}, Index: 2},
		{Entity: "symptoms", Fields: Record{"occurred_at": "2026-10-27 09:00", "description": "上腹隐痛"}, ConditionName: "胃炎", Index: 3},
		{Entity: "doses", Fields: Record{"taken_at": "2026-10-27 09:30"}, MedicationName: "氯雷他定", Index: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["condition_id"] != old["id"] {
		t.Fatalf("existing condition: %v", rows[0]["condition_id"])
	}
	if rows[2]["condition_id"] != rows[1]["id"] {
		t.Fatalf("new condition: symptom %v condition %v", rows[2]["condition_id"], rows[1]["id"])
	}
	if rows[3]["medication_id"] != med["id"] {
		t.Fatalf("medication: %v", rows[3]["medication_id"])
	}
}

func TestCreateInOrderUnknownNameWritesNothing(t *testing.T) {
	s := open(t)
	_, err := s.CreateInOrder([]CaptureRow{
		{Entity: "conditions", Fields: Record{"name": "胃炎"}, Index: 1},
		{Entity: "symptoms", Fields: Record{"occurred_at": "2026-10-27 09:00", "description": "上腹隐痛"}, ConditionName: "没有这种病", Index: 2},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, n, _ := s.List("conditions", ListQuery{Limit: 10}); n != 0 {
		t.Fatalf("condition was kept, n=%d", n)
	}
}
