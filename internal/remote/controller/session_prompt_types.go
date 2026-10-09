package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

// Controller and prompt routing epochs are separate, captured identities.
// Neither a catalogue entry nor this DTO grants decision authority.
type SessionPromptScope struct {
	SessionPath        string `json:"sessionPath"`
	RuntimeEpoch       string `json:"runtimeEpoch"`
	TurnID             string `json:"turnId"`
	PromptID           string `json:"promptId"`
	PromptRuntimeEpoch string `json:"promptRuntimeEpoch"`
	Kind               string `json:"kind"`
}

type SessionPromptRequest struct {
	SessionPromptScope
	Answer json.RawMessage `json:"answer"`
}

type SessionPromptReceipt struct {
	ProtocolVersion int `json:"protocolVersion"`
	SessionPromptScope
	Resolved bool `json:"resolved"`
}

type SessionPromptQuestionAnswer struct {
	QuestionID string   `json:"questionId"`
	Selected   []string `json:"selected"`
}

// Decoded answer values never echo into a public receipt. The wire union is
// validated by kind before dispatch, including explicit false/empty answers.
type SessionPromptAnswer struct {
	Questions []SessionPromptQuestionAnswer
	Allow     bool
	Session   bool
	Persist   bool
	Action    string
	Feedback  string
	Content   map[string]any
}

func DecodeSessionPromptAnswer(input SessionPromptRequest) (SessionPromptAnswer, error) {
	s := input.SessionPromptScope
	for _, field := range []struct {
		value string
		limit int
	}{{s.SessionPath, 32768}, {s.RuntimeEpoch, 4096}, {s.TurnID, 4096}, {s.PromptID, 4096}} {
		if field.value == "" || !cleanField(field.value, field.limit) {
			return SessionPromptAnswer{}, ErrResponse
		}
	}
	if !cleanField(s.PromptRuntimeEpoch, 4096) || len(input.Answer) > 128<<10 || !utf8.Valid(input.Answer) {
		return SessionPromptAnswer{}, ErrResponse
	}
	fields, ok := promptAnswerFields(input.Answer)
	if !ok {
		return SessionPromptAnswer{}, ErrResponse
	}
	allowed := map[string]bool{}
	switch s.Kind {
	case "ask":
		allowed["questions"] = true
	case "approval":
		for _, key := range []string{"allow", "session", "persist"} {
			allowed[key] = true
		}
	case "plan", "recovery":
		allowed["action"] = true
		allowed["feedback"] = true
	case "mcp":
		allowed["action"] = true
		allowed["content"] = true
	default:
		return SessionPromptAnswer{}, ErrResponse
	}
	for key, value := range fields {
		if !allowed[key] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return SessionPromptAnswer{}, ErrResponse
		}
	}
	decode := func(value json.RawMessage, target any) bool {
		d := json.NewDecoder(bytes.NewReader(value))
		d.DisallowUnknownFields()
		return d.Decode(target) == nil && d.Decode(new(any)) == io.EOF
	}
	var result SessionPromptAnswer
	if s.Kind == "ask" {
		if !decode(fields["questions"], &result.Questions) || result.Questions == nil || len(result.Questions) > 32 {
			return result, ErrResponse
		}
		var rawQuestions []json.RawMessage
		if !decode(fields["questions"], &rawQuestions) {
			return result, ErrResponse
		}
		seen := map[string]bool{}
		for i, q := range result.Questions {
			// encoding/json accepts null into a string as its zero value. Do
			// not silently turn a malformed selection into an empty answer, or
			// let duplicate nested decision keys use last-key-wins semantics.
			question, valid := promptAnswerFields(rawQuestions[i])
			var selected []json.RawMessage
			if !valid || len(question) != 2 || question["questionId"] == nil || !decode(question["selected"], &selected) {
				return result, ErrResponse
			}
			for _, value := range selected {
				if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return result, ErrResponse
				}
			}
			if q.QuestionID == "" || !cleanField(q.QuestionID, 4096) || seen[q.QuestionID] || q.Selected == nil || len(q.Selected) > 64 {
				return result, ErrResponse
			}
			seen[q.QuestionID] = true
			for _, text := range q.Selected {
				if !promptText(text, 8192) {
					return result, ErrResponse
				}
			}
		}
	} else if s.Kind == "approval" {
		if !decode(fields["allow"], &result.Allow) {
			return result, ErrResponse
		}
		if v, ok := fields["session"]; ok && !decode(v, &result.Session) {
			return result, ErrResponse
		}
		if v, ok := fields["persist"]; ok && !decode(v, &result.Persist) {
			return result, ErrResponse
		}
		if (result.Session && result.Persist) || (!result.Allow && (result.Session || result.Persist)) {
			return result, ErrResponse
		}
	} else {
		if !decode(fields["action"], &result.Action) {
			return result, ErrResponse
		}
		valid := false
		switch s.Kind {
		case "plan":
			valid = result.Action == "start_execution" || result.Action == "revise_plan" || result.Action == "exit_plan"
		case "recovery":
			valid = result.Action == "continue" || result.Action == "continue_task" || result.Action == "revise"
		case "mcp":
			valid = result.Action == "accept" || result.Action == "decline" || result.Action == "cancel"
		}
		if !valid {
			return result, ErrResponse
		}
		if v, ok := fields["feedback"]; ok && (!decode(v, &result.Feedback) || !promptText(result.Feedback, 4096)) {
			return result, ErrResponse
		}
		if v, ok := fields["content"]; ok {
			if result.Action != "accept" || len(v) > 64<<10 || !decode(v, &result.Content) || result.Content == nil {
				return result, ErrResponse
			}
		}
	}
	return result, nil
}

func promptText(value string, limit int) bool {
	return len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

// A decision cannot ambiguously say allow:false and allow:true. Do not let
// encoding/json's last-key-wins rule turn a malformed answer into authority.
func promptAnswerFields(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	d := json.NewDecoder(bytes.NewReader(raw))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return nil, false
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, false
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, false
		}
		fields[key] = value
	}
	end, err := d.Token()
	return fields, err == nil && end == json.Delim('}') && d.Decode(new(any)) == io.EOF
}
