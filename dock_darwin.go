//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// launchedAtLogin is set while the app finishes launching: 1 when macOS
// opened it as a login item, 0 otherwise, -1 before it is known. The open
// event says so (keyAELaunchedAsLogInItem) only while it is being handled,
// so it is read in the launch notifications, before Wails' Go side runs.
static int launchedAtLogin = -1;

static void readLaunchEvent(void) {
	NSAppleEventDescriptor *ev = [[NSAppleEventManager sharedAppleEventManager] currentAppleEvent];
	if (ev && [ev eventID] == kAEOpenApplication &&
	    [[ev paramDescriptorForKeyword:keyAEPropData] enumCodeValue] == keyAELaunchedAsLogInItem) {
		launchedAtLogin = 1;
	} else if (launchedAtLogin != 1) {
		launchedAtLogin = 0;
	}
}

__attribute__((constructor)) static void watchLaunch(void) {
	@autoreleasepool {
		NSNotificationCenter *nc = [NSNotificationCenter defaultCenter];
		[nc addObserverForName:NSApplicationWillFinishLaunchingNotification object:nil queue:nil
			usingBlock:^(NSNotification *n) { readLaunchEvent(); }];
		[nc addObserverForName:NSApplicationDidFinishLaunchingNotification object:nil queue:nil
			usingBlock:^(NSNotification *n) { readLaunchEvent(); }];
	}
}

static int getLaunchedAtLogin(void) { return launchedAtLogin; }

// With the window closed the app lives in the menu bar only: no Dock icon,
// not in ⌘-Tab. Showing the window brings both back and the app forward.
static void setInDock(int show) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp setActivationPolicy:show ? NSApplicationActivationPolicyRegular : NSApplicationActivationPolicyAccessory];
		if (show) {
			[NSApp activateIgnoringOtherApps:YES];
		}
	});
}
*/
import "C"

import "time"

// launchedAtLogin reports whether macOS started the app as a login item,
// waiting up to a second for the launch to have been read.
func launchedAtLogin() bool {
	for i := 0; i < 20 && C.getLaunchedAtLogin() < 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	return C.getLaunchedAtLogin() == 1
}

func setInDock(show bool) {
	v := C.int(0)
	if show {
		v = 1
	}
	C.setInDock(v)
}
