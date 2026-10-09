package protocolgen

import (
	"encoding/json"
	"reasonix/internal/eventwire"
	"reflect"
	"testing"
)

func TestRemoteEventContractMatchesGoKindsAndNestedWireShapes(t *testing.T) {
	artifact, err := remoteEventArtifact()
	if err != nil {
		t.Fatal(err)
	}
	var contract eventContract
	if err := json.Unmarshal(artifact.Data, &contract); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contract.Kinds, eventwire.KindNames()) || contract.Version != 1 {
		t.Fatal("event kind contract drifted")
	}
	if contract.Event.Fields["seq"].Shape.Kind != "unsigned" || !contract.Event.Fields["seq"].Optional || contract.Event.Fields["kind"].Optional {
		t.Fatal("event correlation shape drifted")
	}
	tool := contract.Event.Fields["tool"].Shape
	if !tool.Nullable || tool.Fields["args"].Shape.Kind != "string" || tool.Fields["readOnly"].Optional {
		t.Fatal("tool wire shape drifted")
	}
	mcp := contract.Event.Fields["mcpInteraction"].Shape
	if mcp.Fields["requestedSchema"].Shape.Kind != "json" {
		t.Fatal("purpose-specific schema JSON lost")
	}
	shape, err := eventTypeShape(reflect.TypeOf(struct {
		Secret   string `json:"-"`
		Required string `json:"required"`
		Optional *bool  `json:"optional,omitempty"`
	}{}), map[reflect.Type]bool{})
	if err != nil || len(shape.Fields) != 2 || !shape.Fields["optional"].Optional || !shape.Fields["optional"].Shape.Nullable {
		t.Fatal("JSON field rules drifted")
	}
}

func TestRemoteEventContractByteEncodingAndDuplicateFields(t *testing.T) {
	bytes, err := eventTypeShape(reflect.TypeOf([]byte{}), map[reflect.Type]bool{})
	if err != nil || bytes.Kind != "string" || !bytes.Nullable {
		t.Fatal("byte slices must retain JSON base64 encoding")
	}
	array, err := eventTypeShape(reflect.TypeOf([2]byte{}), map[reflect.Type]bool{})
	if err != nil || array.Kind != "array" || array.Nullable || array.Items.Kind != "unsigned" {
		t.Fatal("byte arrays must retain JSON numeric array encoding")
	}
	// Deliberately malformed input is built at runtime so go vet can still
	// validate the production types without rejecting this negative fixture.
	duplicate := reflect.StructOf([]reflect.StructField{
		{Name: "First", Type: reflect.TypeOf(""), Tag: `json:"same"`},
		{Name: "Second", Type: reflect.TypeOf(""), Tag: `json:"same"`},
	})
	_, err = eventTypeShape(duplicate, map[reflect.Type]bool{})
	if err == nil {
		t.Fatal("ambiguous JSON names must require an explicit contract")
	}
}
