package computeruse

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/jpeg"
	"math"
	"strings"
)

// ImageGeometry describes the exact image accompanying an observation, rather
// than a renderer thumbnail or a model's assumed display size.
type ImageGeometry struct {
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	PointsPerPixelX float64 `json:"points_per_pixel_x"`
	PointsPerPixelY float64 `json:"points_per_pixel_y"`
}

func observationImageGeometry(v nativeResult) *ImageGeometry {
	encoded, ok := strings.CutPrefix(v.Image, "data:image/jpeg;base64,")
	if !ok || len(encoded) > 5<<20 {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 1920 || config.Height > 1920 {
		return nil
	}
	var state struct{ Bounds frameRect }
	if json.Unmarshal(v.State, &state) != nil || !finitePositive(state.Bounds.Width) || !finitePositive(state.Bounds.Height) {
		return nil
	}
	return &ImageGeometry{Width: config.Width, Height: config.Height, PointsPerPixelX: state.Bounds.Width / float64(config.Width), PointsPerPixelY: state.Bounds.Height / float64(config.Height)}
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validateCoordinateSpace(r Request) error {
	if r.CoordinateSpace != "" && r.CoordinateSpace != "window" && r.CoordinateSpace != "image" {
		return errors.New("coordinate_space must be image or window")
	}
	if r.CoordinateSpace == "image" && ((r.Action != "click" && r.Action != "fill") || r.Element != "" || r.X == nil || r.Y == nil) {
		return errors.New("coordinate_space=image requires a click/fill x/y target without an element path")
	}
	return nil
}

// Resolve only against the host's verified observation. Callers cannot supply
// image dimensions or screen origins, and conversion never sends input.
func windowPointRequest(r Request, observed nativeResult) (Request, error) {
	if r.CoordinateSpace != "image" {
		return r, nil
	}
	geometry := observationImageGeometry(observed)
	if geometry == nil || r.X == nil || r.Y == nil {
		return r, errors.New("screenshot coordinate mapping unavailable; observe again before input")
	}
	if *r.X < 0 || *r.Y < 0 || *r.X >= float64(geometry.Width) || *r.Y >= float64(geometry.Height) {
		return r, errors.New("x/y are outside the observed screenshot; use its image_geometry pixel dimensions, not a preview or global screen coordinates")
	}
	x, y := *r.X*geometry.PointsPerPixelX, *r.Y*geometry.PointsPerPixelY
	r.X, r.Y, r.CoordinateSpace = &x, &y, "window"
	return r, nil
}
