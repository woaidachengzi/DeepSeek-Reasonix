//go:build darwin && cgo

#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <ImageIO/ImageIO.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CommonCrypto/CommonDigest.h>
#include <stdlib.h>
#include <dlfcn.h>
#include <string.h>
#import "native_pointer_darwin.h"

static NSDictionary *RXError(NSString *message, BOOL dispatched) {
    return @{ @"ok":@NO, @"error":message, @"dispatched":@(dispatched) };
}
static id RXAttribute(AXUIElementRef element, CFStringRef name) {
    CFTypeRef value=NULL;
    if (!element || AXUIElementCopyAttributeValue(element,name,&value)!=kAXErrorSuccess) return nil;
    return CFBridgingRelease(value);
}
static NSString *RXText(id value) {
    if (![value isKindOfClass:NSString.class]) return @"";
    NSString *s=value;
    return s.length>512 ? [s substringToIndex:512] : s;
}
static CGRect RXRect(AXUIElementRef element) {
    id p=RXAttribute(element,kAXPositionAttribute),s=RXAttribute(element,kAXSizeAttribute);
    CGPoint point=CGPointZero; CGSize size=CGSizeZero;
    if (p && CFGetTypeID((__bridge CFTypeRef)p)==AXValueGetTypeID()) AXValueGetValue((__bridge AXValueRef)p,kAXValueCGPointType,&point);
    if (s && CFGetTypeID((__bridge CFTypeRef)s)==AXValueGetTypeID()) AXValueGetValue((__bridge AXValueRef)s,kAXValueCGSizeType,&size);
    return (CGRect){point,size};
}
static NSDictionary *RXBounds(CGRect r) {
    return @{ @"x":@(r.origin.x),@"y":@(r.origin.y),@"width":@(r.size.width),@"height":@(r.size.height) };
}
static NSString *RXValueHash(NSString *text) {
    NSData *data=[text dataUsingEncoding:NSUTF8StringEncoding];
    if (!data || data.length>UINT32_MAX) return @"";
    unsigned char hash[CC_SHA256_DIGEST_LENGTH];CC_SHA256(data.bytes,(CC_LONG)data.length,hash);
    NSMutableString *hex=[NSMutableString stringWithCapacity:CC_SHA256_DIGEST_LENGTH*2];
    for (NSUInteger i=0;i<CC_SHA256_DIGEST_LENGTH;i++) [hex appendFormat:@"%02x",hash[i]];
    return hex;
}
static NSDictionary *RXFocus(pid_t pid) {
    AXUIElementRef app=AXUIElementCreateApplication(pid);AXUIElementSetMessagingTimeout(app,0.05);
    id focused=RXAttribute(app,kAXFocusedUIElementAttribute);CFRelease(app);
    if (!focused || CFGetTypeID((__bridge CFTypeRef)focused)!=AXUIElementGetTypeID()) return @{};
    AXUIElementRef e=(__bridge AXUIElementRef)focused;
    NSString *role=RXText(RXAttribute(e,kAXRoleAttribute)),*subrole=RXText(RXAttribute(e,kAXSubroleAttribute));
    BOOL secure=[subrole isEqualToString:@"AXSecureTextField"];
    id rawValue=secure?nil:RXAttribute(e,kAXValueAttribute);
    NSMutableDictionary *result=[@{@"role":role,@"subrole":subrole,@"identifier":RXText(RXAttribute(e,kAXIdentifierAttribute)),@"bounds":RXBounds(RXRect(e)),@"value":secure?@"[secure]":RXText(rawValue)} mutableCopy];
    // An unavailable AX value is not an empty field. Keep it unknown so a
    // caret-only exception or readback cannot rely on an invented empty value.
    if (!secure && ![rawValue isKindOfClass:NSString.class]) [result removeObjectForKey:@"value"];
    if (!secure && ([role isEqualToString:@"AXTextField"] || [role isEqualToString:@"AXTextArea"]) && [rawValue isKindOfClass:NSString.class]) {
        result[@"value_hash"]=RXValueHash(rawValue);result[@"value_length"]=@([rawValue length]);
    }
    if (![role isEqualToString:@"AXTextField"] && ![role isEqualToString:@"AXTextArea"]) return result;
    id selected=RXAttribute(e,kAXSelectedTextRangeAttribute);CFRange range;
    if (!selected || CFGetTypeID((__bridge CFTypeRef)selected)!=AXValueGetTypeID() || !AXValueGetValue((__bridge AXValueRef)selected,kAXValueCFRangeType,&range)) return result;
    result[@"selection"]=@{@"location":@(range.location),@"length":@(range.length)};
    if (range.length!=0) return result;
    AXValueRef parameter=AXValueCreate(kAXValueCFRangeType,&range);CFTypeRef value=NULL;
    AXError error=AXUIElementCopyParameterizedAttributeValue(e,kAXBoundsForRangeParameterizedAttribute,parameter,&value);CFRelease(parameter);
    if (error==kAXErrorSuccess && value) {
        CGRect caret;
        if (CFGetTypeID(value)==AXValueGetTypeID() && AXValueGetValue((AXValueRef)value,kAXValueCGRectType,&caret) && caret.size.width>=0 && caret.size.width<=4 && caret.size.height>0 && caret.size.height<=100) result[@"caret"]=RXBounds(caret);
    }
    if(value)CFRelease(value);
    return result;
}
static void RXTree(AXUIElementRef e,NSString *path,NSUInteger depth,NSMutableArray *items,double deadline,BOOL *truncated) {
    if (depth>7 || items.count>=100 || CFAbsoluteTimeGetCurrent()>deadline) {*truncated=YES;return;}
    NSString *role=RXText(RXAttribute(e,kAXRoleAttribute));
    NSString *subrole=RXText(RXAttribute(e,kAXSubroleAttribute));
    // Do not ask the accessibility server for a secure field's value.
    NSString *value=[subrole isEqualToString:@"AXSecureTextField"] ? @"[secure]" : RXText(RXAttribute(e,kAXValueAttribute));
    [items addObject:@{ @"element":path,@"role":role,@"title":RXText(RXAttribute(e,kAXTitleAttribute)),
        @"description":RXText(RXAttribute(e,kAXDescriptionAttribute)),@"value":value,@"bounds":RXBounds(RXRect(e)) }];
    id children=RXAttribute(e,kAXChildrenAttribute);
    if (![children isKindOfClass:NSArray.class]) return;
    NSUInteger index=0;
    for (id child in children) {
        if (items.count>=100 || CFAbsoluteTimeGetCurrent()>deadline) {*truncated=YES;break;}
        if (CFGetTypeID((__bridge CFTypeRef)child)==AXUIElementGetTypeID()) RXTree((__bridge AXUIElementRef)child,[path stringByAppendingFormat:@".%lu",(unsigned long)index],depth+1,items,deadline,truncated);
        index++;
    }
}
static AXUIElementRef RXWindowAX(pid_t pid,CGRect bounds) {
    AXUIElementRef app=AXUIElementCreateApplication(pid);
    AXUIElementSetMessagingTimeout(app,0.05);
    id window=RXAttribute(app,kAXFocusedWindowAttribute);
    CFRelease(app);
    if (!window || CFGetTypeID((__bridge CFTypeRef)window)!=AXUIElementGetTypeID()) return NULL;
    AXUIElementRef e=(__bridge AXUIElementRef)window;
    CGRect r=RXRect(e);
    if (fabs(r.origin.x-bounds.origin.x)>3 || fabs(r.origin.y-bounds.origin.y)>3 || fabs(r.size.width-bounds.size.width)>6 || fabs(r.size.height-bounds.size.height)>6) return NULL;
    return (AXUIElementRef)CFRetain(e);
}
static NSDictionary *RXWindow(pid_t pid,CGWindowID wanted) {
    NSArray *windows=CFBridgingRelease(CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly|kCGWindowListExcludeDesktopElements,kCGNullWindowID));
    for (NSDictionary *window in windows) {
        if ([window[(id)kCGWindowOwnerPID] intValue]!=pid || [window[(id)kCGWindowLayer] intValue]!=0) continue;
        if (wanted && [window[(id)kCGWindowNumber] unsignedIntValue]!=wanted) continue;
        CGRect r; if (!CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)window[(id)kCGWindowBounds],&r) || r.size.width<=0 || r.size.height<=0) continue;
        return window;
    }
    return nil;
}
static NSString *RXEncode(CGImageRef image) {
    if (!image) return nil;
    NSBitmapImageRep *rep=[[NSBitmapImageRep alloc] initWithCGImage:image];
    NSData *data=[rep representationUsingType:NSBitmapImageFileTypeJPEG properties:@{NSImageCompressionFactor:@0.75}];
    if (!data || data.length>3*1024*1024) return nil;
    return [@"data:image/jpeg;base64," stringByAppendingString:[data base64EncodedStringWithOptions:0]];
}
static NSString *RXCapture(CGWindowID window) {
    if (@available(macOS 14.0,*)) {
        dispatch_semaphore_t done=dispatch_semaphore_create(0);
        __block NSString *encoded=nil;
        [(id)NSClassFromString(@"SCShareableContent") getShareableContentExcludingDesktopWindows:YES onScreenWindowsOnly:NO completionHandler:^(SCShareableContent *content,NSError *error) {
            if (error || !content) {dispatch_semaphore_signal(done);return;}
            SCWindow *selected=nil;
            for (SCWindow *candidate in content.windows) if (candidate.windowID==window) {selected=candidate;break;}
            if (!selected) {dispatch_semaphore_signal(done);return;}
            SCContentFilter *filter=[[NSClassFromString(@"SCContentFilter") alloc] initWithDesktopIndependentWindow:selected];
            SCStreamConfiguration *config=[NSClassFromString(@"SCStreamConfiguration") new];
            double width=selected.frame.size.width,height=selected.frame.size.height;
            double scale=fmin(2.0,1920.0/fmax(width,height));
            config.width=(size_t)fmax(1,width*scale); config.height=(size_t)fmax(1,height*scale);
            config.showsCursor=NO;config.ignoreShadowsSingleWindow=YES;
            [(id)NSClassFromString(@"SCScreenshotManager") captureImageWithFilter:filter configuration:config completionHandler:^(CGImageRef image,NSError *captureError) {
                if (!captureError) encoded=RXEncode(image);
                dispatch_semaphore_signal(done);
            }];
        }];
        if (dispatch_semaphore_wait(done,dispatch_time(DISPATCH_TIME_NOW,8*NSEC_PER_SEC))!=0) return nil;
        return encoded;
    }
    return nil;
}
int reasonix_computer_use_available(void) {
    if (@available(macOS 14.0,*)) {
        static dispatch_once_t once;
        static void *framework;
        dispatch_once(&once,^{framework=dlopen("/System/Library/Frameworks/ScreenCaptureKit.framework/ScreenCaptureKit",RTLD_LAZY|RTLD_LOCAL);});
        return framework!=NULL;
    }
    return 0;
}
static NSDictionary *RXObserve(pid_t pid,CGWindowID wanted,BOOL dispatched) {
    if (!AXIsProcessTrusted() || !CGPreflightScreenCaptureAccess()) return RXError(@"Desktop access is unavailable. Grant Accessibility and Screen Recording to Reasonix in System Settings, then restart Reasonix.",dispatched);
    NSRunningApplication *app=[NSRunningApplication runningApplicationWithProcessIdentifier:pid];
    NSDictionary *window=RXWindow(pid,wanted);
    if (!app || app.terminated || !window) return RXError(@"The app or its observed window is unavailable. List apps and observe a visible window again.",dispatched);
    CGRect bounds; CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)window[(id)kCGWindowBounds],&bounds);
    CGWindowID wid=[window[(id)kCGWindowNumber] unsignedIntValue];
    NSMutableArray *elements=[NSMutableArray array];
    AXUIElementRef ax=RXWindowAX(pid,bounds);
    BOOL truncated=NO;
    if (ax) {RXTree(ax,@"0",0,elements,CFAbsoluteTimeGetCurrent()+2,&truncated);CFRelease(ax);}
    NSString *image=RXCapture(wid);
    if (!image && app.active) return RXError(@"Window capture failed. Check Screen Recording permission and observe a visible window again.",dispatched);
    NSDictionary *state=@{ @"pid":@(pid),@"bundle_id":app.bundleIdentifier?:@"",@"name":app.localizedName?:@"",@"launch_time":@([app.launchDate timeIntervalSince1970]),
        @"window_id":@(wid),@"bounds":RXBounds(bounds),@"frontmost":@(app.active),@"capture_available":image?@YES:@NO,@"focus":RXFocus(pid),@"elements":elements,@"elements_truncated":@(truncated),@"image_coordinates":@"window points; x/y origin at top left; image may be scaled" };
    return @{ @"ok":@YES,@"state":state,@"image":image?:@"",@"window_id":@(wid),@"dispatched":@(dispatched) };
}
static AXUIElementRef RXElement(AXUIElementRef root,NSString *path) {
    NSArray *parts=[path componentsSeparatedByString:@"."];
    if (parts.count<1 || parts.count>9 || ![parts[0] isEqualToString:@"0"]) return NULL;
    AXUIElementRef e=(AXUIElementRef)CFRetain(root);
    for (NSUInteger i=1;i<parts.count;i++) {
        NSString *part=parts[i]; NSCharacterSet *notDigits=[[NSCharacterSet decimalDigitCharacterSet] invertedSet];
        if (part.length==0 || [part rangeOfCharacterFromSet:notDigits].location!=NSNotFound) {CFRelease(e);return NULL;}
        NSUInteger index=part.integerValue;id children=RXAttribute(e,kAXChildrenAttribute);
        if (![children isKindOfClass:NSArray.class] || index>=[children count]) {CFRelease(e);return NULL;}
        id child=children[index];
        if (CFGetTypeID((__bridge CFTypeRef)child)!=AXUIElementGetTypeID()) {CFRelease(e);return NULL;}
        CFRetain((__bridge CFTypeRef)child);CFRelease(e);e=(__bridge AXUIElementRef)child;
    }
    return e;
}
static BOOL RXKey(pid_t pid,CGKeyCode code,CGEventFlags flags) {
    CGEventRef down=CGEventCreateKeyboardEvent(NULL,code,true),up=CGEventCreateKeyboardEvent(NULL,code,false);
    if (!down || !up) {if(down)CFRelease(down);if(up)CFRelease(up);return NO;}
    CGEventSetFlags(down,flags);CGEventSetFlags(up,flags);
    CGEventPostToPid(pid,down);CGEventPostToPid(pid,up);CFRelease(down);CFRelease(up);return YES;
}
static AXError RXScroll(pid_t pid,CGRect bounds,NSString *path,int delta) {
    AXUIElementRef root=RXWindowAX(pid,bounds);
    if (!root) return kAXErrorInvalidUIElement;
    if (!path.length) {
        NSMutableArray *items=[NSMutableArray array];BOOL truncated=NO;
        RXTree(root,@"0",0,items,CFAbsoluteTimeGetCurrent()+2,&truncated);
        CGPoint center=CGPointMake(CGRectGetMidX(bounds),CGRectGetMidY(bounds));
        double smallest=DBL_MAX;
        for (NSDictionary *item in items) {
            if (![item[@"role"] isEqualToString:@"AXScrollArea"]) continue;
            NSDictionary *b=item[@"bounds"];CGRect r=CGRectMake([b[@"x"] doubleValue],[b[@"y"] doubleValue],[b[@"width"] doubleValue],[b[@"height"] doubleValue]);
            if (CGRectContainsPoint(r,center) && r.size.width*r.size.height<smallest) {path=item[@"element"];smallest=r.size.width*r.size.height;}
        }
    }
    AXUIElementRef target=path.length?RXElement(root,path):NULL;CFRelease(root);
    if (!target) return kAXErrorActionUnsupported;
    BOOL explicitBar=[RXText(RXAttribute(target,kAXRoleAttribute)) isEqualToString:@"AXScrollBar"];
    if (explicitBar && ![RXText(RXAttribute(target,kAXOrientationAttribute)) isEqualToString:(__bridge NSString *)kAXVerticalOrientationValue]) {
        CFRelease(target);return kAXErrorActionUnsupported;
    }
    id bar=explicitBar?(__bridge id)target:RXAttribute(target,kAXVerticalScrollBarAttribute);
    AXError outcome=kAXErrorActionUnsupported;
    if (bar && CFGetTypeID((__bridge CFTypeRef)bar)==AXUIElementGetTypeID()) {
        AXUIElementRef scrollbar=(__bridge AXUIElementRef)bar;Boolean writable=false;
        id value=RXAttribute(scrollbar,kAXValueAttribute);
        if ([value isKindOfClass:NSNumber.class] && [value doubleValue]>=0 && [value doubleValue]<=1 && AXUIElementIsAttributeSettable(scrollbar,kAXValueAttribute,&writable)==kAXErrorSuccess && writable) {
            NSNumber *next=@(fmax(0,fmin(1,[value doubleValue]-delta*0.02)));
            outcome=AXUIElementSetAttributeValue(scrollbar,kAXValueAttribute,(__bridge CFTypeRef)next);
        }
        // Unsupported setters have not dispatched input; use a native page
        // control when available. Never repeat an uncertain AX operation.
        if (outcome==kAXErrorActionUnsupported || outcome==kAXErrorNotImplemented || outcome==kAXErrorAttributeUnsupported) {
            NSString *wanted=delta<0?(__bridge NSString *)kAXIncrementPageSubrole:(__bridge NSString *)kAXDecrementPageSubrole;
            id children=RXAttribute(scrollbar,kAXChildrenAttribute);
            if ([children isKindOfClass:NSArray.class]) for(id child in children) {
                if (CFGetTypeID((__bridge CFTypeRef)child)!=AXUIElementGetTypeID()) continue;
                AXUIElementRef button=(__bridge AXUIElementRef)child;
                if ([RXText(RXAttribute(button,kAXSubroleAttribute)) isEqualToString:wanted]) {outcome=AXUIElementPerformAction(button,kAXPressAction);break;}
            }
        }
    }
    CFRelease(target);return outcome;
}
static NSString *RXPointerBlock(pid_t pid,CGWindowID window,CGRect bounds,CGPoint point) {
    if (!CGPreflightPostEventAccess()) return @"input_permission_missing";
    if (CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState,kCGMouseButtonLeft)) return @"pointer_busy";
    NSArray *windows=CFBridgingRelease(CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly|kCGWindowListExcludeDesktopElements,kCGNullWindowID));
    if (!RXPointerWindowMatches(windows,pid,window,point)) return @"pointer_target_changed";
    AXUIElementRef system=AXUIElementCreateSystemWide(),hit=NULL;
    AXUIElementSetMessagingTimeout(system,0.1);
    AXError error=AXUIElementCopyElementAtPosition(system,point.x,point.y,&hit);CFRelease(system);
    if (error!=kAXErrorSuccess || !hit) {if(hit)CFRelease(hit);return @"pointer_target_unverified";}
    id targetWindow=RXAttribute(hit,kAXWindowAttribute);
    if (!targetWindow && [RXText(RXAttribute(hit,kAXRoleAttribute)) isEqualToString:@"AXWindow"]) targetWindow=(__bridge id)hit;
    BOOL matches=NO;
    if (targetWindow && CFGetTypeID((__bridge CFTypeRef)targetWindow)==AXUIElementGetTypeID()) {
        // Web content may expose a remote AX element from a renderer process.
        // Its containing window must still belong to the approved application.
        pid_t owner=0;
        if (AXUIElementGetPid((__bridge AXUIElementRef)targetWindow,&owner)!=kAXErrorSuccess || owner!=pid) {CFRelease(hit);return @"pointer_target_changed";}
        CGRect actual=RXRect((__bridge AXUIElementRef)targetWindow);
        matches=fabs(actual.origin.x-bounds.origin.x)<=3 && fabs(actual.origin.y-bounds.origin.y)<=3 && fabs(actual.size.width-bounds.size.width)<=6 && fabs(actual.size.height-bounds.size.height)<=6;
    }
    CFRelease(hit);
    if (!matches) return @"pointer_target_unverified";
    AXUIElementRef focused=RXWindowAX(pid,bounds);
    if (!focused) return @"window_not_active";
    CFRelease(focused);
    NSRunningApplication *app=[NSRunningApplication runningApplicationWithProcessIdentifier:pid];
    if (!app.active || [RXWindow(pid,0)[(id)kCGWindowNumber] unsignedIntValue]!=window) return @"window_not_active";
    return nil;
}
static NSDictionary *RXAct(NSDictionary *request) {
    NSString *action=request[@"action"];pid_t pid=[request[@"pid"] intValue];CGWindowID wid=[request[@"window_id"] unsignedIntValue];
    NSDictionary *window=RXWindow(pid,wid);NSRunningApplication *app=[NSRunningApplication runningApplicationWithProcessIdentifier:pid];
    if (!window || !app || app.terminated) return RXError(@"The observed app window closed. Observe again before input.",NO);
    if (!AXIsProcessTrusted() || !CGPreflightScreenCaptureAccess()) return RXError(@"Desktop permission was revoked. Restore Accessibility and Screen Recording in System Settings.",NO);
    if ([action isEqualToString:@"activate"]) {
        BOOL accepted=[app activateWithOptions:0];
        if (!accepted) return RXError(@"The app refused activation. Activate it manually and observe again.",NO);
        return RXObserve(pid,wid,YES);
    }
    NSDictionary *top=RXWindow(pid,0);
    if (!app.active || [top[(id)kCGWindowNumber] unsignedIntValue]!=wid) return @{ @"ok":@NO,@"error":@"The observed window is not active. Activate the app and observe again before input.",@"dispatched":@NO,@"blocked_reason":@"window_not_active" };
    if ([action isEqualToString:@"type"] || [action isEqualToString:@"key"]) {
        id verified=request[@"verified_focus"];
        if (![verified isKindOfClass:NSDictionary.class]) return RXError(@"Keyboard input requires a host-verified focus snapshot. Observe again; no input sent.",NO);
        NSMutableDictionary *expected=[verified mutableCopy],*current=[RXFocus(pid) mutableCopy];
        [expected removeObjectForKey:@"caret"];[current removeObjectForKey:@"caret"];
        if (![expected isEqualToDictionary:current]) return @{ @"ok":@NO,@"error":@"Keyboard focus or field contents changed immediately before input. Observe again; no input sent.",@"dispatched":@NO,@"blocked_reason":@"focus_changed" };
    }
    CGRect bounds;CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)window[(id)kCGWindowBounds],&bounds);
    BOOL sent=NO;NSString *dispatchMethod=nil;
    if ([action isEqualToString:@"click"]) {
        NSString *path=request[@"element"];
        if (path.length) {
            AXUIElementRef root=RXWindowAX(pid,bounds),target=root?RXElement(root,path):NULL;
            if (root)CFRelease(root);
            if (!target) return RXError(@"The observed accessibility element is unavailable. Observe again.",NO);
            AXError error=AXUIElementPerformAction(target,kAXPressAction);CFRelease(target);
            if (error==kAXErrorCannotComplete) return @{ @"ok":@NO,@"error":@"Accessibility action result is unknown; observe again before continuing.",@"dispatched":@NO,@"dispatch_unknown":@YES };
            sent=error==kAXErrorSuccess;
        } else {
            double x=[request[@"x"] doubleValue],y=[request[@"y"] doubleValue];
            if (!isfinite(x) || !isfinite(y) || x<0 || y<0 || x>=bounds.size.width || y>=bounds.size.height) return RXError(@"Click coordinates are outside the observed window. Use window point coordinates.",NO);
            CGPoint point=CGPointMake(bounds.origin.x+x,bounds.origin.y+y);
            NSString *blocked=RXPostWindowServerClick(pid,wid,bounds,point,RXPointerBlock,CGEventPost);
            if (blocked) {
                return @{ @"ok":@NO,@"error":@"Pointer target or input access could not be verified. Observe the approved window again; no input sent.",@"dispatched":@NO,@"blocked_reason":blocked };
            }
            // Process-only posts skip WindowServer hit-testing and can arrive
            // with windowNumber=0. Route an ordinary foreground click through
            // WindowServer only after both window-stack and AX ownership checks.
            sent=YES;dispatchMethod=@"window_server_mouse";
        }
    } else if ([action isEqualToString:@"type"]) {
        NSString *text=request[@"text"]?:@"";NSUInteger count=text.length;
        UniChar *characters=calloc(count?:1,sizeof(UniChar));
        if (!characters) return RXError(@"Keyboard allocation failed. No input was sent.",NO);
        [text getCharacters:characters range:NSMakeRange(0,count)];
        // Bound each event; long Unicode strings are not truncated to one key.
        for (NSUInteger offset=0;offset<count;) {
            NSUInteger length=MIN(20,count-offset);
            if (offset+length<count && CFStringIsSurrogateHighCharacter(characters[offset+length-1])) length--;
            CGEventRef down=CGEventCreateKeyboardEvent(NULL,0,true),up=CGEventCreateKeyboardEvent(NULL,0,false);
            if (!down || !up) {if(down)CFRelease(down);if(up)CFRelease(up);free(characters);return RXError(@"Keyboard event allocation failed; some text may have been sent. Observe before continuing.",offset>0);}
            CGEventSetFlags(down,0);CGEventSetFlags(up,0);
            CGEventKeyboardSetUnicodeString(down,length,characters+offset);CGEventKeyboardSetUnicodeString(up,length,characters+offset);
            CGEventPostToPid(pid,down);CGEventPostToPid(pid,up);CFRelease(down);CFRelease(up);offset+=length;
        }
        free(characters);sent=YES;
    } else if ([action isEqualToString:@"key"]) {
        NSDictionary *codes=@{@"return":@36,@"tab":@48,@"escape":@53,@"backspace":@51,@"space":@49,@"left":@123,@"right":@124,@"up":@126,@"down":@125,@"a":@0,@"c":@8,@"v":@9,@"x":@7,@"z":@6};
        NSNumber *code=codes[request[@"key"]];if(!code)return RXError(@"Unsupported key.",NO);
        CGEventFlags flags=0;for(NSString *m in request[@"modifiers"]) {
            if([m isEqualToString:@"command"])flags|=kCGEventFlagMaskCommand;
            else if([m isEqualToString:@"shift"])flags|=kCGEventFlagMaskShift;
            else if([m isEqualToString:@"option"])flags|=kCGEventFlagMaskAlternate;
            else if([m isEqualToString:@"control"])flags|=kCGEventFlagMaskControl;
        }
        sent=RXKey(pid,code.unsignedShortValue,flags);
    } else if ([action isEqualToString:@"scroll"]) {
        AXError error=RXScroll(pid,bounds,request[@"element"],[request[@"delta"] intValue]);
        if (error==kAXErrorCannotComplete) return @{ @"ok":@NO,@"error":@"Scroll result is unknown; observe again before continuing.",@"dispatched":@NO,@"dispatch_unknown":@YES };
        if (error!=kAXErrorSuccess) return RXError(@"This window does not expose a usable vertical accessibility scroll control. Observe its visible scroll buttons or use keyboard navigation.",NO);
        sent=YES;
    }
    if (!sent)return RXError(@"The app did not accept this action. Observe again; do not assume input succeeded.",NO);
    NSMutableDictionary *result=[RXObserve(pid,wid,YES) mutableCopy];
    if (dispatchMethod) result[@"dispatch_method"]=dispatchMethod;
    return result;
}
char *reasonix_computer_use(const char *input) {
    @autoreleasepool {
        @try {
            NSData *bytes=[[NSString stringWithUTF8String:input] dataUsingEncoding:NSUTF8StringEncoding];
            NSDictionary *r=[NSJSONSerialization JSONObjectWithData:bytes options:0 error:nil];
            NSDictionary *result;
            NSString *action=r[@"action"];
            if ([action isEqualToString:@"status"]) {
                result=@{@"ok":@YES,@"state":@{@"platform":@"macos",@"accessibility":(AXIsProcessTrusted()?@YES:@NO),@"screen_recording":@(CGPreflightScreenCaptureAccess()),@"input_posting":@(CGPreflightPostEventAccess()),@"permission_prompted":@NO},@"dispatched":@NO};
            } else if ([action isEqualToString:@"apps"]) {
                NSMutableArray *apps=[NSMutableArray array];
                for(NSRunningApplication *app in NSWorkspace.sharedWorkspace.runningApplications) {
                    if(app.activationPolicy!=NSApplicationActivationPolicyRegular || app.terminated)continue;
                    [apps addObject:@{@"pid":@(app.processIdentifier),@"name":app.localizedName?:@"",@"bundle_id":app.bundleIdentifier?:@"",@"launch_time":@([app.launchDate timeIntervalSince1970])}];
                }
                result=@{@"ok":@YES,@"state":@{@"apps":apps},@"dispatched":@NO};
            } else if ([action isEqualToString:@"observe"]) result=RXObserve([r[@"pid"] intValue],[r[@"window_id"] unsignedIntValue],NO);
            else result=RXAct(r);
            NSData *json=[NSJSONSerialization dataWithJSONObject:result options:NSJSONWritingSortedKeys error:nil];
            return strdup([[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding].UTF8String);
        } @catch(NSException *exception) {
            return strdup("{\"ok\":false,\"error\":\"Native desktop operation failed; execution result is unknown. Observe again before retrying.\",\"dispatched\":false,\"dispatch_unknown\":true}");
        }
    }
}
