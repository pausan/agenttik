//go:build desktop && darwin

#import <Carbon/Carbon.h>
#import <Cocoa/Cocoa.h>
#include "_cgo_export.h"

bool appIsFrontmost(void) {
	return [[NSRunningApplication currentApplication] isActive];
}

static OSStatus onHotKey(EventHandlerCallRef next, EventRef event, void *data) {
	EventHotKeyID hotKey;
	if (GetEventParameter(event, kEventParamDirectObject, typeEventHotKeyID, NULL, sizeof hotKey, NULL, &hotKey) != noErr) {
		return eventNotHandledErr;
	}
	hotkeyEvent(hotKey.id, GetEventKind(event) == kEventHotKeyPressed);
	return noErr;
}

// Carbon hot keys belong to the main thread. The tray registers from there,
// before the event loop runs, so waiting on the main queue would never return.
static void onMainThread(dispatch_block_t block) {
	if ([NSThread isMainThread]) {
		block();
	} else {
		dispatch_sync(dispatch_get_main_queue(), block);
	}
}

void *registerHotKey(uint32_t code, uint32_t modifiers, uint32_t id, int *status) {
	__block EventHotKeyRef ref = NULL;
	__block OSStatus result = noErr;
	onMainThread(^{
		static EventHandlerRef handler;
		if (handler == NULL) {
			EventTypeSpec types[] = {
				{kEventClassKeyboard, kEventHotKeyPressed},
				{kEventClassKeyboard, kEventHotKeyReleased},
			};
			result = InstallApplicationEventHandler(NewEventHandlerUPP(onHotKey), 2, types, NULL, &handler);
			if (result != noErr) {
				return;
			}
		}
		EventHotKeyID hotKey = {'agtk', id};
		result = RegisterEventHotKey(code, modifiers, hotKey, GetApplicationEventTarget(), 0, &ref);
	});
	*status = result;
	return ref;
}

void unregisterHotKey(void *ref) {
	onMainThread(^{
		UnregisterEventHotKey((EventHotKeyRef)ref);
	});
}
