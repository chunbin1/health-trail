package assistant

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lee/health-trail/internal/mcpserver"
	"github.com/lee/health-trail/internal/msg"
	"github.com/lee/health-trail/internal/store"
)

// The assistant uses the tools Claude gets over MCP, from the same server in
// this process. Every read-only tool is offered and runs as it is, so a tool
// added to mcpserver reaches the assistant too. Of the write tools only these
// three are offered, and they are not run: each call becomes a card that the
// person saves (or automatic saving writes). update_record becomes a card
// too, one that is never saved automatically. Deleting a record, undoing a
// check-in and attaching a file by path stay in the window.
var draftTools = map[string]bool{"add_record": true, "add_lab_report": true, "checkin": true, "update_record": true}

// a tool result is cut here before it goes to the model
const maxToolResult = 24 << 10

type toolbox struct {
	session      *mcp.ClientSession
	server       *mcp.ServerSession
	defs         []any
	read         map[string]bool
	instructions string
}

func openTools(ctx context.Context, st *store.Store) (*toolbox, error) {
	srv := mcpserver.New(st, "app", false)
	clientEnd, serverEnd := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverEnd, nil)
	if err != nil {
		return nil, msg.New("没能打开档案工具", "Couldn't open the record tools")
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "health-trail-assistant", Version: "app"}, nil)
	cs, err := client.Connect(ctx, clientEnd, nil)
	if err != nil {
		ss.Close()
		return nil, msg.New("没能打开档案工具", "Couldn't open the record tools")
	}
	tb := &toolbox{session: cs, server: ss, read: map[string]bool{}}
	if r := cs.InitializeResult(); r != nil {
		tb.instructions = r.Instructions
	}
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		tb.close()
		return nil, msg.New("没能打开档案工具", "Couldn't open the record tools")
	}
	tools := res.Tools
	// the same order every time, so the request prefix can be cached
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	for _, t := range tools {
		readOnly := t.Annotations != nil && t.Annotations.ReadOnlyHint
		if !readOnly && !draftTools[t.Name] {
			continue
		}
		if readOnly && !draftTools[t.Name] {
			tb.read[t.Name] = true
		}
		tb.defs = append(tb.defs, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": t.Name, "description": shortDescription(t.Name, t.Description), "parameters": t.InputSchema,
			},
		})
	}
	return tb, nil
}

// fieldTable starts the list of every record type and its fields that MCP
// puts in a tool description.
const fieldTable = mcpserver.FieldTable

// shortDescription keeps the field table in add_record only. Every request
// carries every tool description, and the table is the longest part of
// them; list_records had a second copy.
func shortDescription(name, desc string) string {
	if name == "add_record" {
		return desc
	}
	if i := strings.Index(desc, fieldTable); i >= 0 {
		return strings.TrimSpace(desc[:i]) + "\n\n记录类型和字段名见 add_record 的说明。"
	}
	return desc
}

func (tb *toolbox) close() {
	tb.session.Close()
	tb.server.Close()
}

// run calls a read-only tool. The text goes back to the model; an error is
// told to it as a result, so it can try again.
func (tb *toolbox) run(ctx context.Context, name string, args map[string]any) (string, bool) {
	if !tb.read[name] {
		return name + " 不是只读的工具，这里不能直接执行", true
	}
	res, err := tb.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err.Error(), true
	}
	var b strings.Builder
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	return cut(b.String()), res.IsError
}

func cut(s string) string {
	if len(s) <= maxToolResult {
		return s
	}
	s = s[:maxToolResult]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "\n…（结果太长，后面的省略了。缩小范围再查，比如加上日期或 limit。）"
}

// Step is one tool call, shown under the reply so the person can see what
// was looked at. Type, ID and Text are what the call was about.
type Step struct {
	Tool  string `json:"tool"`
	Type  string `json:"type,omitempty"`
	ID    int64  `json:"id,omitempty"`
	Text  string `json:"text,omitempty"`
	Error bool   `json:"error,omitempty"`
}

func stepOf(name string, args map[string]any) Step {
	s := Step{Tool: name}
	switch name {
	case "search_records":
		s.Text = text(args["query"])
	case "list_records", "add_record":
		s.Type = text(args["type"])
	case "get_record", "update_record":
		s.Type = text(args["type"])
		s.ID = idOf(store.Record{"id": args["id"]})
	case "get_lab_trend":
		s.Text = text(args["item_name"])
	case "get_metric_trend":
		s.Text = text(args["metric_type"])
	case "checkin", "get_checkin_stats":
		s.Text = text(args["habit"])
	case "add_lab_report":
		if r, ok := args["report"].(map[string]any); ok {
			s.Text = text(r["title"])
		}
	}
	// what a model's markup left in a value is not shown
	if !typeRe.MatchString(s.Type) {
		s.Type = ""
	}
	s.Text = clip(strings.Join(strings.Fields(tagRe.ReplaceAllString(s.Text, " ")), " "), 30)
	return s
}

var (
	typeRe = regexp.MustCompile(`^[a-z_]*$`)
	tagRe  = regexp.MustCompile(`<[^>]*>`)
)
