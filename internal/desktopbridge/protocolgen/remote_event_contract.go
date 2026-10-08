package protocolgen

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"reasonix/internal/eventwire"
)

// Unlike the frozen bridge envelope mirrors, this host-only descriptor derives
// from the authoritative Go eventwire payload types and kind registry. It is
// used to validate/project nested payloads before native renderer publication.
const RemoteEventArtifactPath = "desktop/tauri/src/remote_event_contract.generated.json"

type eventShape struct {
	Kind     string                `json:"kind"`
	Nullable bool                  `json:"nullable,omitempty"`
	Fields   map[string]eventField `json:"fields,omitempty"`
	Items    *eventShape           `json:"items,omitempty"`
}
type eventField struct {
	Optional bool       `json:"optional,omitempty"`
	Shape    eventShape `json:"shape"`
}
type eventContract struct {
	Version int        `json:"version"`
	Kinds   []string   `json:"kinds"`
	Event   eventShape `json:"event"`
}

func remoteEventArtifact() (Artifact, error) {
	shape, err := eventTypeShape(reflect.TypeOf(eventwire.Event{}), map[reflect.Type]bool{})
	if err != nil {
		return Artifact{}, err
	}
	data, err := json.MarshalIndent(eventContract{1, eventwire.KindNames(), shape}, "", "  ")
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{Path: RemoteEventArtifactPath, Data: append(data, '\n')}, nil
}

func eventTypeShape(t reflect.Type, stack map[reflect.Type]bool) (eventShape, error) {
	if t == reflect.TypeOf(json.RawMessage{}) {
		return eventShape{Kind: "json", Nullable: true}, nil
	}
	if t.Kind() == reflect.Pointer {
		shape, err := eventTypeShape(t.Elem(), stack)
		shape.Nullable = true
		return shape, err
	}
	if t.Implements(reflect.TypeOf((*json.Marshaler)(nil)).Elem()) || reflect.PointerTo(t).Implements(reflect.TypeOf((*json.Marshaler)(nil)).Elem()) {
		return eventShape{}, fmt.Errorf("event contract custom JSON marshaler %s needs explicit shape", t)
	}
	shape := eventShape{}
	switch t.Kind() {
	case reflect.String:
		shape.Kind = "string"
	case reflect.Bool:
		shape.Kind = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		shape.Kind = "integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		shape.Kind = "unsigned"
	case reflect.Float32, reflect.Float64:
		shape.Kind = "number"
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return eventShape{Kind: "string", Nullable: t.Kind() == reflect.Slice}, nil
		}
		item, err := eventTypeShape(t.Elem(), stack)
		if err != nil {
			return shape, err
		}
		shape.Kind, shape.Items, shape.Nullable = "array", &item, t.Kind() == reflect.Slice
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return shape, fmt.Errorf("unsupported event map %s", t)
		}
		item, err := eventTypeShape(t.Elem(), stack)
		if err != nil {
			return shape, err
		}
		shape.Kind, shape.Items, shape.Nullable = "map", &item, true
	case reflect.Struct:
		if stack[t] {
			return shape, fmt.Errorf("recursive event type %s requires explicit bounded shape", t)
		}
		stack[t] = true
		defer delete(stack, t)
		shape.Kind, shape.Fields = "object", map[string]eventField{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			parts := strings.Split(field.Tag.Get("json"), ",")
			if parts[0] == "-" {
				continue
			}
			if field.Anonymous {
				return shape, fmt.Errorf("embedded event field %s requires explicit shape", field.Name)
			}
			key := parts[0]
			if key == "" {
				key = field.Name
			}
			optional := false
			for _, option := range parts[1:] {
				if option == "omitempty" {
					optional = true
				} else {
					return shape, fmt.Errorf("unsupported event JSON option %q", option)
				}
			}
			child, err := eventTypeShape(field.Type, stack)
			if err != nil {
				return shape, err
			}
			if _, exists := shape.Fields[key]; exists {
				return shape, fmt.Errorf("duplicate event JSON field %q requires explicit shape", key)
			}
			shape.Fields[key] = eventField{Optional: optional, Shape: child}
		}
	default:
		return shape, fmt.Errorf("unsupported event type %s", t)
	}
	return shape, nil
}
