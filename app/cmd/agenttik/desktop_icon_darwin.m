//go:build desktop && darwin

#import <Cocoa/Cocoa.h>

// The bytes are embedded in the binary and live for its whole run, so the
// block can read them after the caller returns.
void setAppIconPNG(const void *data, int length) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSData *png = [NSData dataWithBytesNoCopy:(void *)data length:length freeWhenDone:NO];
		NSImage *icon = [[NSImage alloc] initWithData:png];
		if (icon != nil) {
			[NSApp setApplicationIconImage:icon];
		}
	});
}
