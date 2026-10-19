package mcpserver

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Prompts are analysis templates a person picks in Claude (the "+" menu in
// Claude Desktop). Each one tells Claude which tools to read first and what
// to produce, so a good analysis doesn't depend on how the question was
// worded. They only read; none of them writes records.

type prompt struct {
	name, title, desc string
	args              []*mcp.PromptArgument
	text              func(args map[string]string) string
}

const careful = `

注意：只依据工具返回的数据，不要猜测缺失的数值；说明哪些结论需要医生确认；如果有需要尽快就医的信号（如指标严重异常、持续高热），放在最前面明确提醒。用中文回答，先结论后细节。`

var prompts = []prompt{
	{
		name: "review_latest_labs", title: "解读最近的化验单",
		desc: "读取最近一份（或指定的）检查报告，逐项解读异常指标，并和历史结果对比。",
		args: []*mcp.PromptArgument{{Name: "report", Title: "报告", Description: "报告标题或日期，不填则用最近一份"}},
		text: func(a map[string]string) string {
			which := "最近一份检查报告（list_records type=reports limit=1）"
			if a["report"] != "" {
				which = "「" + a["report"] + "」这份检查报告（用 search_records 或 list_records 找到它）"
			}
			return "请帮我解读" + which + `。
步骤：
1. 用 get_record 读取报告和全部指标；
2. 对每个异常指标，用 get_lab_trend 查看历史结果，说明是新出现、持续还是在好转；
3. 用 health_overview 了解我当前的健康问题和用药，看异常是否和它们有关；
4. 输出：一句话总结 → 异常指标逐项说明（意义、可能原因、与历史对比）→ 建议复查的项目和时间 → 可以问医生的问题。` + careful
		},
	},
	{
		name: "visit_summary", title: "整理就诊摘要",
		desc: "为看医生准备一份简明病史：现有问题、用药、近期症状、关键检查结果。",
		args: []*mcp.PromptArgument{{Name: "topic", Title: "就诊目的", Description: "如：胃痛复诊、年度体检后咨询；不填则全面整理"}},
		text: func(a map[string]string) string {
			focus := "全面情况"
			if a["topic"] != "" {
				focus = "「" + a["topic"] + "」"
			}
			return "我要去看医生，请围绕" + focus + `帮我整理一份可以直接给医生看的病史摘要。
步骤：用 health_overview 了解全貌；用 search_records、get_record 查相关的健康问题、就诊、用药、症状记录；用 get_lab_trend 查相关指标的历史。
输出格式（尽量一页以内）：
- 主要问题与起病时间
- 既往诊疗经过（时间、医院、诊断、处理）
- 当前用药（药名、剂量、开始时间、效果和不良反应）
- 近期症状变化
- 关键检查结果（列出数值、参考范围和日期）
- 过敏史与慢性病
- 我想问医生的问题（3-5 个）` + careful
		},
	},
	{
		name: "symptom_analysis", title: "分析症状",
		desc: "梳理某个症状的发作记录，找规律和诱因，判断是否需要就医。",
		args: []*mcp.PromptArgument{{Name: "symptom", Title: "症状", Description: "如：头痛、胃痛", Required: true}},
		text: func(a map[string]string) string {
			return "请帮我分析「" + a["symptom"] + `」这个症状。
步骤：用 search_records 找出所有相关的症状记录、健康问题、就诊和用药；必要时用 list_records type=symptoms 看更长时间的记录；用 get_checkin_stats 看作息和运动打卡，用 get_metric_trend 看相关的身体数据。
输出：发作时间线 → 频率和严重程度的变化 → 可能的诱因和规律（说明依据）→ 和已有健康问题、用药的关联 → 是否需要就医、挂什么科 → 接下来建议记录哪些信息。` + careful
		},
	},
	{
		name: "health_checkup", title: "健康状况总评",
		desc: "综合所有记录做一次整体评估：风险、趋势和需要关注的事项。",
		text: func(map[string]string) string {
			return `请对我的整体健康状况做一次总评。
步骤：health_overview → list_lab_items（关注异常和多次检测的项目，用 get_lab_trend 看趋势）→ get_metric_trend 看体重、血压等近一年的变化 → get_checkin_stats 看最近 30 天习惯 → list_records type=conditions 看全部健康问题。
输出：总体评价 → 需要关注的风险（按优先级）→ 在变好的方面 → 在变差的方面 → 建议的检查与复查计划 → 生活习惯建议。` + careful
		},
	},
	{
		name: "habit_review", title: "打卡复盘",
		desc: "回顾一段时间的打卡完成情况，结合身体数据给出调整建议。",
		args: []*mcp.PromptArgument{{Name: "days", Title: "天数", Description: "回顾最近多少天，默认 30"}},
		text: func(a map[string]string) string {
			days := strings.TrimSpace(a["days"])
			if days == "" {
				days = "30"
			}
			return "请复盘我最近 " + days + ` 天的打卡情况。
步骤：用 get_checkin_stats（from 设为 ` + days + ` 天前）查看各打卡项的完成率和连续天数；用 get_metric_trend 查看同期的体重、睡眠等数据；用 health_overview 看当前的健康问题。
输出：完成情况一览（表格）→ 做得好的 → 容易中断的及可能原因 → 打卡和身体数据之间的关联（只说数据支持的）→ 下一阶段的具体建议（目标要现实）。`
		},
	},
}

func addPrompts(s *mcp.Server) {
	for _, p := range prompts {
		p := p
		s.AddPrompt(&mcp.Prompt{Name: p.name, Title: p.title, Description: p.desc, Arguments: p.args},
			func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				return &mcp.GetPromptResult{
					Description: p.desc,
					Messages: []*mcp.PromptMessage{{
						Role:    "user",
						Content: &mcp.TextContent{Text: p.text(req.Params.Arguments)},
					}},
				}, nil
			})
	}
}
