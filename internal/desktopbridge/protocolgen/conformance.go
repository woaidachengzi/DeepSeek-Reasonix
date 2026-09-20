package protocolgen

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Definition binds one Go wire DTO to the schema declaration describing it.
type Definition struct {
	Name   string
	Sample any
}

// CheckGoDTOs verifies that each bound Go DTO carries exactly the fields of its
// schema declaration, with matching optionality and JSON-compatible types. Go
// produces the wire format, so this is what keeps the hand-written Go half from
// drifting away from the generated TypeScript and Rust mirrors.
func CheckGoDTOs(root string, definitions []Definition) error {
	defs, err := loadSchema(root)
	if err != nil {
		return err
	}
	c := &checker{defs: defs}
	for _, definition := range definitions {
		raw, ok := defs[definition.Name]
		if !ok {
			return fmt.Errorf("$defs.%s is not declared in %s", definition.Name, SchemaPath)
		}
		if err := c.checkDTO(definition, raw); err != nil {
			return err
		}
	}
	return nil
}

type checker struct {
	defs map[string]json.RawMessage
}

func (c *checker) checkDTO(definition Definition, raw json.RawMessage) error {
	var n node
	if err := json.Unmarshal(raw, &n); err != nil {
		return fmt.Errorf("$defs.%s: %w", definition.Name, err)
	}
	required := make(map[string]bool, len(n.Required))
	for _, key := range n.Required {
		required[key] = true
	}

	typ := reflect.TypeOf(definition.Sample)
	if typ == nil || typ.Kind() != reflect.Struct {
		return fmt.Errorf("%s: sample must be a struct, got %v", definition.Name, typ)
	}

	bound := make(map[string]bool, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		name, options := jsonField(field)
		if name == "" {
			continue
		}
		bound[name] = true
		property, declared := n.Properties[name]
		if !declared {
			return fmt.Errorf("%s: Go field %s serializes as %q, which $defs.%s does not declare",
				definition.Name, field.Name, name, definition.Name)
		}
		optional := strings.Contains(options, "omitempty")
		switch {
		case optional && required[name]:
			return fmt.Errorf("%s: Go field %s is omitempty but $defs.%s requires %q",
				definition.Name, field.Name, definition.Name, name)
		case !optional && !required[name]:
			return fmt.Errorf("%s: Go field %s is always serialized but $defs.%s marks %q optional",
				definition.Name, field.Name, definition.Name, name)
		}
		if err := c.checkType(definition.Name, field.Name, field.Type, property, 0); err != nil {
			return err
		}
	}
	for name := range n.Properties {
		if !bound[name] {
			return fmt.Errorf("$defs.%s declares %q but no Go field serializes it", definition.Name, name)
		}
	}
	return nil
}

const maxRefDepth = 8

func (c *checker) checkType(owner, fieldName string, goType reflect.Type, raw json.RawMessage, depth int) error {
	if depth > maxRefDepth {
		return fmt.Errorf("%s.%s: $ref nesting exceeds %d levels", owner, fieldName, maxRefDepth)
	}
	var n node
	if err := json.Unmarshal(raw, &n); err != nil {
		return fmt.Errorf("%s.%s: decode property: %w", owner, fieldName, err)
	}
	if n.Ref != "" {
		key, err := defKey(n.Ref)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", owner, fieldName, err)
		}
		target, ok := c.defs[key]
		if !ok {
			return fmt.Errorf("%s.%s: $ref %q matches no declaration", owner, fieldName, n.Ref)
		}
		return c.checkType(owner, fieldName, goType, target, depth+1)
	}

	// A string const is a one-value enum; the schema may omit its type.
	if len(n.Const) > 0 && n.Type == "" {
		var literal string
		if json.Unmarshal(n.Const, &literal) == nil {
			n.Type, n.Enum = "string", nil
		}
	}
	if len(n.Enum) > 0 && n.Type == "" {
		n.Type = "string"
	}

	kind := goType.Kind()
	matches := func(kinds ...reflect.Kind) bool {
		for _, want := range kinds {
			if kind == want {
				return true
			}
		}
		return false
	}

	switch n.Type {
	case "string":
		if !matches(reflect.String) {
			return typeMismatch(owner, fieldName, "string", goType)
		}
	case "integer":
		if !matches(reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64) {
			return typeMismatch(owner, fieldName, "integer", goType)
		}
	case "number":
		if !matches(reflect.Float32, reflect.Float64) {
			return typeMismatch(owner, fieldName, "number", goType)
		}
	case "boolean":
		if !matches(reflect.Bool) {
			return typeMismatch(owner, fieldName, "boolean", goType)
		}
	case "array":
		if !matches(reflect.Slice, reflect.Array) {
			return typeMismatch(owner, fieldName, "array", goType)
		}
	case "object":
		// json.RawMessage defers an opaque object to the encoder.
		if goType == rawMessageType {
			return nil
		}
		if !matches(reflect.Struct, reflect.Map) {
			return typeMismatch(owner, fieldName, "object", goType)
		}
	default:
		return fmt.Errorf("%s.%s: unsupported schema type %q", owner, fieldName, n.Type)
	}
	return nil
}

var rawMessageType = reflect.TypeOf(json.RawMessage{})

func typeMismatch(owner, fieldName, want string, goType reflect.Type) error {
	return fmt.Errorf("%s.%s: schema declares %s but the Go type is %s", owner, fieldName, want, goType)
}

func jsonField(field reflect.StructField) (string, string) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", ""
	}
	name, options, _ := strings.Cut(tag, ",")
	if name == "" {
		name = field.Name
	}
	return name, options
}
