package computeruse

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	_ "image/png"
	"strings"
)

// PreviewImage creates a bounded display-only thumbnail from an existing
// observation. It never captures additional screens or accepts external URLs.
func PreviewImage(data string) string {
	const prefix = "data:image/jpeg;base64,"
	inputPrefix := prefix
	if strings.HasPrefix(data, "data:image/png;base64,") {
		inputPrefix = "data:image/png;base64,"
	}
	if !strings.HasPrefix(data, inputPrefix) || len(data) > 5<<20 {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, inputPrefix))
	if err != nil {
		return ""
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 {
		return ""
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	w, h := config.Width, config.Height
	if w > 480 || h > 480 {
		if w >= h {
			h = max(1, h*480/w)
			w = 480
		} else {
			w = max(1, w*480/h)
			h = 480
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, y, src.At(x*config.Width/w, y*config.Height/h))
		}
	}
	// Keep the complete wire event below the desktop observer's 64 KiB limit.
	for _, quality := range []int{70, 45, 25} {
		var out bytes.Buffer
		if jpeg.Encode(&out, dst, &jpeg.Options{Quality: quality}) == nil && out.Len() <= 40<<10 {
			return prefix + base64.StdEncoding.EncodeToString(out.Bytes())
		}
	}
	return ""
}
