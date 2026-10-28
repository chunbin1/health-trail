package assistant

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/lee/health-trail/internal/msg"
)

// Some models (GLM) write tool arguments in their own markup,
// <arg_key>k</arg_key><arg_value>v</arg_value>, and the service does not
// always turn all of it into JSON. Seen from glm-4.6v: the first argument
// is JSON and the rest is left inside its value,
//
//	{"type": "habits\n<arg_key>fields</arg_key>\n<arg_value>{\"name\": \"喝水\"}"}
//
// argsOf reads that back into {"type": "habits", "fields": {...}}. The same
// markup on its own, or after the tool's name, is read too.
const argKey = "<arg_key>"

var markupRe = regexp.MustCompile(`</?(arg_key|arg_value|tool_call)>`)

// argsOf reads a tool call's arguments. Some services send "" for none.
func argsOf(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err == nil && m != nil {
		return unleak(m), nil
	}
	if i := strings.Index(raw, argKey); i >= 0 {
		if pairs := markupPairs(raw[i:]); len(pairs) > 0 {
			return pairs, nil
		}
	}
	return nil, errBadArgs
}

// errBadArgs goes back to the model; badArgs is what the person is told.
var errBadArgs = msg.New(`参数不是 JSON 对象，请重新调用：arguments 写成一个 JSON 对象，比如 {"type": "habits", "fields": {"name": "喝水"}}，不要用 <arg_key> 这类标签`,
	"The arguments aren't a JSON object; call it again with one JSON object, without <arg_key> tags")

// callOf splits markup that ended up in the tool's name, as in
// "add_record<arg_key>type</arg_key>…", and reads the arguments.
func callOf(name, raw string) (string, map[string]any, error) {
	extra := map[string]any{}
	if i := strings.Index(name, argKey); i >= 0 {
		extra = markupPairs(name[i:])
		name = name[:i]
	}
	name = strings.TrimSpace(name)
	args, err := argsOf(raw)
	if err != nil {
		if len(extra) == 0 {
			return name, nil, err
		}
		args = map[string]any{}
	}
	for k, v := range extra {
		if _, ok := args[k]; !ok {
			args[k] = v
		}
	}
	return name, args, nil
}

// unleak takes markup out of the values of JSON arguments: what comes
// before it is that argument's value, and each pair in it is one more
// argument (unless the JSON already has it).
func unleak(m map[string]any) map[string]any {
	more := map[string]any{}
	for k, v := range m {
		s, ok := v.(string)
		if !ok {
			continue
		}
		i := strings.Index(s, argKey)
		if i < 0 {
			continue
		}
		m[k] = valueOf(s[:i])
		for pk, pv := range markupPairs(s[i:]) {
			more[pk] = pv
		}
	}
	for k, v := range more {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return m
}

// markupPairs reads "<arg_key>k</arg_key><arg_value>v</arg_value>…". A value
// may be missing its closing tag; it then runs to the next key.
func markupPairs(s string) map[string]any {
	out := map[string]any{}
	parts := strings.Split(s, argKey)
	for _, p := range parts[1:] {
		k, v, ok := strings.Cut(p, "</arg_key>")
		k = strings.TrimSpace(k)
		if !ok || k == "" || strings.ContainsAny(k, "<>\"{}") {
			continue
		}
		out[k] = valueOf(v)
	}
	return out
}

// valueOf reads one argument's value: JSON if it is JSON (an object, a
// number), otherwise the text.
func valueOf(s string) any {
	s = strings.TrimSpace(markupRe.ReplaceAllString(s, ""))
	if s == "" {
		return ""
	}
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		return v
	}
	return s
}
