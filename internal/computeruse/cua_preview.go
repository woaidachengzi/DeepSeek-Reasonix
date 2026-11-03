package computeruse

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/jpeg"
	"io"
)

// CuaPreviewImage validates a small worker-produced display thumbnail. It is
// never used for model input, capture identity or action coordinates. Unknown
// or malformed optional fields return empty so callers keep the old fallback.
func CuaPreviewImage(reply CuaReply, imageIndex int) string {
	if len(reply.Preview) == 0 || len(reply.Preview) > 64<<10 || imageIndex < 0 || imageIndex >= len(reply.Images) {
		return ""
	}
	source := reply.Images[imageIndex]
	if (source.MIME != "image/png" && source.MIME != "image/jpeg") || len(source.Data) > 5<<20 {
		return ""
	}
	var preview struct {
		Index  *int   `json:"source_image_index"`
		MIME   string `json:"mime_type"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Data   string `json:"data_base64"`
	}
	decoder := json.NewDecoder(bytes.NewReader(reply.Preview))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&preview) != nil || decoder.Decode(new(any)) != io.EOF ||
		preview.Index == nil || *preview.Index != imageIndex || preview.MIME != "image/jpeg" ||
		preview.Width < 1 || preview.Height < 1 || preview.Width > 480 || preview.Height > 480 ||
		len(preview.Data) == 0 || len(preview.Data) > base64.StdEncoding.EncodedLen(40<<10) {
		return ""
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(preview.Data)
	if err != nil || len(raw) == 0 || len(raw) > 40<<10 || base64.StdEncoding.EncodeToString(raw) != preview.Data {
		return ""
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width != preview.Width || config.Height != preview.Height {
		return ""
	}
	// Validate the small JPEG body before giving it to a renderer. The header
	// gate above bounds this decode to at most 480x480, never the full capture.
	if _, err := jpeg.Decode(bytes.NewReader(raw)); err != nil {
		return ""
	}
	return "data:image/jpeg;base64," + preview.Data
}
