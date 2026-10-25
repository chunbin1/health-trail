// Package reminders shows what is due as macOS notifications.
package reminders

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lee/health-trail/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// reminderDays is how far ahead the app schedules. The app stays in the
// menu bar and brings them up to date every 30 seconds; what is scheduled
// still comes if it was quit. Two days keeps a few medicines a day under
// macOS's 64 pending notifications.
const reminderDays = 2

// Service turns what is due (store.Reminders: habits, medicines,
// appointments) into macOS notifications. It brings
// the scheduled ones in line with the data at start and every 30 seconds:
// a check-in, a new time or an archived habit may come from MCP in another
// process, so there is nothing to listen to.
type Service struct {
	st        *store.Store
	ns        *notifications.NotificationService
	lang      func() string // the UI's language, which notifications follow
	scheduled map[string]bool
	allowed   atomic.Int32  // notifications allowed in System Settings: 1 yes, 0 no, -1 not known yet
	kick      chan struct{} // a round now, instead of at the next tick
	// synced reports what goes to Calendar (appointments) and Reminders
	// (medicines): those remind there, on the Mac too, so not here as well
	synced func() (appointments, medicines bool)
}

// New is the reminders for st, in the language lang gives, leaving to
// Calendar and Reminders what synced says goes there.
func New(st *store.Store, ns *notifications.NotificationService, lang func() string, synced func() (appointments, medicines bool)) *Service {
	r := &Service{st: st, ns: ns, lang: lang, synced: synced}
	r.allowed.Store(-1) // not known until the service starts
	return r
}

// Allowed asks macOS now whether the app may show notifications, so the
// window's warning goes as soon as they're turned on; ok is false when it
// can't be known (reminders didn't start). Newly allowed, the reminders are
// scheduled right away.
func (r *Service) Allowed() (allowed, ok bool) {
	if r.allowed.Load() < 0 {
		return false, false
	}
	allowed, err := r.ns.CheckNotificationAuthorization()
	if err != nil {
		v := r.allowed.Load()
		return v == 1, v >= 0
	}
	if allowed && r.allowed.Load() != 1 {
		select {
		case r.kick <- struct{}{}:
		default:
		}
	}
	return allowed, true
}

func (r *Service) ServiceName() string { return "health-trail/reminders" }

// ServiceStartup starts the notification service itself, so that its failing
// (a binary run outside the .app has no bundle identifier) only turns
// reminders off instead of stopping the app.
func (r *Service) ServiceStartup(ctx context.Context, opts application.ServiceOptions) error {
	r.allowed.Store(-1)
	r.kick = make(chan struct{}, 1)
	if err := r.ns.ServiceStartup(ctx, opts); err != nil {
		log.Printf("提醒不可用（需要从 健康足迹.app 启动）：%v", err)
		return nil
	}
	go r.run(ctx)
	return nil
}

// run asks for permission once, then keeps the notifications in line with
// the data. The app stays running for days, so permission is checked on
// every round: turned on later in System Settings, reminders start then
// (and the window stops warning about it).
func (r *Service) run(ctx context.Context) {
	ok, err := r.ns.RequestNotificationAuthorization()
	r.setAllowed(ok)
	if !ok {
		log.Printf("没有通知权限，提醒不会弹出（系统设置 → 通知 → 健康足迹）：%v", err)
	}
	// what an earlier run scheduled is unknown here: start clean
	if err := r.ns.RemoveAllPendingNotifications(); err != nil {
		log.Printf("清除旧提醒失败：%v", err)
	}
	r.scheduled = map[string]bool{}
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		if ok, _ := r.ns.CheckNotificationAuthorization(); ok {
			if r.allowed.Load() != 1 {
				r.scheduled = map[string]bool{} // allowed just now: schedule everything
			}
			r.setAllowed(true)
			r.sync(time.Now())
		} else {
			r.setAllowed(false)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-r.kick:
		}
	}
}

func (r *Service) setAllowed(ok bool) {
	if ok {
		r.allowed.Store(1)
	} else {
		r.allowed.Store(0)
	}
}

func (r *Service) sync(now time.Time) {
	plan, err := r.st.Reminders(now, reminderDays)
	if err != nil {
		log.Printf("读取提醒失败：%v", err)
		return
	}
	// the text is part of the id, so a renamed medicine, a new place or a
	// switch of language replaces what was scheduled
	en := r.lang() == "en"
	appts, meds := false, false
	if r.synced != nil {
		appts, meds = r.synced()
	}
	want := map[string]store.Reminder{}
	for _, p := range plan {
		if appts && strings.HasPrefix(p.ID, "appt-") || meds && strings.HasPrefix(p.ID, "med-") {
			continue
		}
		if en {
			p.Title, p.Body = p.TitleEN, p.BodyEN
		}
		sum := sha256.Sum256([]byte(p.Title + "\n" + p.Body))
		p.ID += "-" + hex.EncodeToString(sum[:4])
		want[p.ID] = p
	}
	for id := range r.scheduled {
		if _, ok := want[id]; !ok {
			r.ns.RemovePendingNotification(id)
			delete(r.scheduled, id)
		}
	}
	for id, p := range want {
		if r.scheduled[id] {
			continue
		}
		err := r.ns.SendNotification(notifications.NotificationOptions{
			ID:       id,
			Title:    p.Title,
			Body:     p.Body,
			Schedule: &notifications.NotificationSchedule{At: p.At.Unix()},
		})
		if err != nil {
			log.Printf("设置提醒失败（%s）：%v", id, err)
			continue
		}
		r.scheduled[id] = true
	}
}
