//go:build desktop && darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdbool.h>

static bool AppIsFrontmost(void)
{
	return [[NSRunningApplication currentApplication] isActive];
}
*/
import "C"

import "golang.design/x/hotkey"

const altModifier = hotkey.ModOption

// appIsForeground reports whether this application is the active one, which is
// what macOS calls the app whose window the desktop has in front.
func appIsForeground() bool { return bool(C.AppIsFrontmost()) }
