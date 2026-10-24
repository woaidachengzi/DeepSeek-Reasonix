//go:build darwin && cgo

#import "native_pointer_darwin.h"
#import <AppKit/AppKit.h>
#include <stdatomic.h>
#include <math.h>

CGWindowID RXPointerWindowAtPoint(CGPoint point) {
    if (!isfinite(point.x) || !isfinite(point.y)) return kCGNullWindowID;
    // A Go sidecar has no NSApplication. NSApplicationLoad is AppKit's public
    // startup entry for non-Cocoa hosts; unlike sharedApplication, it keeps a
    // headless host's activation policy prohibited. No windows or event loop.
    static dispatch_once_t startup;
    static BOOL loaded=NO;
    dispatch_once(&startup,^{loaded=NSApp!=nil || NSApplicationLoad();});
    if (!loaded) return kCGNullWindowID;
    // AppKit uses a bottom-left primary-display origin; CG/AX use top-left.
    NSPoint location=NSMakePoint(point.x,CGRectGetMaxY(CGDisplayBounds(CGMainDisplayID()))-point.y);
    NSInteger hit=[NSWindow windowNumberAtPoint:location belowWindowWithWindowNumber:0];
    return hit>0 && hit<=UINT32_MAX?(CGWindowID)hit:kCGNullWindowID;
}

BOOL RXPointerWindowMatches(NSArray<NSDictionary *> *windows,pid_t pid,CGWindowID window,CGPoint point,CGWindowID mouseTarget) {
    if (!isfinite(point.x) || !isfinite(point.y) || pid<=0 || !window || mouseTarget!=window) return NO;
    // A rectangle is not an input hit test: Dock and click-through overlays
    // can cover an entire display. Require the actual mouse-down target,
    // then independently verify its owner, layer and bounds. No app allowlist.
    for (NSDictionary *candidate in windows) {
        if ([candidate[(id)kCGWindowNumber] unsignedIntValue]!=window) continue;
        id owner=candidate[(id)kCGWindowOwnerPID],layer=candidate[(id)kCGWindowLayer];
        if (![owner isKindOfClass:NSNumber.class] || ![layer isKindOfClass:NSNumber.class]) return NO;
        id alpha=candidate[(id)kCGWindowAlpha];
        if ([alpha isKindOfClass:NSNumber.class] && [alpha doubleValue]<=0) return NO;
        CGRect bounds;
        id value=candidate[(id)kCGWindowBounds];
        if (![value isKindOfClass:NSDictionary.class] || !CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)value,&bounds)) return NO;
        if (!CGRectContainsPoint(bounds,point)) return NO;
        return [owner intValue]==pid && [layer intValue]==0;
    }
    return NO;
}

BOOL RXCreateWindowServerClick(CGPoint point,CGEventRef *move,CGEventRef *down,CGEventRef *up) {
    *move=NULL;*down=NULL;*up=NULL;
    if (!isfinite(point.x) || !isfinite(point.y)) return NO;
    CGEventSourceRef source=CGEventSourceCreate(kCGEventSourceStatePrivate);
    if (!source) return NO;
    CGEventSourceSetLocalEventsSuppressionInterval(source,0);
    *move=CGEventCreateMouseEvent(source,kCGEventMouseMoved,point,kCGMouseButtonLeft);
    *down=CGEventCreateMouseEvent(source,kCGEventLeftMouseDown,point,kCGMouseButtonLeft);
    *up=CGEventCreateMouseEvent(source,kCGEventLeftMouseUp,point,kCGMouseButtonLeft);
    CFRelease(source);
    if (!*move || !*down || !*up) {
        if (*move)CFRelease(*move);if (*down)CFRelease(*down);if (*up)CFRelease(*up);
        *move=NULL;*down=NULL;*up=NULL;return NO;
    }
    static _Atomic uint64_t nextNumber=0;
    int64_t number=(int64_t)(atomic_fetch_add(&nextNumber,1)%INT64_MAX)+1;
    CGEventSetFlags(*move,0);CGEventSetFlags(*down,0);CGEventSetFlags(*up,0);
    CGEventSetIntegerValueField(*down,kCGMouseEventClickState,1);
    CGEventSetIntegerValueField(*up,kCGMouseEventClickState,1);
    CGEventSetIntegerValueField(*down,kCGMouseEventNumber,number);
    CGEventSetIntegerValueField(*up,kCGMouseEventNumber,number);
    CGEventSetDoubleValueField(*down,kCGMouseEventPressure,1);
    CGEventSetDoubleValueField(*up,kCGMouseEventPressure,0);
    return YES;
}

NSString *RXPostWindowServerClick(pid_t pid,CGWindowID window,CGRect bounds,CGPoint point,RXPointerPreflight preflight,RXPointerPost post) {
    if (!preflight || !post || !CGRectContainsPoint(bounds,point)) return @"pointer_target_unverified";
    CGEventRef move=NULL,down=NULL,up=NULL;
    if (!RXCreateWindowServerClick(point,&move,&down,&up)) return @"mouse_event_allocation_failed";
    // Preflight is deliberately the last potentially blocking operation. All
    // events exist before any post; a refusal must deliver zero input events.
    NSString *blocked=preflight(pid,window,bounds,point);
    if (!blocked) {
        post(kCGHIDEventTap,move);post(kCGHIDEventTap,down);post(kCGHIDEventTap,up);
    }
    CFRelease(move);CFRelease(down);CFRelease(up);
    return blocked;
}
