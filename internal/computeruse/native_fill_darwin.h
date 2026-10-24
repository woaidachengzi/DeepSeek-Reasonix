// Included after the window, AX and pointer helpers in native_darwin.m.
static BOOL RXFillWindowOwns(AXUIElementRef element,pid_t pid,CGRect bounds) {
    AXUIElementRef current=(AXUIElementRef)CFRetain(element);
    double deadline=CFAbsoluteTimeGetCurrent()+0.25;
    for (NSUInteger depth=0;current && depth<32 && CFAbsoluteTimeGetCurrent()<deadline;depth++) {
        AXUIElementSetMessagingTimeout(current,0.05);
        if ([RXText(RXAttribute(current,kAXRoleAttribute)) isEqualToString:@"AXWindow"]) {
            pid_t owner=0;CGRect r=RXRect(current);
            BOOL match=AXUIElementGetPid(current,&owner)==kAXErrorSuccess && owner==pid && fabs(r.origin.x-bounds.origin.x)<=3 && fabs(r.origin.y-bounds.origin.y)<=3 && fabs(r.size.width-bounds.size.width)<=6 && fabs(r.size.height-bounds.size.height)<=6;
            CFRelease(current);return match;
        }
        id parent=RXAttribute(current,kAXParentAttribute);
        AXUIElementRef next=parent && CFGetTypeID((__bridge CFTypeRef)parent)==AXUIElementGetTypeID()?(AXUIElementRef)CFRetain((__bridge AXUIElementRef)parent):NULL;
        CFRelease(current);current=next;
    }
    if(current)CFRelease(current);return NO;
}

static NSDictionary *RXFill(NSDictionary *request,pid_t pid,CGWindowID window,CGRect bounds) {
    AXUIElementRef target=NULL;
    NSString *path=request[@"element"];
    CGPoint point;
    if (path.length) {
        AXUIElementRef root=RXWindowAX(pid,bounds);
        target=root?RXElement(root,path):NULL;
        if(root)CFRelease(root);
        if (!target) return @{ @"ok":@NO,@"error":@"Observed editable target is unavailable. Observe again.",@"dispatched":@NO,@"blocked_reason":@"editable_target_unverified" };
        CGRect rect=RXRect(target);point=CGPointMake(CGRectGetMidX(rect),CGRectGetMidY(rect));
    } else {
        point=CGPointMake(bounds.origin.x+[request[@"x"] doubleValue],bounds.origin.y+[request[@"y"] doubleValue]);
        NSString *blocked=CGRectContainsPoint(bounds,point)?RXPointerBlock(pid,window,bounds,point):@"pointer_target_unverified";
        if (blocked) return @{ @"ok":@NO,@"error":@"Editable target is not the approved uncovered window. Observe again.",@"dispatched":@NO,@"blocked_reason":blocked };
        AXUIElementRef system=AXUIElementCreateSystemWide();AXUIElementSetMessagingTimeout(system,0.05);
        AXUIElementCopyElementAtPosition(system,point.x,point.y,&target);CFRelease(system);
    }
    if (!target) return @{ @"ok":@NO,@"error":@"The point does not expose an editable text field. Do not repeat fill; inspect a fresh observation.",@"dispatched":@NO,@"blocked_reason":@"editable_target_unverified" };
    AXUIElementSetMessagingTimeout(target,0.05);
    NSString *role=RXText(RXAttribute(target,kAXRoleAttribute)),*subrole=RXText(RXAttribute(target,kAXSubroleAttribute));
    CGRect rect=RXRect(target);
    BOOL editable=([role isEqualToString:@"AXTextField"] || [role isEqualToString:@"AXTextArea"]) && ![subrole isEqualToString:@"AXSecureTextField"] && CGRectContainsPoint(rect,point) && CGRectContainsRect(bounds,rect) && RXFillWindowOwns(target,pid,bounds);
    Boolean settable=false;
    if ([RXAttribute(target,kAXEnabledAttribute) isEqual:@NO]) editable=NO;
    if (editable) editable=AXUIElementIsAttributeSettable(target,kAXValueAttribute,&settable)==kAXErrorSuccess && settable;
    id before=editable?RXAttribute(target,kAXValueAttribute):nil;
    if (!editable || ![before isKindOfClass:NSString.class]) {
        CFRelease(target);return @{ @"ok":@NO,@"error":@"This is not a verified writable non-secure text field. Use a fresh observation; no text was sent.",@"dispatched":@NO,@"blocked_reason":@"editable_target_unverified" };
    }
    NSString *blocked=RXPointerBlock(pid,window,bounds,point);
    if (blocked) {CFRelease(target);return @{ @"ok":@NO,@"error":@"Editable target changed before writing. Observe again.",@"dispatched":@NO,@"blocked_reason":blocked };}
    if (![before isEqual:RXAttribute(target,kAXValueAttribute)] || !CGRectEqualToRect(rect,RXRect(target))) {
        CFRelease(target);return @{ @"ok":@NO,@"error":@"Field content or geometry changed before writing. Observe again.",@"dispatched":@NO,@"blocked_reason":@"controls_changed" };
    }
    NSString *text=request[@"text"];
    BOOL unchanged=[before isEqualToString:text];
    AXError error=unchanged?kAXErrorSuccess:AXUIElementSetAttributeValue(target,kAXValueAttribute,(__bridge CFTypeRef)text);
    if (error!=kAXErrorSuccess) {
        CFRelease(target);return @{ @"ok":@NO,@"error":@"Text field write was not confirmed. Observe its actual contents; do not repeat automatically.",@"dispatched":@NO,@"dispatch_unknown":@YES,@"input_verification":@"unconfirmed" };
    }
    id after=RXAttribute(target,kAXValueAttribute);
    BOOL confirmed=[after isKindOfClass:NSString.class] && [after isEqualToString:text] && CGRectEqualToRect(RXRect(target),rect) && RXFillWindowOwns(target,pid,bounds);
    CFRelease(target);
    NSMutableDictionary *result=[RXObserve(pid,window,!unchanged) mutableCopy];
    result[@"dispatch_method"]=@"accessibility_value";
    result[@"input_verification"]=confirmed?@"confirmed":@"unconfirmed";
    result[@"unchanged"]=@(unchanged);
    return result;
}
