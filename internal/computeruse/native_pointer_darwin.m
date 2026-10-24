//go:build darwin && cgo

#import "native_pointer_darwin.h"
#include <stdatomic.h>
#include <math.h>

BOOL RXPointerWindowMatches(NSArray<NSDictionary *> *windows,pid_t pid,CGWindowID window,CGPoint point) {
    if (!isfinite(point.x) || !isfinite(point.y) || pid<=0 || !window) return NO;
    // WindowServer returns these front to back. A window from another app,
    // another window of the same app, or an overlay must block global input.
    for (NSDictionary *candidate in windows) {
        id alpha=candidate[(id)kCGWindowAlpha];
        if ([alpha isKindOfClass:NSNumber.class] && [alpha doubleValue]<=0) continue;
        CGRect bounds;
        id value=candidate[(id)kCGWindowBounds];
        if (![value isKindOfClass:NSDictionary.class] || !CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)value,&bounds)) return NO;
        if (!CGRectContainsPoint(bounds,point)) continue;
        return [candidate[(id)kCGWindowOwnerPID] intValue]==pid &&
               [candidate[(id)kCGWindowNumber] unsignedIntValue]==window &&
               [candidate[(id)kCGWindowLayer] intValue]==0;
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
