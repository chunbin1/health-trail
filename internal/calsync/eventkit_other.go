//go:build !darwin

package calsync

import "time"

// EventKit is the Mac's Calendar and Reminders. Other systems have no
// equivalent wired up, so the sync stays off and the settings page hides it.
type EventKit struct{}

func (k *EventKit) Supported() bool { return false }

func (k *EventKit) Access(bool) access { return accessDenied }

func (k *EventKit) Request(bool) bool { return false }

func (k *EventKit) Calendar(bool, string, string) (string, string, error) {
	return "", "", errUnsupported()
}

func (k *EventKit) Events(string, time.Time, time.Time) ([]calItem, error) {
	return nil, errUnsupported()
}

func (k *EventKit) Reminders(string, time.Time) ([]calItem, error) {
	return nil, errUnsupported()
}

func (k *EventKit) SaveEvent(string, calEvent) (string, error) {
	return "", errUnsupported()
}

func (k *EventKit) SaveReminder(string, calReminder) (string, error) {
	return "", errUnsupported()
}

func (k *EventKit) Remove(string) error { return errUnsupported() }
