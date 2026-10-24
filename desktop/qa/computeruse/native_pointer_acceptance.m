#import <WebKit/WebKit.h>
#import <unistd.h>
#import "../../../internal/computeruse/native_darwin.m"

@interface RXAcceptance : NSObject<NSApplicationDelegate,WKNavigationDelegate,WKScriptMessageHandler>
@property NSWindow *window;
@property NSButton *button;
@property WKWebView *web;
@property int nativeClicks;
@property int webClicks;
@property NSUInteger screenIndex;
@property NSMutableArray *cases;
@end

static void save(NSDictionary *result) {
    NSString *root=NSBundle.mainBundle.bundlePath.stringByDeletingLastPathComponent;
    NSData *data=[NSJSONSerialization dataWithJSONObject:result options:NSJSONWritingPrettyPrinted error:nil];
    [data writeToFile:[root stringByAppendingPathComponent:@"result.json"] options:NSDataWritingAtomic error:nil];
}
@implementation RXAcceptance
-(void)clicked:(id)sender {self.nativeClicks++;}
-(void)userContentController:(WKUserContentController*)controller didReceiveScriptMessage:(WKScriptMessage*)message {self.webClicks++;}
-(void)finish:(NSString*)error {
    save(@{@"ok":@(error==nil),@"error":error?:@"",@"accessibility":@(AXIsProcessTrusted()),@"input_posting":@(CGPreflightPostEventAccess()),@"cases":self.cases?:@[],@"scope":@"Only this disposable app's native button and local WKWebView HTML; production RXPointerBlock and RXPostWindowServerClick"});
    [NSApp terminate:nil];
}
-(CGPoint)pointForView:(NSView*)view {
    NSPoint local=[view convertPoint:NSMakePoint(NSMidX(view.bounds),NSMidY(view.bounds)) toView:nil];
    NSPoint screen=[self.window convertPointToScreen:local];
    return CGPointMake(screen.x,NSMaxY(NSScreen.screens[0].frame)-screen.y);
}
-(void)trial:(BOOL)web {
    CGPoint point=[self pointForView:web?self.web:self.button];
    NSDictionary *info=RXWindow(getpid(),(CGWindowID)self.window.windowNumber);
    CGRect bounds;
    if (!info || !CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)info[(id)kCGWindowBounds],&bounds)) {[self finish:@"own window not available"];return;}
    int before=web?self.webClicks:self.nativeClicks;
    CGWindowID window=(CGWindowID)self.window.windowNumber;
    // The target app must keep servicing AX requests while the production
    // gateway inspects it, as a separate desktop bridge process normally does.
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED,0),^{
        NSString *blocked=RXPostWindowServerClick(getpid(),window,bounds,point,RXPointerBlock,CGEventPost);
        dispatch_async(dispatch_get_main_queue(),^{
            if (blocked) {
                NSMutableArray *hits=[NSMutableArray array];
                NSArray *windows=CFBridgingRelease(CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly|kCGWindowListExcludeDesktopElements,kCGNullWindowID));
                for (NSDictionary *candidate in windows) {
                    CGRect rect;
                    if (!CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)candidate[(id)kCGWindowBounds],&rect) || !CGRectContainsPoint(rect,point)) continue;
                    [hits addObject:@{@"pid":candidate[(id)kCGWindowOwnerPID]?:@0,@"window":candidate[(id)kCGWindowNumber]?:@0,@"layer":candidate[(id)kCGWindowLayer]?:@0,@"alpha":candidate[(id)kCGWindowAlpha]?:@0,@"bounds":RXBounds(rect)}];
                }
                [self.cases addObject:@{@"kind":web?@"web":@"native",@"screen":@(self.screenIndex),@"own_pid":@(getpid()),@"own_window":@(self.window.windowNumber),@"mouse_target":@(RXPointerWindowAtPoint(point)),@"bounds":RXBounds(bounds),@"point":@{@"x":@(point.x),@"y":@(point.y)},@"blocked":blocked,@"windows_at_point":hits}];
                [self finish:blocked];return;
            }
            dispatch_after(dispatch_time(DISPATCH_TIME_NOW,700*NSEC_PER_MSEC),dispatch_get_main_queue(),^{
                int after=web?self.webClicks:self.nativeClicks;
                [self.cases addObject:@{@"kind":web?@"web":@"native",@"screen":@(self.screenIndex),@"bounds":RXBounds(bounds),@"point":@{@"x":@(point.x),@"y":@(point.y)},@"before":@(before),@"after":@(after),@"delta":@(after-before)}];
                if (after-before!=1) {[self finish:@"independent click counter did not increase exactly once"];return;}
                if (!web) {[self trial:YES];return;}
                self.screenIndex++;
                if (self.screenIndex<MIN(2,NSScreen.screens.count)) {[self moveAndTest];return;}
                [self finish:nil];
            });
        });
    });
}
-(void)moveAndTest {
    NSRect area=NSScreen.screens[self.screenIndex].visibleFrame;
    [self.window setFrameOrigin:NSMakePoint(NSMidX(area)-self.window.frame.size.width/2,NSMidY(area)-self.window.frame.size.height/2)];
    [self.window makeKeyAndOrderFront:nil];[NSApp activateIgnoringOtherApps:YES];
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW,700*NSEC_PER_MSEC),dispatch_get_main_queue(),^{[self trial:NO];});
}
-(void)webView:(WKWebView*)view didFinishNavigation:(WKNavigation*)navigation {[self moveAndTest];}
-(void)applicationDidFinishLaunching:(NSNotification*)notification {
    self.cases=[NSMutableArray array];
    self.window=[[NSWindow alloc] initWithContentRect:NSMakeRect(500,300,440,340) styleMask:NSWindowStyleMaskTitled|NSWindowStyleMaskClosable backing:NSBackingStoreBuffered defer:NO];
    self.window.title=@"Reasonix — local click acceptance only";
    NSView *root=self.window.contentView;
    self.button=[NSButton buttonWithTitle:@"Native click counter" target:self action:@selector(clicked:)];self.button.frame=NSMakeRect(50,260,340,40);[root addSubview:self.button];
    WKWebViewConfiguration *configuration=[WKWebViewConfiguration new];configuration.websiteDataStore=WKWebsiteDataStore.nonPersistentDataStore;
    [configuration.userContentController addScriptMessageHandler:self name:@"hit"];
    self.web=[[WKWebView alloc] initWithFrame:NSMakeRect(50,50,340,160) configuration:configuration];self.web.navigationDelegate=self;[root addSubview:self.web];
    [self.web loadHTMLString:@"<!doctype html><meta charset='utf-8'><body style='margin:0'><button style='width:340px;height:160px;font-size:18px' onclick='window.webkit.messageHandlers.hit.postMessage(1)'>Local web click counter</button>" baseURL:nil];
    [self.window makeKeyAndOrderFront:nil];[NSApp activateIgnoringOtherApps:YES];
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW,20*NSEC_PER_SEC),dispatch_get_main_queue(),^{[self finish:@"test timed out"];});
}
-(BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication*)app {return YES;}
@end
int main(int argc,const char**argv) {
    @autoreleasepool {
        [NSApplication sharedApplication];[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        if ((argc>1 && strcmp(argv[1],"--preflight")==0) || !AXIsProcessTrusted() || !CGPreflightPostEventAccess()) {
            save(@{@"ok":@NO,@"phase":@"preflight",@"accessibility":@(AXIsProcessTrusted()),@"input_posting":@(CGPreflightPostEventAccess()),@"permission_prompted":@NO,@"events_posted":@0});return 0;
        }
        RXAcceptance *delegate=[RXAcceptance new];NSApp.delegate=delegate;[NSApp run];
    }
    return 0;
}
