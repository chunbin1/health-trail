//go:build darwin

package folders

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
#include <stdlib.h>

// makeBookmark returns a security-scoped bookmark for path (malloc'd, *len
// bytes), or NULL with *err set. In the App Sandbox the folder must be one
// the person just picked in an open dialog: that is what grants the access
// the bookmark keeps.
static void *makeBookmark(const char *path, int *len, char **err) {
	@autoreleasepool {
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
		NSError *e = nil;
		NSData *d = [url bookmarkDataWithOptions:NSURLBookmarkCreationWithSecurityScope
			includingResourceValuesForKeys:nil relativeToURL:nil error:&e];
		if (!d) {
			*err = strdup(e ? [[e localizedDescription] UTF8String] : "bookmark failed");
			return NULL;
		}
		*len = (int)[d length];
		void *out = malloc(*len);
		memcpy(out, [d bytes], *len);
		return out;
	}
}

// openBookmark resolves a security-scoped bookmark and starts using its
// access, for the rest of the process. It returns the folder's path
// (malloc'd) or NULL with *err set; *stale says the bookmark should be made
// again.
static char *openBookmark(const void *data, int len, int *stale, char **err) {
	@autoreleasepool {
		NSData *d = [NSData dataWithBytes:data length:len];
		BOOL isStale = NO;
		NSError *e = nil;
		NSURL *url = [NSURL URLByResolvingBookmarkData:d options:NSURLBookmarkResolutionWithSecurityScope
			relativeToURL:nil bookmarkDataIsStale:&isStale error:&e];
		if (!url) {
			*err = strdup(e ? [[e localizedDescription] UTF8String] : "resolve failed");
			return NULL;
		}
		*stale = isStale ? 1 : 0;
		if (![url startAccessingSecurityScopedResource]) {
			*err = strdup("no access granted");
			return NULL;
		}
		return strdup([[url path] UTF8String]);
	}
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

func makeBookmark(path string) ([]byte, error) {
	cp := C.CString(path)
	defer C.free(unsafe.Pointer(cp))
	var n C.int
	var cerr *C.char
	p := C.makeBookmark(cp, &n, &cerr)
	if p == nil {
		defer C.free(unsafe.Pointer(cerr))
		return nil, errors.New(C.GoString(cerr))
	}
	defer C.free(p)
	return C.GoBytes(p, n), nil
}

func openBookmark(b []byte) (path string, stale bool, err error) {
	if len(b) == 0 {
		return "", false, errors.New("empty bookmark")
	}
	cb := C.CBytes(b)
	defer C.free(cb)
	var st C.int
	var cerr *C.char
	p := C.openBookmark(cb, C.int(len(b)), &st, &cerr)
	if p == nil {
		defer C.free(unsafe.Pointer(cerr))
		return "", false, errors.New(C.GoString(cerr))
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p), st == 1, nil
}
