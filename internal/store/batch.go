package store

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

// CheckNew reports whether in would be accepted as a new row. It writes nothing.
func (s *Store) CheckNew(entityName string, in Record) error {
	e, err := entity(entityName)
	if err != nil {
		return err
	}
	_, err = s.build(e, in)
	return err
}

// CaptureRow is one record inside a single transaction. ConditionName and
// MedicationName fill condition_id and medication_id when those are empty:
// a row created earlier in this batch, or one already stored under that name.
// Index is the 1-based position to name in an error (0 means the position
// in the slice).
type CaptureRow struct {
	Entity         string
	Fields         Record
	ConditionName  string
	MedicationName string
	Index          int
}

// CreateInOrder inserts every row, or none of them. Conditions are written
// before other rows, then medications, so a later row can name one of them.
func (s *Store) CreateInOrder(items []CaptureRow) ([]Record, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if len(items) > 30 {
		return nil, invalid("一次最多保存 30 条", "At most 30 can be saved at once")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	known := map[string]map[int64]struct{}{}
	condNames := map[string]int64{}
	medNames := map[string]int64{}
	placed := make([]struct {
		entity string
		id     int64
	}, len(items))

	order := make([]int, 0, len(items))
	// metric kinds before their first reading, so the reading can name a
	// kind created in this same batch
	for _, want := range []string{"conditions", "medications", "metric_kinds"} {
		for i, it := range items {
			if it.Entity == want {
				order = append(order, i)
			}
		}
	}
	for i, it := range items {
		if it.Entity != "conditions" && it.Entity != "medications" && it.Entity != "metric_kinds" {
			order = append(order, i)
		}
	}
	kindNames := []string{}
	// the unit of a metric kind made in this batch: prepare looks units up
	// outside the transaction, where that kind isn't visible yet
	kindUnits := map[string]any{}

	for _, i := range order {
		it := items[i]
		n := it.Index
		if n == 0 {
			n = i + 1
		}
		e, err := entity(it.Entity)
		if err != nil {
			return nil, prefixErr(n, err)
		}
		fields := cloneRecord(it.Fields)
		if it.ConditionName != "" && fields["condition_id"] == nil {
			id, ok, err := resolveName(tx, "conditions", it.ConditionName, condNames)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, invalid("第 %d 条：没有叫「%s」的健康问题", "Item %d: there's no health issue called %q", n, it.ConditionName)
			}
			fields["condition_id"] = id
		}
		if it.MedicationName != "" && fields["medication_id"] == nil {
			id, ok, err := resolveName(tx, "medications", it.MedicationName, medNames)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, invalid("第 %d 条：没有叫「%s」的药物", "Item %d: there's no medication called %q", n, it.MedicationName)
			}
			fields["medication_id"] = id
		}
		r, err := s.buildWithNames(e, fields, known, kindNames)
		if err != nil {
			return nil, prefixErr(n, err)
		}
		if t, ok := r["metric_type"].(string); ok && r["unit"] == nil && (e.Name == "metrics" || e.Name == "habits") {
			if u, ok := kindUnits[t]; ok {
				r["unit"] = u
			}
		}
		if err := editable(e, r); err != nil {
			return nil, prefixErr(n, err)
		}
		id, err := insert(tx, e, r)
		if err != nil {
			return nil, prefixErr(n, err)
		}
		if known[e.Name] == nil {
			known[e.Name] = map[int64]struct{}{}
		}
		known[e.Name][id] = struct{}{}
		if e.Name == "metric_kinds" {
			if name, _ := r["name"].(string); name != "" {
				kindNames = append(kindNames, name)
				if r["unit"] != nil {
					kindUnits[name] = r["unit"]
				}
			}
		}
		if e.Name == "conditions" {
			if name, _ := r["name"].(string); name != "" {
				if _, seen := condNames[name]; !seen {
					condNames[name] = id
				}
			}
		}
		if e.Name == "medications" {
			if name, _ := r["name"].(string); name != "" {
				if _, seen := medNames[name]; !seen {
					medNames[name] = id
				}
			}
		}
		placed[i] = struct {
			entity string
			id     int64
		}{e.Name, id}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	out := make([]Record, len(placed))
	for i, p := range placed {
		if out[i], err = s.Get(p.entity, p.id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func pos(n int) string { return "第 " + strconv.Itoa(n) + " 条：" }

func cloneRecord(in Record) Record {
	out := make(Record, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// resolveName finds a row by its exact name: one created earlier in this
// transaction (names), then one already stored.
func resolveName(tx *sql.Tx, table, name string, names map[string]int64) (int64, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, false, nil
	}
	if id, ok := names[name]; ok {
		return id, true, nil
	}
	q := `SELECT id FROM ` + table + ` WHERE name = ? ORDER BY id LIMIT 1`
	if table == "conditions" {
		q = `SELECT id FROM conditions WHERE name = ? ORDER BY CASE status WHEN '进行中' THEN 0 WHEN '慢性' THEN 1 WHEN '观察中' THEN 2 ELSE 3 END, id LIMIT 1`
	}
	if table == "medications" {
		q = `SELECT id FROM medications WHERE name = ? ORDER BY CASE status WHEN '在用' THEN 0 WHEN '按需' THEN 1 ELSE 2 END, id LIMIT 1`
	}
	var id int64
	err := tx.QueryRow(q, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}
