//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static int trayFontConfigured = 0;

static NSButton *findStatusBarButton(NSView *view, Class btnClass) {
	if ([view isKindOfClass:btnClass]) {
		return (NSButton *)view;
	}
	for (NSView *sub in view.subviews) {
		NSButton *found = findStatusBarButton(sub, btnClass);
		if (found) {
			return found;
		}
	}
	return nil;
}

// ensureTrayMonospacedFont switches the macOS NSStatusBarButton font to
// tabular/monospaced digits so the menu bar countdown does not change width
// every second and shift neighboring status items.
static void ensureTrayMonospacedFont(void) {
	if (__atomic_load_n(&trayFontConfigured, __ATOMIC_ACQUIRE)) {
		return;
	}
	dispatch_async(dispatch_get_main_queue(), ^{
		if (__atomic_load_n(&trayFontConfigured, __ATOMIC_RELAXED)) {
			return;
		}
		Class statusBtnClass = NSClassFromString(@"NSStatusBarButton");
		if (!statusBtnClass) {
			statusBtnClass = [NSButton class];
		}
		for (NSWindow *win in [NSApp windows]) {
			if (![NSStringFromClass([win class]) containsString:@"StatusBar"]) {
				continue;
			}
			NSButton *btn = findStatusBarButton(win.contentView, statusBtnClass);
			if (!btn) {
				continue;
			}
			NSFont *base = btn.font ?: [NSFont menuBarFontOfSize:0];
			NSFontDescriptor *desc = [base.fontDescriptor fontDescriptorByAddingAttributes:@{
				NSFontFeatureSettingsAttribute: @[
					@{
						NSFontFeatureTypeIdentifierKey: @(kNumberSpacingType),
						NSFontFeatureSelectorIdentifierKey: @(kMonospacedNumbersSelector)
					}
				]
			}];
			NSFont *mono = [NSFont fontWithDescriptor:desc size:base.pointSize];
			if (!mono) {
				mono = [NSFont monospacedDigitSystemFontOfSize:base.pointSize weight:NSFontWeightRegular];
			}
			if (mono) {
				btn.font = mono;
			}
			__atomic_store_n(&trayFontConfigured, 1, __ATOMIC_RELEASE);
			break;
		}
	});
}
*/
import "C"

func ensureTrayMonospacedFont() {
	C.ensureTrayMonospacedFont()
}
