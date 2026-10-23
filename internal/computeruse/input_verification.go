package computeruse

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"unicode/utf16"
)

type textSelection struct {
	Location int `json:"location"`
	Length   int `json:"length"`
}

type focusedInput struct {
	Role        string         `json:"role"`
	Subrole     string         `json:"subrole"`
	Identifier  string         `json:"identifier"`
	Bounds      frameRect      `json:"bounds"`
	Value       *string        `json:"value"`
	ValueHash   string         `json:"value_hash"`
	ValueLength int            `json:"value_length"`
	Selection   *textSelection `json:"selection"`
	Caret       *frameRect     `json:"caret"`
}

type inputState struct {
	Bounds    frameRect    `json:"bounds"`
	Frontmost bool         `json:"frontmost"`
	Focus     focusedInput `json:"focus"`
}

func canonicalState(raw json.RawMessage) []byte {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return raw
	}
	data, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return data
}

func observationChangeReason(before, after nativeResult) string {
	if identity(before) != identity(after) {
		return "window_changed"
	}
	var a, b inputState
	if json.Unmarshal(before.State, &a) == nil && json.Unmarshal(after.State, &b) == nil {
		if a.Bounds != b.Bounds {
			return "window_changed"
		}
		if !b.Frontmost {
			return "window_not_active"
		}
		af, _ := json.Marshal(a.Focus)
		bf, _ := json.Marshal(b.Focus)
		if !bytes.Equal(af, bf) {
			return "focus_changed"
		}
	}
	if !bytes.Equal(canonicalState(before.State), canonicalState(after.State)) {
		return "controls_changed"
	}
	return "visual_changed"
}

func verifiedInputResult(result Result, request Request, before, after nativeResult) Result {
	if request.Action != "type" || (!result.Dispatched && !result.DispatchUnknown) {
		return result
	}
	result.InputVerification = "unconfirmed"
	var a, b inputState
	if json.Unmarshal(before.State, &a) != nil || json.Unmarshal(after.State, &b) != nil || !a.Frontmost || !b.Frontmost || identity(before) != identity(after) {
		return result
	}
	if !sameEditableTarget(a.Focus, b.Focus) {
		return result
	}
	selection := a.Focus.Selection
	if selection == nil {
		return result
	}
	old := utf16.Encode([]rune(*a.Focus.Value))
	// The display value is bounded by native AX. Do not infer an edit using a
	// truncated prefix, or treat an unverified dispatch as a failed operation.
	if (a.Focus.ValueHash != "" && a.Focus.ValueLength != len(old)) || (a.Focus.ValueHash == "" && len(old) >= 512) {
		return result
	}
	if selection.Location < 0 || selection.Length < 0 || selection.Location > len(old) || selection.Length > len(old)-selection.Location {
		return result
	}
	typed := utf16.Encode([]rune(request.Text))
	expected := append(append(append([]uint16{}, old[:selection.Location]...), typed...), old[selection.Location+selection.Length:]...)
	text := string(utf16.Decode(expected))
	hash := sha256.Sum256([]byte(text))
	if b.Focus.ValueHash != "" {
		if b.Focus.ValueLength != len(expected) || b.Focus.ValueHash != hex.EncodeToString(hash[:]) {
			return result
		}
	} else if len(expected) >= 512 || *b.Focus.Value != text {
		return result
	}
	result.InputVerification = "confirmed"
	return result
}

func editableFocus(f focusedInput) bool {
	return (f.Role == "AXTextField" || f.Role == "AXTextArea") && f.Subrole != "AXSecureTextField" && f.Value != nil && *f.Value != "[secure]"
}

func sameEditableTarget(a, b focusedInput) bool {
	return editableFocus(a) && editableFocus(b) && a.Role == b.Role && a.Subrole == b.Subrole && a.Identifier == b.Identifier && a.Bounds == b.Bounds
}
