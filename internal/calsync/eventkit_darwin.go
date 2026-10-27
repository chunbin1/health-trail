//go:build darwin

package calsync

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework EventKit -framework Foundation -framework CoreGraphics
#import <EventKit/EventKit.h>
#include <stdlib.h>

// One store for the process. EventKit calls are serialised on the Go side.
static EKEventStore *ekStore(void) {
	static EKEventStore *store;
	static dispatch_once_t once;
	dispatch_once(&once, ^{ store = [[EKEventStore alloc] init]; });
	return store;
}

static EKEntityType ekType(int reminders) { return reminders ? EKEntityTypeReminder : EKEntityTypeEvent; }

static char *ekDup(NSString *s) { return s ? strdup([s UTF8String]) : NULL; }

static char *toJSON(id obj) {
	NSData *d = [NSJSONSerialization dataWithJSONObject:obj options:0 error:nil];
	return d ? strdup([[[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding] UTF8String]) : NULL;
}

static NSDictionary *fromJSON(const char *s) {
	NSData *d = [[NSString stringWithUTF8String:s] dataUsingEncoding:NSUTF8StringEncoding];
	return [NSJSONSerialization JSONObjectWithData:d options:0 error:nil];
}

// 0 not asked yet, 1 full access, 2 denied or restricted, 3 write only
static int ekStatus(int reminders) {
	EKAuthorizationStatus st = [EKEventStore authorizationStatusForEntityType:ekType(reminders)];
	if (st == EKAuthorizationStatusNotDetermined) return 0;
	if (@available(macOS 14.0, *)) {
		if (st == EKAuthorizationStatusFullAccess) return 1;
		if (st == EKAuthorizationStatusWriteOnly) return 3;
		return 2;
	}
	return st == 3 ? 1 : 2; // EKAuthorizationStatusAuthorized
}

// ekRequest asks for full access (the system's prompt, the first time) and
// waits for the answer.
static int ekRequest(int reminders) {
	__block BOOL ok = NO;
	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	void (^done)(BOOL, NSError *) = ^(BOOL granted, NSError *e) { ok = granted; dispatch_semaphore_signal(sem); };
	if (@available(macOS 14.0, *)) {
		if (reminders) [ekStore() requestFullAccessToRemindersWithCompletion:done];
		else [ekStore() requestFullAccessToEventsWithCompletion:done];
	} else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		[ekStore() requestAccessToEntityType:ekType(reminders) completion:done];
#pragma clang diagnostic pop
	}
	dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 300 * NSEC_PER_SEC));
	if (ok) [ekStore() reset]; // see the calendars that access now opens
	return ok;
}

// ekCalendar finds the app's calendar (or reminders list): the saved one, else
// one by its title, else a new one in iCloud (else where new items go by
// default, else on this Mac). Returns {"id", "source"}.
static char *ekCalendar(int reminders, const char *title, const char *saved, char **err) {
	EKEventStore *s = ekStore();
	EKEntityType t = ekType(reminders);
	NSString *name = [NSString stringWithUTF8String:title];
	EKCalendar *cal = saved && *saved ? [s calendarWithIdentifier:[NSString stringWithUTF8String:saved]] : nil;
	if (!cal || !cal.allowsContentModifications) {
		cal = nil;
		for (EKCalendar *c in [s calendarsForEntityType:t]) {
			if ([c.title isEqualToString:name] && c.allowsContentModifications) { cal = c; break; }
		}
	}
	if (!cal) {
		// accounts to try, in order: iCloud when it already keeps this kind
		// (with the newer iCloud Reminders not every iCloud entry does),
		// where new ones go by default, any account that keeps this kind,
		// then this Mac. The first that takes the new calendar wins.
		EKSource *def = (reminders ? s.defaultCalendarForNewReminders : s.defaultCalendarForNewEvents).source;
		NSMutableArray<EKSource *> *candidates = [NSMutableArray array];
		for (EKSource *x in s.sources) {
			if ([x.title isEqualToString:@"iCloud"] && [x calendarsForEntityType:t].count > 0) [candidates addObject:x];
		}
		if (def) [candidates addObject:def];
		for (EKSource *x in s.sources) {
			if ([x calendarsForEntityType:t].count > 0) [candidates addObject:x];
		}
		for (EKSource *x in s.sources) {
			if (x.sourceType == EKSourceTypeLocal) [candidates addObject:x];
		}
		NSError *last = nil;
		NSMutableSet *tried = [NSMutableSet set];
		for (EKSource *src in candidates) {
			if ([tried containsObject:src.sourceIdentifier]) continue;
			[tried addObject:src.sourceIdentifier];
			EKCalendar *c = [EKCalendar calendarForEntityType:t eventStore:s];
			c.title = name;
			c.source = src;
			CGColorRef green = CGColorCreateGenericRGB(0.18, 0.44, 0.37, 1); // the app's
			c.CGColor = green;
			CGColorRelease(green);
			NSError *e = nil;
			if ([s saveCalendar:c commit:YES error:&e]) { cal = c; break; }
			last = e;
		}
		if (!cal) {
			*err = ekDup(last ? last.localizedDescription : @"找不到可以新建日历的账户");
			return NULL;
		}
	}
	return toJSON(@{@"id": cal.calendarIdentifier, @"source": cal.source.title ?: @""});
}

static NSString *urlOf(EKCalendarItem *it) { return it.URL ? it.URL.absoluteString : @""; }

// ekEvents lists the calendar's events between from and to (unix seconds).
static char *ekEvents(const char *calID, double from, double to, char **err) {
	EKCalendar *cal = [ekStore() calendarWithIdentifier:[NSString stringWithUTF8String:calID]];
	if (!cal) { *err = ekDup(@"日历不见了"); return NULL; }
	NSPredicate *p = [ekStore() predicateForEventsWithStartDate:[NSDate dateWithTimeIntervalSince1970:from]
		endDate:[NSDate dateWithTimeIntervalSince1970:to] calendars:@[cal]];
	NSMutableArray *out = [NSMutableArray array];
	for (EKEvent *ev in [ekStore() eventsMatchingPredicate:p]) {
		[out addObject:@{@"id": ev.calendarItemIdentifier, @"url": urlOf(ev)}];
	}
	return toJSON(out);
}

// ekReminders lists the list's open reminders and those completed since
// (unix seconds).
static char *ekReminders(const char *listID, double since, char **err) {
	EKEventStore *s = ekStore();
	EKCalendar *cal = [s calendarWithIdentifier:[NSString stringWithUTF8String:listID]];
	if (!cal) { *err = ekDup(@"提醒事项列表不见了"); return NULL; }
	NSMutableArray *out = [NSMutableArray array];
	NSArray *preds = @[
		[s predicateForIncompleteRemindersWithDueDateStarting:nil ending:nil calendars:@[cal]],
		[s predicateForCompletedRemindersWithCompletionDateStarting:[NSDate dateWithTimeIntervalSince1970:since] ending:nil calendars:@[cal]],
	];
	for (NSPredicate *p in preds) {
		__block NSArray *found = nil;
		dispatch_semaphore_t sem = dispatch_semaphore_create(0);
		[s fetchRemindersMatchingPredicate:p completion:^(NSArray<EKReminder *> *rs) { found = rs; dispatch_semaphore_signal(sem); }];
		dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 60 * NSEC_PER_SEC));
		for (EKReminder *r in found) {
			[out addObject:@{@"id": r.calendarItemIdentifier, @"url": urlOf(r), @"completed": @(r.completed),
				@"completed_at": @(r.completionDate ? r.completionDate.timeIntervalSince1970 : 0)}];
		}
	}
	return toJSON(out);
}

static NSArray *alarmsFrom(NSDictionary *in) {
	NSMutableArray *alarms = [NSMutableArray array];
	for (NSNumber *at in in[@"alarms"] ?: @[]) {
		[alarms addObject:[EKAlarm alarmWithAbsoluteDate:[NSDate dateWithTimeIntervalSince1970:at.doubleValue]]];
	}
	return alarms;
}

// ekSaveEvent creates or updates (by "id") an event from {title, start, end,
// location, notes, url, alarms: [unix seconds]}. Returns its id.
static char *ekSaveEvent(const char *calID, const char *json, char **err) {
	EKEventStore *s = ekStore();
	NSDictionary *in = fromJSON(json);
	EKCalendar *cal = [s calendarWithIdentifier:[NSString stringWithUTF8String:calID]];
	if (!cal) { *err = ekDup(@"日历不见了"); return NULL; }
	EKEvent *ev = nil;
	if ([in[@"id"] length]) {
		id it = [s calendarItemWithIdentifier:in[@"id"]];
		if ([it isKindOfClass:[EKEvent class]]) ev = it;
	}
	if (!ev) ev = [EKEvent eventWithEventStore:s];
	ev.calendar = cal;
	ev.title = in[@"title"];
	ev.startDate = [NSDate dateWithTimeIntervalSince1970:[in[@"start"] doubleValue]];
	ev.endDate = [NSDate dateWithTimeIntervalSince1970:[in[@"end"] doubleValue]];
	ev.location = [in[@"location"] length] ? in[@"location"] : nil;
	ev.notes = [in[@"notes"] length] ? in[@"notes"] : nil;
	ev.URL = [NSURL URLWithString:in[@"url"]];
	ev.alarms = alarmsFrom(in);
	NSError *e = nil;
	if (![s saveEvent:ev span:EKSpanThisEvent commit:YES error:&e]) { *err = ekDup(e.localizedDescription); return NULL; }
	return ekDup(ev.calendarItemIdentifier);
}

// ekSaveReminder creates or updates (by "id") a reminder from {title, due,
// notes, url, completed, alarms}. Returns its id.
static char *ekSaveReminder(const char *listID, const char *json, char **err) {
	EKEventStore *s = ekStore();
	NSDictionary *in = fromJSON(json);
	EKCalendar *cal = [s calendarWithIdentifier:[NSString stringWithUTF8String:listID]];
	if (!cal) { *err = ekDup(@"提醒事项列表不见了"); return NULL; }
	EKReminder *r = nil;
	if ([in[@"id"] length]) {
		id it = [s calendarItemWithIdentifier:in[@"id"]];
		if ([it isKindOfClass:[EKReminder class]]) r = it;
	}
	if (!r) r = [EKReminder reminderWithEventStore:s];
	r.calendar = cal;
	r.title = in[@"title"];
	r.notes = [in[@"notes"] length] ? in[@"notes"] : nil;
	r.URL = [NSURL URLWithString:in[@"url"]];
	NSDate *due = [NSDate dateWithTimeIntervalSince1970:[in[@"due"] doubleValue]];
	NSCalendar *greg = [NSCalendar calendarWithIdentifier:NSCalendarIdentifierGregorian];
	NSDateComponents *dc = [greg components:NSCalendarUnitYear | NSCalendarUnitMonth | NSCalendarUnitDay | NSCalendarUnitHour | NSCalendarUnitMinute fromDate:due];
	dc.timeZone = [NSTimeZone localTimeZone];
	r.dueDateComponents = dc;
	r.alarms = alarmsFrom(in);
	r.completed = [in[@"completed"] boolValue];
	NSError *e = nil;
	if (![s saveReminder:r commit:YES error:&e]) { *err = ekDup(e.localizedDescription); return NULL; }
	return ekDup(r.calendarItemIdentifier);
}

// ekRemove deletes an event or a reminder; one that's already gone is fine.
static int ekRemove(const char *itemID, char **err) {
	EKEventStore *s = ekStore();
	id it = [s calendarItemWithIdentifier:[NSString stringWithUTF8String:itemID]];
	NSError *e = nil;
	BOOL ok = YES;
	if ([it isKindOfClass:[EKEvent class]]) ok = [s removeEvent:it span:EKSpanThisEvent commit:YES error:&e];
	else if ([it isKindOfClass:[EKReminder class]]) ok = [s removeReminder:it commit:YES error:&e];
	if (!ok) { *err = ekDup(e.localizedDescription); return 0; }
	return 1;
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"sync"
	"time"
	"unsafe"

	"github.com/lee/health-trail/internal/msg"
)

// EventKit is the Mac's Calendar and Reminders, through EventKit. What is
// written there syncs through iCloud to the iPhone and the watch.
type EventKit struct{ mu sync.Mutex }

func cstr(s string) (*C.char, func()) {
	c := C.CString(s)
	return c, func() { C.free(unsafe.Pointer(c)) }
}

// result turns a returned C string and error into Go, freeing both.
func result(out, cerr *C.char) (string, error) {
	if cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
		return "", errors.New(C.GoString(cerr))
	}
	if out == nil {
		return "", msg.New("EventKit 没有返回结果", "EventKit returned nothing")
	}
	defer C.free(unsafe.Pointer(out))
	return C.GoString(out), nil
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func (k *EventKit) Access(reminders bool) access {
	k.mu.Lock()
	defer k.mu.Unlock()
	return access(C.ekStatus(cbool(reminders)))
}

func (k *EventKit) Request(reminders bool) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return C.ekRequest(cbool(reminders)) == 1
}

func (k *EventKit) Calendar(reminders bool, title, saved string) (id, source string, err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	t, free1 := cstr(title)
	defer free1()
	sv, free2 := cstr(saved)
	defer free2()
	var cerr *C.char
	out, err := result(C.ekCalendar(cbool(reminders), t, sv, &cerr), cerr)
	if err != nil {
		return "", "", err
	}
	var r struct{ ID, Source string }
	err = json.Unmarshal([]byte(out), &r)
	return r.ID, r.Source, err
}

func (k *EventKit) Events(calID string, from, to time.Time) ([]calItem, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	c, free := cstr(calID)
	defer free()
	var cerr *C.char
	out, err := result(C.ekEvents(c, C.double(from.Unix()), C.double(to.Unix()), &cerr), cerr)
	if err != nil {
		return nil, err
	}
	var items []calItem
	err = json.Unmarshal([]byte(out), &items)
	return items, err
}

func (k *EventKit) Reminders(listID string, completedSince time.Time) ([]calItem, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	c, free := cstr(listID)
	defer free()
	var cerr *C.char
	out, err := result(C.ekReminders(c, C.double(completedSince.Unix()), &cerr), cerr)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID          string  `json:"id"`
		URL         string  `json:"url"`
		Completed   bool    `json:"completed"`
		CompletedAt float64 `json:"completed_at"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, err
	}
	items := make([]calItem, len(raw))
	for i, r := range raw {
		items[i] = calItem{ID: r.ID, URL: r.URL, Completed: r.Completed}
		if r.CompletedAt > 0 {
			items[i].CompletedAt = time.Unix(int64(r.CompletedAt), 0)
		}
	}
	return items, nil
}

func (k *EventKit) save(fn func(*C.char, *C.char, **C.char) *C.char, calID string, v any) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	c, free1 := cstr(calID)
	defer free1()
	j, free2 := cstr(string(raw))
	defer free2()
	var cerr *C.char
	return result(fn(c, j, &cerr), cerr)
}

func (k *EventKit) SaveEvent(calID string, e calEvent) (string, error) {
	return k.save(func(c, j *C.char, err **C.char) *C.char { return C.ekSaveEvent(c, j, err) }, calID, e)
}

func (k *EventKit) SaveReminder(listID string, r calReminder) (string, error) {
	return k.save(func(c, j *C.char, err **C.char) *C.char { return C.ekSaveReminder(c, j, err) }, listID, r)
}

func (k *EventKit) Remove(id string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	c, free := cstr(id)
	defer free()
	var cerr *C.char
	if C.ekRemove(c, &cerr) == 0 {
		_, err := result(nil, cerr)
		return err
	}
	return nil
}
