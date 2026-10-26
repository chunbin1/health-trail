package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/lee/health-trail/internal/backup"
	"github.com/lee/health-trail/internal/update"
)

// updates looks for a newer release on GitHub (at start, every few hours
// and when the window comes up, unless switched off, and when asked) and
// installs it when the person
// clicks: a backup first, then the new copy in place of this one, then a
// relaunch. Window only: an install over the network would restart the app
// under the person.
type updates struct {
	settings *settings
	url      string // update.LatestURL; HEALTH_TRAIL_UPDATE_URL for a test server
	client   *http.Client
	app      string         // the running .app, "" outside one
	backup   func() error   // a backup before installing, when one is set up
	quit     func()         // quits the app, for the relaunch
	changed  func(*updates) // the menu bar follows what's found
	checked  sync.Once      // whether this copy can replace itself, asked once
	checkErr error
	opened   chan struct{} // the window came up: a check if the last is a while ago
	mu       sync.Mutex
	state    updateState
}

type updateState struct {
	CheckedAt string          `json:"checked_at,omitempty"` // YYYY-MM-DD HH:MM:SS
	Checking  bool            `json:"checking"`
	Latest    *update.Release `json:"latest,omitempty"` // only when newer than this build
	Error     string          `json:"error,omitempty"`
	Phase     string          `json:"phase,omitempty"` // backup, download, install, restart
	Done      int64           `json:"done,omitempty"`  // of the download
	Total     int64           `json:"total,omitempty"`
}

func newUpdates(s *settings) *updates {
	u := &updates{settings: s, url: update.LatestURL, client: &http.Client{Timeout: 10 * time.Minute}, app: update.Bundle(), opened: make(chan struct{}, 1)}
	if v := os.Getenv("HEALTH_TRAIL_UPDATE_URL"); v != "" {
		u.url = v
	}
	update.Cleanup(u.app)
	return u
}

// How often it looks by itself: GitHub answers 60 requests an hour without
// a token, so this stays far below that.
const (
	checkEvery   = 4 * time.Hour    // in the background
	checkOnOpen  = 30 * time.Minute // when the window comes up
	checkLooking = 10 * time.Minute // how soon a check missed in sleep is made up
)

// due says whether a check is due, the last one made at last (zero if
// none yet); onOpen for the window coming up.
func due(now, last time.Time, onOpen bool) bool {
	if onOpen {
		return now.Sub(last) >= checkOnOpen
	}
	return now.Sub(last) >= checkEvery
}

// loop checks a little after start, then every few hours (looking often,
// as the Mac may have slept through the time), and when the window comes
// up after a while.
func (u *updates) loop(ctx context.Context) {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		onOpen := false
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			timer.Reset(checkLooking)
		case <-u.opened:
			onOpen = true
		}
		u.mu.Lock()
		last, _ := time.ParseInLocation(stamp, u.state.CheckedAt, time.Local)
		u.mu.Unlock()
		if u.settings.autoUpdate() && due(time.Now(), last, onOpen) {
			u.check(ctx)
		}
	}
}

// windowOpened tells the loop the window came up (or to the front).
func (u *updates) windowOpened() {
	select {
	case u.opened <- struct{}{}:
	default:
	}
}

// check asks GitHub once; a check already under way is waited for.
func (u *updates) check(ctx context.Context) {
	u.mu.Lock()
	if u.state.Checking || u.state.Phase != "" {
		u.mu.Unlock()
		return
	}
	u.state.Checking = true
	u.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r, err := update.Latest(ctx, u.client, u.url)
	u.mu.Lock()
	u.state.Checking = false
	u.state.CheckedAt = time.Now().Format(stamp)
	u.state.Error = ""
	switch {
	case err != nil:
		log.Printf("检查更新失败：%v", err)
		u.state.Error = u.settings.msg("没能连上 GitHub 检查更新，请稍后再试", "Couldn't reach GitHub to check for updates; try again later")
		if errors.Is(err, update.ErrNoRelease) {
			u.state.Error = u.settings.msg("还没有找到发布的版本", "No published version found yet")
		}
	case update.Newer(r.Version, Version):
		if u.state.Latest == nil || u.state.Latest.Version != r.Version {
			log.Printf("有新版本：%s（当前 %s）", r.Version, Version)
		}
		u.state.Latest = r
	default:
		u.state.Latest = nil
	}
	u.mu.Unlock()
	if u.changed != nil {
		u.changed(u)
	}
}

// latest is the newer release found, or nil.
func (u *updates) latest() *update.Release {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state.Latest
}

func (u *updates) installer() *update.Installer {
	return &update.Installer{App: u.app, BundleID: bundleID, Client: u.client, Progress: func(done, total int64) {
		u.mu.Lock()
		u.state.Done, u.state.Total = done, total
		u.mu.Unlock()
	}}
}

// whyNot says, in the person's language, why this copy can't update itself
// ("" when it can): what the settings page shows instead of the button.
func (u *updates) whyNot() string {
	if u.app == "" {
		return u.settings.msg("只有 Mac 上的健康足迹 App 能自己更新", "Only the Health Trail Mac app can update itself")
	}
	u.checked.Do(func() { _, u.checkErr = u.installer().Check() })
	switch err := u.checkErr; {
	case err == nil:
		return ""
	case errors.Is(err, update.ErrUnsigned):
		return u.settings.msg("这一份是自己构建的，不能自动更新；要换成正式版，请下载 dmg 安装", "This copy was built locally and can't update itself; download the dmg to switch to the released app")
	default:
		return u.settings.msg("请先把健康足迹拖进「应用程序」文件夹再更新，或者下载 dmg 手动安装", "Move Health Trail into the Applications folder first, or download the dmg and install it by hand")
	}
}

// install runs in the background; the page follows it through status.
func (u *updates) install() error {
	if why := u.whyNot(); why != "" {
		return errors.New(why)
	}
	u.mu.Lock()
	r := u.state.Latest
	if r == nil || !r.Installable() || u.state.Phase != "" {
		u.mu.Unlock()
		return errors.New(u.settings.msg("现在没有可以安装的新版本", "There's no update to install right now"))
	}
	u.state.Phase, u.state.Error, u.state.Done, u.state.Total = "backup", "", 0, r.ZipSize
	u.mu.Unlock()
	go u.run(r)
	return nil
}

func (u *updates) run(r *update.Release) {
	fail := func(msg string, err error) {
		log.Printf("更新到 %s 失败：%v", r.Version, err)
		u.mu.Lock()
		u.state.Phase, u.state.Error = "", msg
		u.mu.Unlock()
	}
	if u.backup != nil {
		if err := u.backup(); err != nil {
			fail(u.settings.msg("更新前的备份没有成功，没有更新。请先在「备份」里看看哪里出了问题", "The backup before updating failed, so nothing was updated. Check Backups first"), err)
			return
		}
	}
	u.setPhase("download")
	stage, err := u.installer().Install(context.Background(), r)
	if err != nil {
		msg := u.settings.msg("下载没有完成，请稍后再试；也可以下载 dmg 手动安装", "The download didn't finish; try again later, or download the dmg and install it by hand")
		switch {
		case errors.Is(err, update.ErrBadDownload):
			msg = u.settings.msg("下载的文件没有通过签名和公证检查，没有安装。请到发布页面下载 dmg", "The download didn't pass the signature and notarization checks, so it wasn't installed. Get the dmg from the release page")
		case errors.Is(err, update.ErrNotMovable):
			msg = u.settings.msg("没能替换健康足迹所在的文件夹，请下载 dmg 手动安装", "Couldn't replace the app in its folder; download the dmg and install it by hand")
		}
		fail(msg, err)
		return
	}
	u.setPhase("restart")
	log.Printf("已更新到 %s，正在重启", r.Version)
	if err := update.Relaunch(u.app, stage, os.Args[1:]); err != nil {
		// the new copy is in place: it opens next time
		fail(u.settings.msg("新版本已经装好，请退出健康足迹再打开", "The new version is installed; quit Health Trail and open it again"), err)
		return
	}
	u.quit()
}

func (u *updates) setPhase(p string) {
	u.mu.Lock()
	u.state.Phase = p
	u.mu.Unlock()
}

func (u *updates) status() map[string]any {
	u.mu.Lock()
	st := u.state
	u.mu.Unlock()
	out := map[string]any{"current": Version, "auto": u.settings.autoUpdate(), "state": st}
	if st.Latest != nil {
		out["installable"] = st.Latest.Installable()
		out["why_not"] = u.whyNot()
	}
	return out
}

func (u *updates) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, u.status())
	})
	mux.HandleFunc("POST /api/update/check", func(w http.ResponseWriter, r *http.Request) {
		u.check(r.Context())
		writeJSON(w, u.status())
	})
	mux.HandleFunc("PUT /api/update", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Auto bool `json:"auto"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, "无效的请求")
			return
		}
		u.settings.mu.Lock()
		u.settings.NoUpdateCheck = !in.Auto
		u.settings.mu.Unlock()
		if err := u.settings.save(); err != nil {
			log.Printf("保存设置失败：%v", err)
		}
		writeJSON(w, u.status())
	})
	mux.HandleFunc("POST /api/update/install", func(w http.ResponseWriter, r *http.Request) {
		if err := u.install(); err != nil {
			writeError(w, err.Error())
			return
		}
		writeJSON(w, u.status())
	})
}

// beforeUpdate backs up before an install, when backups are set up.
func (b *backups) beforeUpdate() error {
	if b.load().Config.Dir == "" {
		return nil
	}
	_, err := b.run(backup.BeforeUpdate)
	return err
}
