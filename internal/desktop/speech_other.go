//go:build !darwin

package desktop

type stubEar struct{}

func platformEar() Ear { return stubEar{} }

func (stubEar) Available() bool { return false }

func (stubEar) Start(string) error { return errSpeechUnavailable }

func (stubEar) Stop() (string, error) { return "", errSpeechUnavailable }

func (stubEar) View() SpeechView { return SpeechView{} }
