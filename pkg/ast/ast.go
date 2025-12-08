// Package ast defines the abstract syntax tree for XPB schema files.
package ast

// File represents a parsed .xpb schema file.
type File struct {
	Package  string
	Messages []*Message
	Enums    []*Enum
}

// Message represents a message definition.
type Message struct {
	Name   string
	Fields []*Field
}

// Field represents a field within a message.
type Field struct {
	Number   uint32
	Name     string
	Type     FieldType
	Optional bool
	Repeated bool // For arrays: []Type
}

// FieldType represents the type of a field.
type FieldType struct {
	Kind    TypeKind
	Message string     // Only set if Kind == TypeMessage
	Enum    string     // Only set if Kind == TypeEnum
	KeyType *FieldType // Only set if Kind == TypeMap
	ValType *FieldType // Only set if Kind == TypeMap
}

// TypeKind represents the kind of a field type.
type TypeKind int

const (
	TypeBool TypeKind = iota
	TypeInt32
	TypeInt64
	TypeUint32
	TypeUint64
	TypeFloat32
	TypeFloat64
	TypeString
	TypeBytes
	TypeMessage // Nested message
	TypeEnum    // Enum type
	TypeMap     // Map type
)

// String returns the string representation of the type kind.
func (k TypeKind) String() string {
	switch k {
	case TypeBool:
		return "bool"
	case TypeInt32:
		return "int32"
	case TypeInt64:
		return "int64"
	case TypeUint32:
		return "uint32"
	case TypeUint64:
		return "uint64"
	case TypeFloat32:
		return "float32"
	case TypeFloat64:
		return "float64"
	case TypeString:
		return "string"
	case TypeBytes:
		return "bytes"
	case TypeMessage:
		return "message"
	case TypeEnum:
		return "enum"
	case TypeMap:
		return "map"
	default:
		return "unknown"
	}
}

// Enum represents an enum definition.
type Enum struct {
	Name   string
	Values []*EnumValue
}

// EnumValue represents a single enum value.
type EnumValue struct {
	Name   string
	Number int32
}

// ParseTypeName parses a type name string into a FieldType.
func ParseTypeName(name string) (FieldType, bool) {
	switch name {
	case "bool":
		return FieldType{Kind: TypeBool}, true
	case "int32":
		return FieldType{Kind: TypeInt32}, true
	case "int64":
		return FieldType{Kind: TypeInt64}, true
	case "uint32":
		return FieldType{Kind: TypeUint32}, true
	case "uint64":
		return FieldType{Kind: TypeUint64}, true
	case "float32":
		return FieldType{Kind: TypeFloat32}, true
	case "float64":
		return FieldType{Kind: TypeFloat64}, true
	case "string":
		return FieldType{Kind: TypeString}, true
	case "bytes":
		return FieldType{Kind: TypeBytes}, true
	default:
		// Assume it's a message type (or enum - will be resolved later)
		return FieldType{Kind: TypeMessage, Message: name}, true
	}
}

// IsScalar returns true if the type is a scalar type (not message/map).
func (t FieldType) IsScalar() bool {
	switch t.Kind {
	case TypeBool, TypeInt32, TypeInt64, TypeUint32, TypeUint64,
		TypeFloat32, TypeFloat64, TypeString, TypeBytes:
		return true
	default:
		return false
	}
}
