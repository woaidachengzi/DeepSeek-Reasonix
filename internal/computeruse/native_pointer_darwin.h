#import <Foundation/Foundation.h>
#import <ApplicationServices/ApplicationServices.h>

// Read-only mouse-target preflight and event allocation. No input is posted.
BOOL RXPointerWindowMatches(NSArray<NSDictionary *> *windows, pid_t pid,
                            CGWindowID window, CGPoint point, CGWindowID mouseTarget);
CGWindowID RXPointerWindowAtPoint(CGPoint point);
BOOL RXCreateWindowServerClick(CGPoint point, CGEventRef *move,
                              CGEventRef *down, CGEventRef *up);
typedef NSString *(*RXPointerPreflight)(pid_t,CGWindowID,CGRect,CGPoint);
typedef void (*RXPointerPost)(CGEventTapLocation,CGEventRef);
NSString *RXPostWindowServerClick(pid_t pid,CGWindowID window,CGRect bounds,CGPoint point,
                                  RXPointerPreflight preflight,RXPointerPost post);
typedef BOOL (*RXPointerRead)(CGPoint *);
BOOL RXReadPointer(CGPoint *point);
BOOL RXPointerNear(CGPoint actual,CGPoint expected);
NSString *RXPostWindowServerMove(pid_t pid,CGWindowID window,CGRect bounds,CGPoint point,
                                 RXPointerPreflight preflight,RXPointerPost post);
NSString *RXPostConfirmedWindowServerClick(pid_t pid,CGWindowID window,CGRect bounds,CGPoint point,
                                          RXPointerRead read,RXPointerPreflight preflight,RXPointerPost post);
