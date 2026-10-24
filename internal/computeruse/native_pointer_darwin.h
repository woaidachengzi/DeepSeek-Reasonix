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
