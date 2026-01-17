package cpp

import (
	"fmt"
	"strings"

	xpbast "github.com/anthropic/xpb/pkg/ast"
)

func Generate(file *xpbast.File) ([]byte, error) {
	var sb strings.Builder

	packageName := "xpb"
	if file.Package != "" {
		packageName = file.Package
	}
	guardName := strings.ToUpper(packageName) + "_HPP"

	sb.WriteString("#ifndef " + guardName + "\n")
	sb.WriteString("#define " + guardName + "\n\n")
	sb.WriteString("#include <xpb/xpb.hpp>\n\n")

	for _, msg := range file.Messages {
		writeMessage(&sb, msg, file)
	}

	for _, enum := range file.Enums {
		writeEnum(&sb, enum)
	}

	sb.WriteString("#endif // " + guardName + "\n")

	return []byte(sb.String()), nil
}

func writeMessage(sb *strings.Builder, msg *xpbast.Message, file *xpbast.File) {
	typeName := capitalize(msg.Name)
	sb.WriteString(fmt.Sprintf("struct %s {\n", typeName))

	for _, field := range msg.Fields {
		fieldType := cppType(field.Type, file)
		fieldName := lowercaseFirst(field.Name)
		sb.WriteString(fmt.Sprintf("    %s %s;\n", fieldType, fieldName))
	}

	sb.WriteString("\n")

	writeMarshalMethod(sb, msg, typeName, file)
	writeUnmarshalMethod(sb, msg, typeName, file)

	sb.WriteString("};\n\n")
}

func writeMarshalMethod(sb *strings.Builder, msg *xpbast.Message, typeName string, file *xpbast.File) {
	sb.WriteString(fmt.Sprintf("    std::vector<uint8_t> Marshal() const {\n"))
	sb.WriteString("        xpb::Encoder enc(64);\n")

	for _, field := range msg.Fields {
		fieldName := lowercaseFirst(field.Name)
		switch field.Type.Kind {
		case xpbast.TypeBool:
			sb.WriteString(fmt.Sprintf("        enc.writeBool(%s);\n", fieldName))
		case xpbast.TypeInt32:
			sb.WriteString(fmt.Sprintf("        enc.writeInt32(%s);\n", fieldName))
		case xpbast.TypeInt64:
			sb.WriteString(fmt.Sprintf("        enc.writeInt64(%s);\n", fieldName))
		case xpbast.TypeUint32:
			sb.WriteString(fmt.Sprintf("        enc.writeUint32(%s);\n", fieldName))
		case xpbast.TypeUint64:
			sb.WriteString(fmt.Sprintf("        enc.writeUint64(%s);\n", fieldName))
		case xpbast.TypeFloat32:
			sb.WriteString(fmt.Sprintf("        enc.writeFloat32(%s);\n", fieldName))
		case xpbast.TypeFloat64:
			sb.WriteString(fmt.Sprintf("        enc.writeFloat64(%s);\n", fieldName))
		case xpbast.TypeString:
			sb.WriteString(fmt.Sprintf("        enc.writeString(%s);\n", fieldName))
		case xpbast.TypeBytes:
			sb.WriteString(fmt.Sprintf("        enc.writeBytes(%s);\n", fieldName))
		case xpbast.TypeMessage:
			sb.WriteString(fmt.Sprintf("        enc.writeMessage(%s.Marshal());\n", fieldName))
		case xpbast.TypeEnum:
			sb.WriteString(fmt.Sprintf("        enc.writeInt32(static_cast<int32_t>(%s));\n", fieldName))
		default:
			sb.WriteString(fmt.Sprintf("        enc.writeInt32(static_cast<int32_t>(%s));\n", fieldName))
		}
	}

	sb.WriteString("        return enc.release();\n")
	sb.WriteString("    }\n")
}

func writeUnmarshalMethod(sb *strings.Builder, msg *xpbast.Message, typeName string, file *xpbast.File) {
	sb.WriteString(fmt.Sprintf("    static %s Unmarshal(const uint8_t* data, size_t len) {\n", typeName))
	sb.WriteString("        xpb::Decoder dec(data, len);\n")
	sb.WriteString("        " + typeName + " m;\n")

	for _, field := range msg.Fields {
		fieldName := lowercaseFirst(field.Name)
		switch field.Type.Kind {
		case xpbast.TypeBool:
			sb.WriteString(fmt.Sprintf("        m.%s = dec.readBool();\n", fieldName))
		case xpbast.TypeInt32:
			sb.WriteString(fmt.Sprintf("        m.%s = dec.readInt32();\n", fieldName))
		case xpbast.TypeInt64:
			sb.WriteString(fmt.Sprintf("        m.%s = dec.readInt64();\n", fieldName))
		case xpbast.TypeUint32:
			sb.WriteString(fmt.Sprintf("        m.%s = dec.readUint32();\n", fieldName))
		case xpbast.TypeUint64:
			sb.WriteString(fmt.Sprintf("        m.%s = dec.readUint64();\n", fieldName))
		case xpbast.TypeFloat32:
			sb.WriteString(fmt.Sprintf("        m.%s = dec.readFloat32();\n", fieldName))
		case xpbast.TypeFloat64:
			sb.WriteString(fmt.Sprintf("        m.%s = dec.readFloat64();\n", fieldName))
		case xpbast.TypeString:
			sb.WriteString(fmt.Sprintf("        m.%s = dec.readString();\n", fieldName))
		case xpbast.TypeBytes:
			sb.WriteString(fmt.Sprintf("        m.%s = dec.readBytes();\n", fieldName))
		case xpbast.TypeMessage:
			sb.WriteString(fmt.Sprintf("        m.%s = %s::Unmarshal(dec.readMessageBytes().data(), dec.readMessageBytes().size());\n",
				fieldName, capitalize(field.Type.Message)))
		case xpbast.TypeEnum:
			sb.WriteString(fmt.Sprintf("        m.%s = static_cast<%s>(dec.readInt32());\n", fieldName, cppType(field.Type, file)))
		default:
			sb.WriteString(fmt.Sprintf("        m.%s = static_cast<%s>(dec.readInt32());\n", fieldName, cppType(field.Type, file)))
		}
	}

	sb.WriteString("        return m;\n")
	sb.WriteString("    }\n")
}

func writeEnum(sb *strings.Builder, enum *xpbast.Enum) {
	typeName := capitalize(enum.Name)
	sb.WriteString(fmt.Sprintf("enum class %s {\n", typeName))

	for i, v := range enum.Values {
		if i < len(enum.Values)-1 {
			sb.WriteString(fmt.Sprintf("    %s = %d,\n", uppercaseFirst(v.Name), v.Number))
		} else {
			sb.WriteString(fmt.Sprintf("    %s = %d\n", uppercaseFirst(v.Name), v.Number))
		}
	}

	sb.WriteString("};\n\n")
}

func cppType(t xpbast.FieldType, file *xpbast.File) string {
	switch t.Kind {
	case xpbast.TypeBool:
		return "bool"
	case xpbast.TypeInt32:
		return "int32_t"
	case xpbast.TypeInt64:
		return "int64_t"
	case xpbast.TypeUint32:
		return "uint32_t"
	case xpbast.TypeUint64:
		return "uint64_t"
	case xpbast.TypeFloat32:
		return "float"
	case xpbast.TypeFloat64:
		return "double"
	case xpbast.TypeString:
		return "std::string"
	case xpbast.TypeBytes:
		return "std::vector<uint8_t>"
	case xpbast.TypeMessage:
		return capitalize(t.Message)
	case xpbast.TypeEnum:
		for _, enum := range file.Enums {
			if enum.Name == t.Enum {
				return capitalize(enum.Name)
			}
		}
		return capitalize(t.Enum)
	default:
		return "int32_t"
	}
}

func isMessage(name string, file *xpbast.File) bool {
	for _, msg := range file.Messages {
		if msg.Name == name {
			return true
		}
	}
	return false
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func lowercaseFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func uppercaseFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
