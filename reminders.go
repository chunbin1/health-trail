package main

import (
	"context"
	"log"
	"time"

	"github.com/lee/health-vault/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// reminderDays is how far ahead the app schedules. macOS here only takes
// one-off notifications, but it delivers them while the app is closed, so a
// week ahead keeps reminders coming as long as the app is opened weekly.
const reminderDays = 7

// reminders turns the habits' remind_at into macOS notifications. It brings
// the scheduled ones in line with the data at start and every 30 seconds:
// a check-in, a new time or an archived habit may come from MCP in another
// process, so there is nothing to listen to.
type reminders struct {
	st        *store.Store
	ns        *notifications.NotificationService
	scheduled map[string]bool
}

func (r *reminders) ServiceName() string { return "health-vault/reminders" }

// ServiceStartup starts the notification service itself, so that its failing
// (a binary run outside the .app has no bundle identifier) only turns
// reminders off instead of stopping the app.
func (r *reminders) ServiceStartup(ctx context.Context, opts application.ServiceOptions) error {
	if err := r.ns.ServiceStartup(ctx, opts); err != nil {
		log.Printf("提醒不可用（需要从 健康档案.app 启动）：%v", err)
		return nil
	}
	go r.run(ctx)
	return nil
}

func (r *reminders) run(ctx context.Context) {
	if ok, err := r.ns.RequestNotificationAuthorization(); !ok {
		log.Printf("没有通知权限，提醒不会弹出（系统设置 → 通知 → 健康档案）：%v", err)
		return
	}
	// what an earlier run scheduled is unknown here: start clean
	if err := r.ns.RemoveAllPendingNotifications(); err != nil {
		log.Printf("清除旧提醒失败：%v", err)
	}
	r.scheduled = map[string]bool{}
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		r.sync(time.Now())
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (r *reminders) sync(now time.Time) {
	plan, err := r.st.Reminders(now, reminderDays)
	if err != nil {
		log.Printf("读取提醒失败：%v", err)
		return
	}
	want := map[string]store.Reminder{}
	for _, p := range plan {
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
