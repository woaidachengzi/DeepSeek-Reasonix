#import "native_pointer_darwin.h"
#import "native_capture_darwin.h"
#import <AppKit/AppKit.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>

#define CHECK(value) do { if (!(value)) { fprintf(stderr,"pointer assertion failed at line %d\n",__LINE__);exit(1); } } while (0)

static NSDictionary *window(pid_t pid,CGWindowID number,CGRect bounds,int layer,double alpha) {
    return @{(id)kCGWindowOwnerPID:@(pid),(id)kCGWindowNumber:@(number),
             (id)kCGWindowBounds:CFBridgingRelease(CGRectCreateDictionaryRepresentation(bounds)),
             (id)kCGWindowLayer:@(layer),(id)kCGWindowAlpha:@(alpha)};
}

static int checks=0,posts=0;
static NSString *refusal=nil;
static CGPoint expected;
static NSString *preflight(pid_t pid,CGWindowID number,CGRect bounds,CGPoint point) {
    checks++;
    CHECK(pid==42 && number==11 && CGRectContainsPoint(bounds,point));
    CHECK(CGPointEqualToPoint(point,expected));
    return refusal;
}
static void post(CGEventTapLocation tap,CGEventRef event) {
    CHECK(checks==1 && refusal==nil && tap==kCGHIDEventTap);
    CHECK(CGPointEqualToPoint(CGEventGetLocation(event),expected));
    CHECK(CGEventGetFlags(event)==0);
    CGEventType types[]={kCGEventMouseMoved,kCGEventLeftMouseDown,kCGEventLeftMouseUp};
    CHECK(posts<3 && CGEventGetType(event)==types[posts]);
    if (posts>0) {
        CHECK(CGEventGetIntegerValueField(event,kCGMouseEventClickState)==1);
        CHECK(CGEventGetIntegerValueField(event,kCGMouseEventButtonNumber)==kCGMouseButtonLeft);
        CHECK(CGEventGetIntegerValueField(event,kCGMouseEventNumber)>0);
    }
    posts++;
}
static void routingTests(void) {
    CGRect main=CGRectMake(0,30,1920,983),left=CGRectMake(-1920,30,1920,983);
    CGPoint p=CGPointMake(117,195),negative=CGPointMake(-1803,195);
    NSDictionary *target=window(42,11,main,0,1);
    CHECK(RXPointerWindowMatches(@[target],42,11,p,11));
    CHECK(RXPointerWindowMatches(@[window(42,11,left,0,1)],42,11,negative,11));
    CHECK(!RXPointerWindowMatches(@[target],42,11,negative,11));
    // Actual input-covering windows block regardless of owner or layer.
    CHECK(!RXPointerWindowMatches(@[window(99,12,main,0,1),target],42,11,p,12));
    CHECK(!RXPointerWindowMatches(@[window(42,12,main,0,1),target],42,11,p,12));
    CHECK(!RXPointerWindowMatches(@[window(99,12,main,25,1),target],42,11,p,12));
    // A click-through surface can have alpha 1 and cover the entire screen.
    CHECK(RXPointerWindowMatches(@[window(99,12,main,20,1),target],42,11,p,11));
    CHECK(RXPointerWindowMatches(@[window(99,12,main,3,1),target],42,11,p,11));
    CHECK(!RXPointerWindowMatches(@[target],42,11,p,0));
    CHECK(!RXPointerWindowMatches(@[window(99,11,main,0,1)],42,11,p,11));
    CHECK(!RXPointerWindowMatches(@[window(42,11,main,25,1)],42,11,p,11));
    CHECK(!RXPointerWindowMatches(@[window(42,11,main,0,0)],42,11,p,11));
    CHECK(!RXPointerWindowMatches(@[],42,11,p,11));
    CHECK(!RXPointerWindowMatches(@[@{}],42,11,p,11));
    CHECK(!RXPointerWindowMatches(@[@{(id)kCGWindowNumber:@11}],42,11,p,11));
    NSMutableDictionary *missingLayer=[target mutableCopy];[missingLayer removeObjectForKey:(id)kCGWindowLayer];
    CHECK(!RXPointerWindowMatches(@[missingLayer],42,11,p,11));
    CHECK(!RXPointerWindowMatches(@[target],42,11,CGPointMake(NAN,195),11));
    CHECK(!RXPointerWindowMatches(@[target],42,11,CGPointMake(1920,195),11));
}
static void dispatchTests(void) {
    for (NSString *reason in @[@"pointer_target_changed",@"pointer_target_unverified",@"input_permission_missing",@"pointer_busy",@"window_not_active"]) {
        checks=0;posts=0;refusal=reason;expected=CGPointMake(117,195);
        NSString *blocked=RXPostWindowServerClick(42,11,CGRectMake(0,30,1920,983),expected,preflight,post);
        CHECK([blocked isEqualToString:reason] && checks==1 && posts==0);
    }
    for (NSNumber *origin in @[@0,@(-1920)]) {
        checks=0;posts=0;refusal=nil;expected=CGPointMake(origin.doubleValue+117,195);
        CHECK(RXPostWindowServerClick(42,11,CGRectMake(origin.doubleValue,30,1920,983),expected,preflight,post)==nil);
        CHECK(checks==1 && posts==3);
    }
    checks=0;posts=0;
    CHECK(RXPostWindowServerClick(42,11,CGRectMake(0,30,1920,983),CGPointMake(-1,195),preflight,post)!=nil);
    CHECK(checks==0 && posts==0);
    CHECK(RXPostWindowServerClick(42,11,CGRectMake(0,30,1920,983),CGPointMake(117,195),NULL,post)!=nil);
    CHECK(posts==0);
    CGEventRef move=NULL,down=NULL,up=NULL;
    CHECK(!RXCreateWindowServerClick(CGPointMake(INFINITY,0),&move,&down,&up));
    CHECK(!move && !down && !up);
}
static void headlessHitTest(void) {
    CHECK(NSApp==nil);
    NSArray *windows=CFBridgingRelease(CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly|kCGWindowListExcludeDesktopElements,0));
    CGPoint point=CGPointZero;
    BOOL found=NO;
    for (NSDictionary *candidate in windows) {
        CGRect rect;
        if ([candidate[(id)kCGWindowLayer] intValue]!=0 || !CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)candidate[(id)kCGWindowBounds],&rect) || rect.size.width<100 || rect.size.height<100) continue;
        point=CGPointMake(CGRectGetMidX(rect),CGRectGetMidY(rect));found=YES;break;
    }
    if (!found) return;
    dispatch_semaphore_t done=dispatch_semaphore_create(0);
    __block CGWindowID hit=0;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_DEFAULT,0),^{
        @autoreleasepool {hit=RXPointerWindowAtPoint(point);dispatch_semaphore_signal(done);}
    });
    CHECK(dispatch_semaphore_wait(done,dispatch_time(DISPATCH_TIME_NOW,5*NSEC_PER_SEC))==0);
    CHECK(hit!=0 && NSApp!=nil && NSApp.activationPolicy==NSApplicationActivationPolicyProhibited);
    CHECK([NSApp windows].count==0 && posts==0);
}
int main(void) {
    @autoreleasepool {
        CHECK(CGSizeEqualToSize(RXCaptureSize(CGSizeMake(1686,983)),CGSizeMake(1686,983)));
        CHECK(CGSizeEqualToSize(RXCaptureSize(CGSizeMake(800,600)),CGSizeMake(800,600)));
        CHECK(CGSizeEqualToSize(RXCaptureSize(CGSizeMake(3840,2160)),CGSizeMake(1920,1080)));
        CHECK(CGSizeEqualToSize(RXCaptureSize(CGSizeMake(2160,3840)),CGSizeMake(1080,1920)));
        CHECK(CGSizeEqualToSize(RXCaptureSize(CGSizeMake(NAN,983)),CGSizeZero));
        CHECK(CGSizeEqualToSize(RXCaptureSize(CGSizeMake(1686,0)),CGSizeZero));
        CFDictionaryRef session=CGSessionCopyCurrentDictionary();
        if (!session) return 77;
        CFRelease(session);
        routingTests();dispatchTests();
        headlessHitTest();
        puts("PASS native pointer: window ownership, overlays, negative origins, preflight refusals send zero events, valid move/down/up. No real input posted.");
    }
    return 0;
}
