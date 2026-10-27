package assistant

import (
	"regexp"
	"strings"
)

// Some models say they have booked or saved something without calling a
// tool, whatever the prompt says. A reply like that with no card is sent back
// once; if it still has no card, the reply says that nothing was saved.

// "已为您安排" is a claim; "我来帮您预约" is not.
var doneRe = regexp.MustCompile(`已经?(为您|帮您|给您|帮你|为你|给你)?(创建|安排|预约|设置|设定|保存|记下|记录|添加|修改|更新|改成|改为)|(创建|安排|预约|设置|设|保存|记|添加|修改|改|建)好了`)

// the same in English, for a model that answers in English
var doneEnRe = regexp.MustCompile(`(?i)\b(i've|i have|has been|have been|is now|are now|already|successfully)\s+(been\s+)?(booked|scheduled|saved|created|added|set up|set|recorded|logged|updated|changed)\b|\ball set\b`)

// claimsDone tells whether a reply says something was already done. "已经
// 记录过" is about an earlier reply, not this one.
func claimsDone(reply string) bool {
	for _, loc := range doneRe.FindAllStringIndex(reply, -1) {
		if !strings.HasPrefix(reply[loc[1]:], "过") {
			return true
		}
	}
	return doneEnRe.MatchString(reply)
}

const claimNudge = "（App 检查）你的回复说已经做好了，但这次没有调用 add_record、add_lab_report、checkin 或 update_record，什么都没有整理成卡片，也没有保存。" +
	"用户要记录或修改的，现在调用对应的工具整理成卡片，再写一两句回复；缺必要的信息（比如预约的具体时刻）就问用户，不要说已经做了。"

const claimNote = "\n\n（这次回复没有整理出卡片，什么都没有保存。）"

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
