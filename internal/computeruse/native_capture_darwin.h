#import <CoreGraphics/CoreGraphics.h>
#include <math.h>

// Match logical window points whenever possible. Upscaling small windows makes
// screenshot pixels look like valid point coordinates while missing the target.
static CGSize RXCaptureSize(CGSize window) {
    if (!isfinite(window.width) || !isfinite(window.height) || window.width<=0 || window.height<=0) return CGSizeZero;
    double scale=fmin(1.0,1920.0/fmax(window.width,window.height));
    return CGSizeMake(fmax(1,floor(window.width*scale)),fmax(1,floor(window.height*scale)));
}
