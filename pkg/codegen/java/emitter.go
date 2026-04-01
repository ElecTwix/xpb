package java

import (
	"fmt"
	"strings"

	xpbast "github.com/ElecTwix/xpb/pkg/ast"
)

func Generate(file *xpbast.File) ([]byte, error) {
	var sb strings.Builder

	packageName := "xpb"
	if file.Package != "" {
		packageName = toLowerCamel(file.Package)
	}

	sb.WriteString(fmt.Sprintf("package %s;\n\n", packageName))
	sb.WriteString("import xpb.Encoder;\n")
	sb.WriteString("import xpb.Decoder;\n\n")

	for _, msg := range file.Messages {
		writeMessage(&sb, msg, file, packageName)
	}

	for _, enum := range file.Enums {
		writeEnum(&sb, enum, packageName)
	}

	return []byte(sb.String()), nil
}

func writeMessage(sb *strings.Builder, msg *xpbast.Message, file *xpbast.File, packageName string) {
	typeName := capitalize(msg.Name)

	sb.WriteString(fmt.Sprintf("public class %s {\n", typeName))

	for _, field := range msg.Fields {
		fieldType := javaType(field.Type, file)
		fieldName := lowercaseFirst(field.Name)
		sb.WriteString(fmt.Sprintf("    public %s %s;\n", fieldType, fieldName))
	}

	sb.WriteString("\n")

	writeMarshalMethod(sb, msg, typeName, file)
	writeUnmarshalMethod(sb, msg, typeName, packageName)

	sb.WriteString("}\n\n")
}

func writeMarshalMethod(sb *strings.Builder, msg *xpbast.Message, typeName string, file *xpbast.File) {
	sb.WriteString(fmt.Sprintf("    public byte[] marshal() {\n"))
	sb.WriteString("        Encoder enc = new Encoder(64);\n")

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
			sb.WriteString(fmt.Sprintf("        enc.writeMessage(%s.marshal());\n", fieldName))
		case xpbast.TypeEnum:
			sb.WriteString(fmt.Sprintf("        enc.writeInt32(%s);\n", fieldName))
		}
	}

	sb.WriteString("        return enc.finish();\n")
	sb.WriteString("    }\n\n")
}

func writeUnmarshalMethod(sb *strings.Builder, msg *xpbast.Message, typeName string, packageName string) {
	sb.WriteString(fmt.Sprintf("    public static %s unmarshal(byte[] data) {\n", typeName))
	sb.WriteString("        Decoder dec = new Decoder(data);\n")
	sb.WriteString(fmt.Sprintf("        %s m = new %s();\n", typeName, typeName))

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
			sb.WriteString(fmt.Sprintf("        m.%s = %s.unmarshal(dec.readMessageBytes());\n",
				fieldName, capitalize(field.Type.Message)))
		case xpbast.TypeEnum:
			sb.WriteString(fmt.Sprintf("        m.%s = dec.readInt32();\n", fieldName))
		}
	}

	sb.WriteString("        return m;\n")
	sb.WriteString("    }\n\n")
}

func writeEnum(sb *strings.Builder, enum *xpbast.Enum, packageName string) {
	typeName := capitalize(enum.Name)
	sb.WriteString(fmt.Sprintf("public enum %s {\n", typeName))

	for i, v := range enum.Values {
		if i < len(enum.Values)-1 {
			sb.WriteString(fmt.Sprintf("    %s(%d),\n", uppercaseFirst(v.Name), v.Number))
		} else {
			sb.WriteString(fmt.Sprintf("    %s(%d);\n", uppercaseFirst(v.Name), v.Number))
		}
	}

	sb.WriteString("\n")
	sb.WriteString("    private final int value;\n\n")
	sb.WriteString(fmt.Sprintf("    %s(int value) {\n", typeName))
	sb.WriteString("        this.value = value;\n")
	sb.WriteString("    }\n\n")
	sb.WriteString("    public int getValue() {\n")
	sb.WriteString("        return value;\n")
	sb.WriteString("    }\n")
	sb.WriteString("}\n\n")
}

func javaType(t xpbast.FieldType, file *xpbast.File) string {
	switch t.Kind {
	case xpbast.TypeBool:
		return "boolean"
	case xpbast.TypeInt32:
		return "int"
	case xpbast.TypeInt64:
		return "long"
	case xpbast.TypeUint32:
		return "int"
	case xpbast.TypeUint64:
		return "long"
	case xpbast.TypeFloat32:
		return "float"
	case xpbast.TypeFloat64:
		return "double"
	case xpbast.TypeString:
		return "String"
	case xpbast.TypeBytes:
		return "byte[]"
	case xpbast.TypeMessage:
		return capitalize(t.Message)
	case xpbast.TypeEnum:
		return capitalize(t.Enum)
	default:
		return "int"
	}
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

func toLowerCamel(s string) string {
	if s == "" {
		return s
	}
	result := strings.ToLower(s[:1])
	if len(s) > 1 {
		result += s[1:]
	}
	return result
}
