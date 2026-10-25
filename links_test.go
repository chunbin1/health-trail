package main

import "testing"

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
		if got := linkRoute(link); got != want {
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
