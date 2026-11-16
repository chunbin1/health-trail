package assistant

import (
	"regexp"
	"strings"
)

// A model can say it changed the archive without calling a tool, and the
// wording is never a closed list. The check is structural: a reply that
// called nothing did not change anything. It is sent back once, unless it
// is asking the user something. If it still makes no card, the reply says
// the archive was not changed.

const claimNudge = "（App 检查）这一轮没有调用任何工具，所以没有卡片，档案也不会改。" +
	"用户要记录、修改、归档或打卡的，现在调用 add_record、update_record、add_lab_report 或 checkin，并在同一条消息里写给用户的那一两句。" +
	"只是在回答、解释或向用户追问的，把刚才的回复再写一遍，不要说已经改过档案。"

const claimNote = "\n\n（这轮没有修改档案。）"
const claimNoteEn = "\n\n(This reply didn't change your records.)"

// asks reports a reply that is waiting on the user, so it is not sent back.
func asks(reply string) bool {
	s := strings.TrimSpace(reply)
	return strings.HasSuffix(s, "？") || strings.HasSuffix(s, "?") || strings.HasSuffix(s, "吗")
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
