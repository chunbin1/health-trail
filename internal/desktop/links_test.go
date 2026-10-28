package desktop

import (
	"strings"
	"testing"
)

func TestLinkRoute(t *testing.T) {
	for link, want := range map[string]string{
		"healthtrail://appointments/1":              "/appointments/appointments/1",
		"healthtrail://appointments/42":             "/appointments/appointments/42",
		"healthtrail://medications/3":               "/medications/medications/3",
		"healthtrail://medications/3/202610250800":  "/medications/medications/3",
		"healthtrail://medications/3/2026102508":    "",
		"healthtrail://medications/3/20261025080x":  "",
		"healthtrail://appointments/1/202610250800": "",
		"healthtrail://appointments/0":              "",
		"healthtrail://appointments/01":             "",
		"healthtrail://appointments/-1":             "",
		"healthtrail://appointments/1?delete=1":     "",
		"healthtrail://appointments/1#x":            "",
		"healthtrail://appointments/99999999999":    "",
		"healthtrail://appointments/":               "",
		"healthtrail://appointments":                "",
		"healthtrail://doses/1":                     "",
		"healthtrail://settings/1":                  "",
		"healthtrail://appointments/1\";alert(1)//": "",
		"healthtrail://../appointments/1":           "",
		"http://appointments/1":                     "",
		"HEALTHTRAIL://appointments/1":              "",
		"":                                          "",
	} {
		if got := linkRoute(link, nil); got != want {
			t.Errorf("linkRoute(%q) = %q, want %q", link, got, want)
		}
	}
}

// The uid names the record on any device; without a match the id does.
func TestLinkRouteByUID(t *testing.T) {
	const uid = "0123456789abcdef0123456789abcdef"
	byUID := func(entity, u string) (int64, bool) {
		if entity == "appointments" && u == uid {
			return 7, true
		}
		return 0, false
	}
	for link, want := range map[string]string{
		"healthtrail://appointments/1?uid=" + uid:                           "/appointments/appointments/7",
		"healthtrail://medications/3/202610250800?uid=" + uid:               "/medications/medications/3",
		"healthtrail://appointments/1?uid=ffffffffffffffffffffffffffffffff": "/appointments/appointments/1",
		"healthtrail://appointments/1?uid=" + uid + "&delete=1":             "",
		"healthtrail://appointments/1?uid=0123":                             "",
		"healthtrail://appointments/1?uid=" + strings.ToUpper(uid):          "",
		"healthtrail://appointments/1?id=" + uid:                            "",
		"healthtrail://appointments/x?uid=" + uid:                           "",
	} {
		if got := linkRoute(link, byUID); got != want {
			t.Errorf("linkRoute(%q) = %q, want %q", link, got, want)
		}
	}
}

func TestLinkIn(t *testing.T) {
	if got := linkIn([]string{"/Applications/健康足迹.app/Contents/MacOS/health-trail", "-data", "/tmp/x", "healthtrail://appointments/1"}); got != "healthtrail://appointments/1" {
		t.Errorf("linkIn = %q", got)
	}
	if got := linkIn([]string{"health-trail", "-port", "8734"}); got != "" {
		t.Errorf("linkIn without a link = %q", got)
	}
}
