// Package msg holds messages for people in both of the app's languages.
// The Chinese text is the error's text, the one MCP and the model read; a
// window or page in English shows the English one (In).
package msg

import (
	"errors"
	"fmt"
	"strings"
)

// English is anything that has its own English text: an Error, a Word,
// store.ValidationError.
type English interface{ English() string }

// Error is a message for a person in Chinese and English.
type Error struct {
	Zh, En string
	cause  error
}

func (e *Error) Error() string   { return e.Zh }
func (e *Error) English() string { return e.En }
func (e *Error) Unwrap() error   { return e.cause }

// New is an Error with fixed text.
func New(zh, en string) *Error { return &Error{Zh: zh, En: en} }

// Errorf formats zh and en with the same args. In the English text, an arg
// with English text (an Error, a Word) is written in English. An error arg
// is kept as the cause, for errors.Is and errors.As.
func Errorf(zh, en string, args ...any) *Error {
	e := &Error{Zh: fmt.Sprintf(zh, args...), En: Sprintf("en", en, args...)}
	for _, a := range args {
		if err, ok := a.(error); ok {
			e.cause = err
		}
	}
	return e
}

// Sprintf formats with each arg in lang: "en" writes an arg that has
// English text in English.
func Sprintf(lang, format string, args ...any) string {
	if lang != "en" {
		return fmt.Sprintf(format, args...)
	}
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = a
		if e, ok := a.(English); ok {
			out[i] = e.English()
		} else if err, ok := a.(error); ok {
			out[i] = In(err, "en")
		}
	}
	return fmt.Sprintf(format, out...)
}

// Word is a name or label in both languages; it prints as the Chinese.
type Word struct{ Zh, En string }

func (w Word) String() string  { return w.Zh }
func (w Word) English() string { return w.En }

// In is err's text in lang ("en" or anything else for Chinese). An error
// with no English text is shown as it is.
func In(err error, lang string) string {
	if err == nil {
		return ""
	}
	if lang == "en" {
		var e English
		if errors.As(err, &e) {
			if s := e.English(); s != "" {
				return s
			}
		}
	}
	return err.Error()
}

// Pick is zh or en by lang.
func Pick(lang, zh, en string) string {
	if lang == "en" {
		return en
	}
	return zh
}

// Join is errors.Join in both languages; nil when every err is nil.
func Join(errs ...error) error {
	var zh, en []string
	var first error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if first == nil {
			first = err
		}
		zh = append(zh, err.Error())
		en = append(en, In(err, "en"))
	}
	if first == nil {
		return nil
	}
	return &Error{Zh: strings.Join(zh, "\n"), En: strings.Join(en, "\n"), cause: first}
}
