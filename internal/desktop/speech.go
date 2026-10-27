package desktop

import (
	"errors"
	"fmt"
	"time"
)

// How long one dictation may run before it stops on its own.
var speechLimit = 60 * time.Second

// Ear is one dictation session. The Mac window uses the system recognizer;
// tests use a stand-in.
type Ear interface {
	Available() bool
	Start(locale string) error
	Stop() (string, error)
	View() SpeechView
}

// SpeechView is what the page polls while listening.
type SpeechView struct {
	Listening bool
	Text      string
	ErrCode   int
}

// speechFailure is a recognizer problem named by a code. The page gets a
// sentence in its language, not the code.
type speechFailure struct{ Code int }

func (e speechFailure) Error() string { return fmt.Sprintf("speech %d", e.Code) }

var errSpeechUnavailable = errors.New("speech unavailable")

func (a *app) beginSpeech() error {
	if a.ear == nil || !a.ear.Available() {
		return errSpeechUnavailable
	}
	locale := "zh-CN"
	if a.settings.lang() == "en" {
		locale = "en-US"
	}
	if err := a.ear.Start(locale); err != nil {
		return err
	}
	a.speechMu.Lock()
	defer a.speechMu.Unlock()
	if a.speechTimer != nil {
		a.speechTimer.Stop()
	}
	a.speechTimer = time.AfterFunc(speechLimit, func() {
		a.speechMu.Lock()
		defer a.speechMu.Unlock()
		if a.ear != nil {
			a.ear.Stop()
		}
		a.speechTimer = nil
	})
	return nil
}

func (a *app) endSpeech() (string, error) {
	a.speechMu.Lock()
	if a.speechTimer != nil {
		a.speechTimer.Stop()
		a.speechTimer = nil
	}
	ear := a.ear
	a.speechMu.Unlock()
	if ear == nil {
		return "", errSpeechUnavailable
	}
	return ear.Stop()
}

func (a *app) speechMessage(err error) string {
	var se speechFailure
	if errors.As(err, &se) {
		switch se.Code {
		case 1:
			return a.msg("没有语音识别的权限。请在「系统设置 → 隐私与安全性 → 语音识别」里允许健康足迹。",
				"Speech recognition isn't allowed. Allow Health Trail in System Settings → Privacy & Security → Speech Recognition.")
		case 2:
			return a.msg("没有麦克风的权限。请在「系统设置 → 隐私与安全性 → 麦克风」里允许健康足迹。",
				"The microphone isn't allowed. Allow Health Trail in System Settings → Privacy & Security → Microphone.")
		case 3:
			return a.msg("这台 Mac 还不能在本机识别这种语言。请到「系统设置 → 键盘 → 听写」打开听写并下载语言。不会改用云端识别。",
				"This Mac can't recognize that language on the device yet. Turn on Dictation in System Settings → Keyboard → Dictation and download the language. Audio is not sent to a cloud recognizer.")
		case 4:
			return a.msg("正在听，请先结束这一次。", "Already listening. Finish this one first.")
		}
	}
	if errors.Is(err, errSpeechUnavailable) {
		return a.msg("只有 Mac 窗口里可以语音。", "Voice input is only in the Mac window.")
	}
	return a.msg("没能开始听写，请再试一次。", "Couldn't start listening. Try again.")
}
