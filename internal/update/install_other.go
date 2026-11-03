//go:build !darwin && !windows

package update

import (
	"context"
	"errors"
)

// Only the Mac app and the Windows exe replace themselves.

func Bundle() string { return "" }

func (in *Installer) Check() (string, error) { return "", ErrUnsigned }

func (in *Installer) Install(context.Context, *Release) (string, error) { return "", ErrUnsigned }

func Relaunch(string, string, []string) error { return errors.New("not supported here") }
