//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Speech -framework AVFoundation -framework Foundation -framework CoreFoundation
#include <stdlib.h>
#include <unistd.h>
#include <CoreFoundation/CoreFoundation.h>
#import <AVFoundation/AVFoundation.h>
#import <Speech/Speech.h>

static NSLock *gLock;
static NSString *gText;
static int gErrCode;
static int gRunning;
static int gStopping;
static AVAudioEngine *gEngine;
static SFSpeechAudioBufferRecognitionRequest *gRequest;
static SFSpeechRecognitionTask *gTask;
static SFSpeechRecognizer *gRec;

__attribute__((constructor)) static void speechInit(void) {
	gLock = [[NSLock alloc] init];
	gText = @"";
}

static void setText(NSString *s) {
	if (!s) s = @"";
	[gLock lock];
	gText = [s copy];
	[gLock unlock];
}

static void setErr(int code) {
	[gLock lock];
	if (gErrCode == 0) gErrCode = code;
	[gLock unlock];
}

static char *copyText(void) {
	[gLock lock];
	NSString *s = gText ?: @"";
	char *out = strdup(s.UTF8String ? s.UTF8String : "");
	[gLock unlock];
	return out;
}

static int lastErr(void) {
	[gLock lock];
	int c = gErrCode;
	[gLock unlock];
	return c;
}

// wait runs fn and waits until it sets *done, pumping the main run loop
// when we are already on it so the permission prompt can appear.
static int waitDone(void (^fn)(int *done, int *result)) {
	__block int done = 0;
	__block int result = 0;
	fn(&done, &result);
	for (int i = 0; i < 1200 && !done; i++) {
		if ([NSThread isMainThread]) CFRunLoopRunInMode(kCFRunLoopDefaultMode, 0.05, false);
		else usleep(50000);
	}
	return result;
}

static int ensureAuth(void) {
	SFSpeechRecognizerAuthorizationStatus st = [SFSpeechRecognizer authorizationStatus];
	if (st == SFSpeechRecognizerAuthorizationStatusNotDetermined) {
		st = (SFSpeechRecognizerAuthorizationStatus)waitDone(^(int *done, int *result) {
			[SFSpeechRecognizer requestAuthorization:^(SFSpeechRecognizerAuthorizationStatus s) {
				*result = (int)s;
				*done = 1;
			}];
		});
	}
	if (st != SFSpeechRecognizerAuthorizationStatusAuthorized) {
		return 1;
	}
	AVAuthorizationStatus mic = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeAudio];
	if (mic == AVAuthorizationStatusNotDetermined) {
		int granted = waitDone(^(int *done, int *result) {
			[AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio completionHandler:^(BOOL ok) {
				*result = ok ? 1 : 0;
				*done = 1;
			}];
		});
		mic = granted ? AVAuthorizationStatusAuthorized : AVAuthorizationStatusDenied;
	}
	if (mic != AVAuthorizationStatusAuthorized) {
		return 2;
	}
	return 0;
}

static void teardown(void) {
	gStopping = 1;
	if (gEngine) {
		if (gEngine.isRunning) [gEngine stop];
		AVAudioInputNode *input = nil;
		@try { input = gEngine.inputNode; } @catch (NSException *e) { input = nil; }
		if (input) {
			@try { [input removeTapOnBus:0]; } @catch (NSException *e) {}
		}
	}
	if (gRequest) {
		[gRequest endAudio];
		gRequest = nil;
	}
	if (gTask) {
		[gTask cancel];
		gTask = nil;
	}
	gEngine = nil;
	gRec = nil;
	gRunning = 0;
}

static int startEngine(const char *locale) {
	if (gRunning) return 4;
	gStopping = 0;
	[gLock lock];
	gErrCode = 0;
	gText = @"";
	[gLock unlock];

	NSString *loc = locale ? [NSString stringWithUTF8String:locale] : @"zh-CN";
	gRec = [[SFSpeechRecognizer alloc] initWithLocale:[NSLocale localeWithLocaleIdentifier:loc]];
	if (!gRec || !gRec.supportsOnDeviceRecognition) {
		gRec = nil;
		return 3;
	}
	gRequest = [[SFSpeechAudioBufferRecognitionRequest alloc] init];
	gRequest.shouldReportPartialResults = YES;
	gRequest.requiresOnDeviceRecognition = YES;
	gRequest.taskHint = SFSpeechRecognitionTaskHintDictation;

	gEngine = [[AVAudioEngine alloc] init];
	AVAudioInputNode *input = nil;
	@try {
		input = gEngine.inputNode;
	} @catch (NSException *e) {
		teardown();
		return 2;
	}
	AVAudioFormat *fmt = [input outputFormatForBus:0];
	if (fmt.sampleRate == 0 || fmt.channelCount == 0) {
		teardown();
		return 5;
	}
	gTask = [gRec recognitionTaskWithRequest:gRequest resultHandler:^(SFSpeechRecognitionResult *result, NSError *error) {
		if (gStopping) return;
		if (result != nil && result.bestTranscription.formattedString.length > 0) {
			setText(result.bestTranscription.formattedString);
		}
		if (error) {
			setErr(5);
			gRunning = 0;
		}
	}];
	@try {
		[input installTapOnBus:0 bufferSize:1024 format:fmt block:^(AVAudioPCMBuffer *buffer, AVAudioTime *when) {
			if (gRequest && !gStopping) [gRequest appendAudioPCMBuffer:buffer];
		}];
	} @catch (NSException *e) {
		teardown();
		return 5;
	}
	[gEngine prepare];
	NSError *err = nil;
	if (![gEngine startAndReturnError:&err]) {
		teardown();
		return 5;
	}
	gRunning = 1;
	return 0;
}

static int speechStart(const char *locale) {
	int auth = ensureAuth();
	if (auth != 0) return auth;
	if ([NSThread isMainThread]) return startEngine(locale);
	__block int code = 0;
	dispatch_sync(dispatch_get_main_queue(), ^{
		code = startEngine(locale);
	});
	return code;
}

static void speechStop(void) {
	void (^stop)(void) = ^{
		teardown();
	};
	if ([NSThread isMainThread]) stop();
	else dispatch_sync(dispatch_get_main_queue(), stop);
}

static int speechListening(void) { return gRunning; }
static char *speechText(void) { return copyText(); }
static int speechError(void) { return lastErr(); }
*/
import "C"
import "unsafe"

type macEar struct{}

func platformEar() Ear { return macEar{} }

func (macEar) Available() bool { return true }

func (macEar) Start(locale string) error {
	c := C.CString(locale)
	defer C.free(unsafe.Pointer(c))
	if code := int(C.speechStart(c)); code != 0 {
		return speechFailure{Code: code}
	}
	return nil
}

func (macEar) Stop() (string, error) {
	C.speechStop()
	return cString(C.speechText()), nil
}

func (macEar) View() SpeechView {
	return SpeechView{
		Listening: C.speechListening() != 0,
		Text:      cString(C.speechText()),
		ErrCode:   int(C.speechError()),
	}
}

func cString(p *C.char) string {
	if p == nil {
		return ""
	}
	s := C.GoString(p)
	C.free(unsafe.Pointer(p))
	return s
}
