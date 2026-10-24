// Included after RXObserve and RXPointerBlock; uses the same ownership gateway
// as ordinary clicks. Movement never posts a mouse-down or mouse-up event.
#include <unistd.h>

static NSDictionary *RXPointerPreviewError(NSString *reason,BOOL moved) {
    return @{ @"ok":@NO,@"error":@"Cursor preview changed or could not be verified. No click was sent; observe and preview again.",
              @"dispatched":@(moved),@"dispatch_method":moved?@"window_server_move":@"",@"blocked_reason":reason };
}

static NSDictionary *RXMove(NSDictionary *request,pid_t pid,CGWindowID wid,CGRect bounds) {
    CGPoint target=CGPointMake(bounds.origin.x+[request[@"x"] doubleValue],bounds.origin.y+[request[@"y"] doubleValue]);
    NSString *blocked=RXPostWindowServerMove(pid,wid,bounds,target,RXPointerBlock,CGEventPost);
    if (blocked) return RXPointerPreviewError(blocked,NO);
    // WindowServer dispatch is asynchronous. Bound the read-only arrival
    // check to 100ms; never synthesize a click or replace an actual position.
    CGPoint actual=CGPointZero;BOOL arrived=NO;
    for (int attempt=0;attempt<20;attempt++) {
        usleep(5000);
        if (RXReadPointer(&actual) && RXPointerNear(actual,target)) {arrived=YES;break;}
    }
    if (!arrived) return RXPointerPreviewError(@"pointer_position_unverified",YES);
    NSMutableDictionary *result=[RXObserve(pid,wid,YES) mutableCopy];
    result[@"dispatch_method"]=@"window_server_move";
    if (![result[@"ok"] boolValue]) return result;
    CGPoint after;
    if (!RXReadPointer(&after) || !RXPointerNear(after,actual) || !RXPointerNear(after,target)) return RXPointerPreviewError(@"pointer_moved",YES);
    CGRect current;
    NSDictionary *window=RXWindow(pid,wid);
    if (!window || !CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)window[(id)kCGWindowBounds],&current) || !CGRectEqualToRect(current,bounds)) return RXPointerPreviewError(@"window_changed",YES);
    blocked=RXPointerBlock(pid,wid,bounds,after);
    if (blocked) return RXPointerPreviewError(blocked,YES);
    result[@"pointer"]=@{@"screen_x":@(after.x),@"screen_y":@(after.y)};
    return result;
}

static NSDictionary *RXConfirmClick(NSDictionary *request,pid_t pid,CGWindowID wid,CGRect bounds) {
    NSDictionary *verified=request[@"verified_pointer"];
    if (![verified isKindOfClass:NSDictionary.class] || ![verified[@"screen_x"] isKindOfClass:NSNumber.class] || ![verified[@"screen_y"] isKindOfClass:NSNumber.class]) return RXPointerPreviewError(@"pointer_preview_required",NO);
    CGPoint point=CGPointMake([verified[@"screen_x"] doubleValue],[verified[@"screen_y"] doubleValue]);
    CGPoint requested=CGPointMake(bounds.origin.x+[request[@"x"] doubleValue],bounds.origin.y+[request[@"y"] doubleValue]);
    if (!RXPointerNear(point,requested)) return RXPointerPreviewError(@"window_changed",NO);
    NSString *blocked=RXPostConfirmedWindowServerClick(pid,wid,bounds,point,RXReadPointer,RXPointerBlock,CGEventPost);
    if (blocked) return RXPointerPreviewError(blocked,NO);
    NSMutableDictionary *result=[RXObserve(pid,wid,YES) mutableCopy];
    result[@"dispatch_method"]=@"window_server_mouse";
    return result;
}
