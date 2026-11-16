package assistant

import (
	"regexp"
	"strings"
)

// A model can say it changed the archive without calling a tool, and the
// wording is never a closed list. The check combines user intent (asking for
// or confirming an action) and whether the reply claims changes were made.
// A reply with no cards is sent back once if an action was requested or claimed;
// questions and pure Q&A are left alone. If it still makes no card, the reply
// says the archive was not changed.

const claimNudge = "（App 检查）这一轮没有调用任何工具，所以没有卡片，档案也不会改。" +
	"用户要记录、修改、归档或打卡的，现在调用 add_record、update_record、add_lab_report 或 checkin，并在同一条消息里写给用户的那一两句。" +
	"只是在回答、解释或向用户追问的，把刚才的回复再写一遍，不要说已经改过档案。"

const claimNote = "\n\n（这轮没有修改档案。）"
const claimNoteEn = "\n\n(This reply didn't change your records.)"

// doneRe matches Chinese statements claiming an archive change was made.
var doneRe = regexp.MustCompile(
	`已经?(为您|帮您|给您|帮你|为你|给你)?(创建|安排|预约|设置|设定|保存|记下|记录|添加|修改|更新|改成|改为|归档|停用|新建|建立|删除|撤销|登记|调整)` +
		`|(创建|安排|预约|设置|设|保存|记|添加|修改|改|建|整理|登记|调整|归档|停用|更新)好了` +
		`|整理成(新)?(打卡项|记录|预约|用药|健康问题|习惯)` +
		`|(我把|把它|将它)(它|旧的)?(归档|停用|删除|修改|改成|更新)` +
		`|(已|已经|帮您|为您)(把|将)?[^\n，。！？；]*(归档|停用|删除|修改|改成)`,
)

var doneEnRe = regexp.MustCompile(`(?i)\b(i've|i have|has been|have been|is now|are now|already|successfully)\s+(been\s+)?(booked|scheduled|saved|created|added|set up|set|recorded|logged|updated|changed|archived|deleted)\b|\ball set\b`)

// claimsChange tells whether a reply says an archive change was already made.
// "已经记录过" is about an earlier reply or history, not this one.
func claimsChange(reply string) bool {
	for _, loc := range doneRe.FindAllStringIndex(reply, -1) {
		if !strings.HasPrefix(reply[loc[1]:], "过") {
			return true
		}
	}
	return doneEnRe.MatchString(reply)
}

// actionUserRe matches user requests to create, change, archive, delete,
// remind, or check in, or affirmative approvals of suggested actions.
var actionUserRe = regexp.MustCompile(
	`^(可以|好的?|行|没问题|直接安排|你定|按你说的|按你的建议|听你的|好啊|可以的|行啊|ok|yes|sure)[。！!.]*$` +
		`|(?i)(帮我|替我|请帮|麻烦)?(记录|记下|记一下|记个|记一条|录入|登记|打卡|设提醒|设置提醒|提醒我|保存|存一下)` +
		`|(?i)(新建|创建|添加|加上|加一个|弄个|建一个|建立|搞个).*(打卡|打卡项|习惯|记录|预约|用药|健康问题|病史|提醒)` +
		`|(?i)(修改|改一下|改成|改为|更新|调成|调整|删除|删掉|不要了|归档|停用)` +
		`|(?i)\b(remind me|book|schedule|create|add|update|delete|archive|check in|log)\b`,
)

// userWantsAction reports whether the latest user turn was requesting or
// approving a record change.
func userWantsAction(turns []Turn) bool {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role == "user" {
			text := strings.TrimSpace(turns[i].Text)
			return actionUserRe.MatchString(text)
		}
	}
	return false
}

// asks reports a reply that is waiting on the user, so it is not sent back.
func asks(reply string) bool {
	s := plainReply(reply)
	s = strings.TrimRight(s, " \t\r\n。！!.”\"'）)》>」』")
	if isQuestionSuffix(s) {
		return true
	}
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimRight(lines[i], " \t\r\n。！!.”\"'）)》>」』")
		if line == "" {
			continue
		}
		return isQuestionSuffix(line)
	}
	return false
}

func isQuestionSuffix(s string) bool {
	return strings.HasSuffix(s, "？") || strings.HasSuffix(s, "?") ||
		strings.HasSuffix(s, "吗") || strings.HasSuffix(s, "呢") || strings.HasSuffix(s, "吧")
}

var (
	boldRe    = regexp.MustCompile(`\*\*|__`)
	headingRe = regexp.MustCompile(`(?m)^#{1,6}[ \t]+`)
	bulletRe  = regexp.MustCompile(`(?m)^([ \t]*)[-*][ \t]+`)
)

// plainReply takes out the markdown the reply is not meant to have: the
// window shows it as it is.
func plainReply(s string) string {
	s = boldRe.ReplaceAllString(s, "")
	s = headingRe.ReplaceAllString(s, "")
	s = bulletRe.ReplaceAllString(s, "${1}· ")
	return strings.TrimSpace(s)
}
