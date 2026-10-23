//go:build darwin && cgo

package computeruse

/*
#cgo CFLAGS: -Wno-deprecated-declarations
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit -framework ApplicationServices -framework ImageIO
#include <stdlib.h>
char *reasonix_computer_use(const char *request);
int reasonix_computer_use_available(void);
*/
import "C"

import (
	"context"
	"encoding/json"
	"unsafe"
)

type nativeBackend struct{}

func Available() bool { return C.reasonix_computer_use_available() != 0 }
func (nativeBackend) call(ctx context.Context, r Request) (nativeResult, error) {
	if err := ctx.Err(); err != nil {
		return nativeResult{}, err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return nativeResult{}, err
	}
	input := C.CString(string(b))
	defer C.free(unsafe.Pointer(input))
	output := C.reasonix_computer_use(input)
	defer C.free(unsafe.Pointer(output))
	var result nativeResult
	err = json.Unmarshal([]byte(C.GoString(output)), &result)
	if err == nil && !result.OK {
		return result, nil
	}
	return result, err
}
