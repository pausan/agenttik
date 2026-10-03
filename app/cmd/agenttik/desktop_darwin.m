//go:build desktop && darwin

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
#include "_cgo_export.h"

// A Dock click on a window hidden to the tray would otherwise do nothing.
static BOOL shouldHandleReopen(id self, SEL _cmd, NSApplication *app, BOOL visible) {
	appReopen();
	return YES;
}

// Wails sends Quit through the close hook, which hides to the tray. Quit from
// the Dock or the menu is meant, so it takes the same path as Cmd+Q.
static NSApplicationTerminateReply shouldTerminate(id self, SEL _cmd, NSApplication *app) {
	appQuit();
	return NSTerminateCancel;
}

// The icon bytes are embedded in the binary and outlive the block.
void startNativeApp(const void *data, int length, int hooks) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSData *png = [NSData dataWithBytesNoCopy:(void *)data length:length freeWhenDone:NO];
		NSImage *icon = [[NSImage alloc] initWithData:png];
		if (icon != nil) {
			[NSApp setApplicationIconImage:icon];
		}
		if (!hooks) {
			return;
		}
		Class delegate = [[NSApp delegate] class];
		NSString *reopenTypes = [NSString stringWithFormat:@"%s%s%s%s%s", @encode(BOOL), @encode(id), @encode(SEL), @encode(id), @encode(BOOL)];
		class_replaceMethod(delegate, @selector(applicationShouldHandleReopen:hasVisibleWindows:), (IMP)shouldHandleReopen, [reopenTypes UTF8String]);
		Method terminate = class_getInstanceMethod(delegate, @selector(applicationShouldTerminate:));
		class_replaceMethod(delegate, @selector(applicationShouldTerminate:), (IMP)shouldTerminate, method_getTypeEncoding(terminate));
		// Wails' own Quit item skips applicationShouldTerminate:.
		SEL wailsQuit = NSSelectorFromString(@"Quit");
		for (NSMenuItem *top in [[NSApp mainMenu] itemArray]) {
			for (NSMenuItem *item in [[top submenu] itemArray]) {
				if (item.action == wailsQuit) {
					item.target = nil;
					item.action = @selector(terminate:);
				}
			}
		}
	});
}
