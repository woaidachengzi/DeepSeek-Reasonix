package computeruse

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func previewReply(t testing.TB) CuaReply {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 480, 270)), &jpeg.Options{Quality: 70}); err != nil {
		t.Fatal(err)
	}
	preview, err := json.Marshal(map[string]any{"source_image_index": 0, "mime_type": "image/jpeg", "width": 480, "height": 270, "data_base64": base64.StdEncoding.EncodeToString(buf.Bytes())})
	if err != nil {
		t.Fatal(err)
	}
	return CuaReply{Images: []CuaImage{{MIME: "image/png", Data: "source-is-not-decoded-by-the-thumbnail-validator"}}, Preview: preview}
}

func TestCuaPreviewImageAcceptsOnlyBoundedValidJpegForItsSource(t *testing.T) {
	reply := previewReply(t)
	before := string(reply.Preview)
	thumb := CuaPreviewImage(reply, 0)
	if !strings.HasPrefix(thumb, "data:image/jpeg;base64,") || string(reply.Preview) != before {
		t.Fatal("valid display thumbnail was rejected or reply changed")
	}
	for _, index := range []int{-1, 1} {
		if CuaPreviewImage(reply, index) != "" {
			t.Fatalf("accepted foreign source index %d", index)
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal(reply.Preview, &parsed); err != nil {
		t.Fatal(err)
	}
	var pngBytes bytes.Buffer
	_ = png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 480, 270)))
	patches := []map[string]any{
		{"width": 481}, {"height": 0}, {"height": 269}, {"width": "480"},
		{"source_image_index": -1}, {"source_image_index": 1}, {"mime_type": "image/svg+xml"},
		{"data_base64": "https://example.invalid/private.jpg"}, {"unexpected": true},
		{"data_base64": base64.StdEncoding.EncodeToString(pngBytes.Bytes())},
		{"data_base64": strings.Repeat("A", base64.StdEncoding.EncodedLen(40<<10)+4)},
	}
	for index, patch := range patches {
		t.Run(fmt.Sprintf("invalid_%d", index), func(t *testing.T) {
			candidate := make(map[string]any, len(parsed)+1)
			for k, v := range parsed {
				candidate[k] = v
			}
			for k, v := range patch {
				candidate[k] = v
			}
			reply.Preview = mustJSON(t, candidate)
			if CuaPreviewImage(reply, 0) != "" {
				t.Fatal("invalid optional thumbnail accepted")
			}
		})
	}
	for _, raw := range []string{"null", `"url"`, `[]`, `{"width":480}`, strings.Repeat(" ", (64<<10)+1)} {
		reply.Preview = json.RawMessage(raw)
		if CuaPreviewImage(reply, 0) != "" {
			t.Fatal("invalid optional object accepted")
		}
	}
}

func mustJSON(t testing.TB, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMalformedCuaPreviewDoesNotChangeActionOrFullImages(t *testing.T) {
	for _, preview := range []string{`{"width":"bad"}`, `"not-an-object"`, `null`, `{"extra":"value"}`} {
		wire := `{"protocol":1,"is_error":false,"result":{"effect":"unverifiable"},"images":[{"mime_type":"image/png","data_base64":"original"}],"preview":` + preview + `}`
		var reply CuaReply
		if err := json.Unmarshal([]byte(wire), &reply); err != nil || reply.IsError || reply.Images[0].Data != "original" || CuaPreviewImage(reply, 0) != "" {
			t.Fatalf("display-only failure changed the completed action: %+v %v", reply, err)
		}
	}
}

func BenchmarkCuaPreviewValidation(b *testing.B) {
	reply := previewReply(b)
	b.ReportAllocs()
	for b.Loop() {
		if CuaPreviewImage(reply, 0) == "" {
			b.Fatal("thumbnail validation failed")
		}
	}
}
