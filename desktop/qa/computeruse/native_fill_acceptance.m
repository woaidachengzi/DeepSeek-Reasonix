#import <WebKit/WebKit.h>
#import <unistd.h>
#import "../../../internal/computeruse/native_darwin.m"

static NSDictionary *RXQACompact(NSDictionary *result) {
    NSMutableDictionary *summary=[NSMutableDictionary dictionary];
    for (NSString *key in @[@"ok",@"error",@"dispatched",@"dispatch_unknown",@"dispatch_method",@"input_verification",@"unchanged",@"blocked_reason"]) if (result[key]) summary[key]=result[key];
    return summary;
}

@interface RXFillAcceptance : NSObject<NSApplicationDelegate,WKNavigationDelegate>
@property NSWindow *window;
@property NSTextField *field;
@property NSSecureTextField *secure;
@property NSButton *button;
@property WKWebView *web;
@property NSUInteger screenIndex;
@property NSMutableArray *cases;
@end

@implementation RXFillAcceptance
-(void)finish:(NSString*)error {
    BOOL all=error==nil,native=error==nil;NSUInteger nativeCount=0,webCount=0;
    for (NSDictionary *item in self.cases) {
        if (![item[@"passed"] boolValue]) all=NO;
        if ([item[@"kind"] isEqual:@"fill_native"]) {nativeCount++;native=native && [item[@"passed"] boolValue];}
        if ([item[@"kind"] isEqual:@"fill_web"] && [item[@"passed"] boolValue]) webCount++;
    }
    NSDictionary *report=@{@"ok":@(all),@"native_fill_confirmed":@(native && nativeCount>0),@"web_fill_confirmed_cases":@(webCount),@"error":error?:@"",@"cases":self.cases?:@[],@"scope":@"Production RXFill against only this app's native field and inline WKWebView textarea, with secure/button refusals and idempotent repeat. Unsupported web fields are recorded as failed fill cases even when native cases pass. No keyboard events, external pages or messages. Post-write screen capture may be unavailable; independent local values verify the write."};
    NSString *root=NSBundle.mainBundle.bundlePath.stringByDeletingLastPathComponent;
    [[NSJSONSerialization dataWithJSONObject:report options:NSJSONWritingPrettyPrinted error:nil] writeToFile:[root stringByAppendingPathComponent:@"result.json"] atomically:YES];
    [NSApp terminate:nil];
}
-(CGPoint)pointForView:(NSView*)view {
    NSPoint p=[self.window convertPointToScreen:[view convertPoint:NSMakePoint(NSMidX(view.bounds),NSMidY(view.bounds)) toView:nil]];
    return CGPointMake(p.x,NSMaxY(NSScreen.screens[0].frame)-p.y);
}
-(NSDictionary*)fillAt:(CGPoint)point text:(NSString*)text {
    NSDictionary *info=RXWindow(getpid(),(CGWindowID)self.window.windowNumber);CGRect bounds;
    if (!info || !CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)info[(id)kCGWindowBounds],&bounds)) return @{};
    // A separate process matches production AX delivery. Calling our own AX
    // setter on a worker thread executes AppKit there and can deadlock TextKit.
    NSDictionary *request=@{@"pid":@(getpid()),@"window_id":@(self.window.windowNumber),@"x":@(point.x-bounds.origin.x),@"y":@(point.y-bounds.origin.y),@"text":text};
    NSData *data=[NSJSONSerialization dataWithJSONObject:request options:0 error:nil];
    NSTask *task=[NSTask new];task.executableURL=[NSURL fileURLWithPath:NSBundle.mainBundle.executablePath];task.arguments=@[@"--fill-worker",[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding]];
    NSPipe *pipe=[NSPipe pipe];task.standardOutput=pipe;
    if (![task launchAndReturnError:nil]) return @{};
    data=[pipe.fileHandleForReading readDataToEndOfFile];[task waitUntilExit];
    NSDictionary *result=[NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
    return [result isKindOfClass:NSDictionary.class]?result:@{};
}
-(void)trial:(BOOL)web {
    NSString *text=web?@"Local web QA\n第二行 ✅":@"Local native QA 中文 ✅";
    CGPoint point=[self pointForView:web?self.web:self.field];
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED,0),^{
        NSDictionary *first=[self fillAt:point text:text];
        NSDictionary *repeat=[first[@"input_verification"] isEqual:@"confirmed"]?[self fillAt:point text:text]:@{};
        dispatch_async(dispatch_get_main_queue(),^{
            void (^check)(NSString*)=^(NSString *actual) {
                BOOL pass=[actual isEqualToString:text] && [first[@"input_verification"] isEqual:@"confirmed"] && [first[@"dispatched"] boolValue] && [repeat[@"input_verification"] isEqual:@"confirmed"] && [repeat[@"unchanged"] boolValue] && ![repeat[@"dispatched"] boolValue] && ![repeat[@"dispatch_unknown"] boolValue];
                [self.cases addObject:@{@"kind":web?@"fill_web":@"fill_native",@"screen":@(self.screenIndex),@"x":@(point.x),@"y":@(point.y),@"independent_value_matches":@([actual isEqualToString:text]),@"first":RXQACompact(first),@"repeat":RXQACompact(repeat),@"passed":@(pass)}];
                BOOL unsupported=web && [first[@"blocked_reason"] isEqual:@"editable_target_unverified"] && ![first[@"dispatched"] boolValue] && ![first[@"dispatch_unknown"] boolValue];
                if (!pass && !unsupported) {[self finish:@"Local field write/readback or idempotent repeat failed"];return;}
                if (!web) {[self trial:YES];return;}
                self.screenIndex++;
                if (self.screenIndex<MIN(2,NSScreen.screens.count)) {[self moveAndTest];return;}
                [self finish:nil];
            };
            if (web) {
                [self.web evaluateJavaScript:@"document.querySelector('textarea').value" completionHandler:^(id value,NSError *error){check([value isKindOfClass:NSString.class]?value:@"");}];
            } else check(self.field.stringValue);
        });
    });
}
-(void)negativeTrials {
    CGPoint secure=[self pointForView:self.secure],button=[self pointForView:self.button];
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED,0),^{
        NSDictionary *a=[self fillAt:secure text:@"must not write"],*b=[self fillAt:button text:@"must not press"];
        dispatch_async(dispatch_get_main_queue(),^{
            BOOL pass=! [a[@"dispatched"] boolValue] && ![a[@"dispatch_unknown"] boolValue] && [a[@"blocked_reason"] isEqual:@"editable_target_unverified"] && ![b[@"dispatched"] boolValue] && ![b[@"dispatch_unknown"] boolValue] && [b[@"blocked_reason"] isEqual:@"editable_target_unverified"] && [self.secure.stringValue isEqual:@"fixture-only"];
            [self.cases addObject:@{@"kind":@"refuse_secure_and_button",@"screen":@(self.screenIndex),@"passed":@(pass)}];
            if (pass) [self trial:NO];else [self finish:@"A non-editable or secure target was not refused"];
        });
    });
}
-(void)moveAndTest {
    NSRect area=NSScreen.screens[self.screenIndex].visibleFrame;
    [self.window setFrameOrigin:NSMakePoint(NSMidX(area)-220,NSMidY(area)-190)];
    [self.window makeKeyAndOrderFront:nil];[NSApp activateIgnoringOtherApps:YES];
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW,700*NSEC_PER_MSEC),dispatch_get_main_queue(),^{
        self.field.stringValue=@"initial native";
        [self.web evaluateJavaScript:@"document.querySelector('textarea').value='initial web'" completionHandler:^(id value,NSError *error){[self negativeTrials];}];
    });
}
-(void)webView:(WKWebView*)web didFinishNavigation:(WKNavigation*)navigation {[self moveAndTest];}
-(void)applicationDidFinishLaunching:(NSNotification*)notification {
    self.cases=[NSMutableArray array];
    self.window=[[NSWindow alloc] initWithContentRect:NSMakeRect(300,300,440,380) styleMask:NSWindowStyleMaskTitled|NSWindowStyleMaskClosable backing:NSBackingStoreBuffered defer:NO];
    self.window.title=@"Reasonix — local text fill acceptance only";
    self.field=[[NSTextField alloc] initWithFrame:NSMakeRect(40,310,360,26)];[self.window.contentView addSubview:self.field];
    self.secure=[[NSSecureTextField alloc] initWithFrame:NSMakeRect(40,270,200,26)];self.secure.stringValue=@"fixture-only";[self.window.contentView addSubview:self.secure];
    self.button=[NSButton buttonWithTitle:@"Must not press" target:nil action:nil];self.button.frame=NSMakeRect(250,270,150,26);[self.window.contentView addSubview:self.button];
    WKWebViewConfiguration *config=[WKWebViewConfiguration new];config.websiteDataStore=WKWebsiteDataStore.nonPersistentDataStore;
    self.web=[[WKWebView alloc] initWithFrame:NSMakeRect(40,40,360,180) configuration:config];self.web.navigationDelegate=self;[self.window.contentView addSubview:self.web];
    [self.web loadHTMLString:@"<!doctype html><meta charset='utf-8'><body style='margin:0'><textarea style='box-sizing:border-box;width:360px;height:180px;font-size:18px'></textarea>" baseURL:nil];
    [self.window makeKeyAndOrderFront:nil];[NSApp activateIgnoringOtherApps:YES];
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW,20*NSEC_PER_SEC),dispatch_get_main_queue(),^{[self finish:@"test timed out"];});
}
@end
int main(int argc,const char**argv) {
    @autoreleasepool {
        if (argc==3 && strcmp(argv[1],"--fill-worker")==0) {
            NSDictionary *request=[NSJSONSerialization JSONObjectWithData:[@(argv[2]) dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
            pid_t pid=[request[@"pid"] intValue];CGWindowID window=[request[@"window_id"] unsignedIntValue];CGRect bounds;
            NSRunningApplication *parent=[NSRunningApplication runningApplicationWithProcessIdentifier:pid];
            if (pid!=getppid() || ![parent.bundleURL.path isEqual:NSBundle.mainBundle.bundlePath] || !AXIsProcessTrusted() || !CGPreflightPostEventAccess()) return 2;
            NSDictionary *info=RXWindow(pid,window);
            if (!info || !CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)info[(id)kCGWindowBounds],&bounds)) return 3;
            // Production always traverses the observed AX tree before acting.
            // Initialize the fixture's remote web AX bridge through that same
            // read-only path; do not force private accessibility flags.
            AXUIElementRef root=RXWindowAX(pid,bounds);
            if (root) {NSMutableArray *elements=[NSMutableArray array];BOOL truncated=NO;RXTree(root,@"0",0,elements,CFAbsoluteTimeGetCurrent()+0.25,&truncated);CFRelease(root);}
            NSDictionary *result=RXFill(request,pid,window,bounds);
            NSData *output=[NSJSONSerialization dataWithJSONObject:RXQACompact(result) options:0 error:nil];
            fwrite(output.bytes,1,output.length,stdout);return 0;
        }
        [NSApplication sharedApplication];[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        if ((argc>1 && strcmp(argv[1],"--preflight")==0) || !AXIsProcessTrusted() || !CGPreflightPostEventAccess()) {
            NSString *path=[NSBundle.mainBundle.bundlePath.stringByDeletingLastPathComponent stringByAppendingPathComponent:@"result.json"];
            [[NSJSONSerialization dataWithJSONObject:@{@"ok":@NO,@"phase":@"preflight",@"accessibility":@(AXIsProcessTrusted()),@"input_posting":@(CGPreflightPostEventAccess()),@"writes":@0} options:NSJSONWritingPrettyPrinted error:nil] writeToFile:path atomically:YES];return 0;
        }
        RXFillAcceptance *delegate=[RXFillAcceptance new];NSApp.delegate=delegate;[NSApp run];
    }
    return 0;
}
