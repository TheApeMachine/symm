//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Metal -framework Foundation
#import <Metal/Metal.h>
#import <Foundation/Foundation.h>
#import <dispatch/dispatch.h>
#import <string.h>

static char symm_metallib_err[512];

static const char* symm_metallib_load_error(const void* bytes, size_t length) {
	symm_metallib_err[0] = '\0';

	if (bytes == NULL || length == 0) {
		strncpy(symm_metallib_err, "empty metallib payload", sizeof(symm_metallib_err) - 1);
		return symm_metallib_err;
	}

	id<MTLDevice> device = MTLCreateSystemDefaultDevice();

	if (device == nil) {
		strncpy(symm_metallib_err, "Metal device unavailable", sizeof(symm_metallib_err) - 1);
		return symm_metallib_err;
	}

	dispatch_data_t data = dispatch_data_create(
		bytes,
		length,
		nil,
		DISPATCH_DATA_DESTRUCTOR_DEFAULT
	);

	if (data == nil) {
		strncpy(symm_metallib_err, "failed to create dispatch_data", sizeof(symm_metallib_err) - 1);
		return symm_metallib_err;
	}

	NSError* error = nil;
	id<MTLLibrary> library = [device newLibraryWithData:data error:&error];

	if (library != nil) {
		return NULL;
	}

	if (error != nil) {
		const char* message = [[error localizedDescription] UTF8String];

		if (message != NULL) {
			strncpy(symm_metallib_err, message, sizeof(symm_metallib_err) - 1);
			return symm_metallib_err;
		}
	}

	strncpy(symm_metallib_err, "failed to load metallib", sizeof(symm_metallib_err) - 1);
	return symm_metallib_err;
}
*/
import "C"

import (
	"unsafe"
)

func metallibLoadError(payload []byte) string {
	if len(payload) == 0 {
		return "empty metallib payload"
	}

	cErr := C.symm_metallib_load_error(unsafe.Pointer(&payload[0]), C.size_t(len(payload)))

	if cErr == nil {
		return ""
	}

	return C.GoString(cErr)
}
