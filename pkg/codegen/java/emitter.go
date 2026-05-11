package java

import (
	"fmt"
	"strings"

	xpbast "github.com/ElecTwix/xpb/pkg/ast"
	"github.com/ElecTwix/xpb/pkg/codegen/common"
)

func Generate(file *xpbast.File) ([]byte, error) {
	var sb strings.Builder
	ctx := xpbast.NewEnumSet(file)

	packageName := "xpb"
	if file.Package != "" {
		packageName = toLowerCamel(file.Package)
	}

	sb.WriteString(fmt.Sprintf("package %s;\n\n", packageName))
	sb.WriteString("import xpb.Encoder;\n")
	sb.WriteString("import xpb.Decoder;\n\n")

	for _, msg := range file.Messages {
		writeMessage(&sb, msg, file, packageName, ctx)
	}

	for _, enum := range file.Enums {
		writeEnum(&sb, enum, packageName)
	}

	return []byte(sb.String()), nil
}

func writeMessage(sb *strings.Builder, msg *xpbast.Message, file *xpbast.File, packageName string, ctx xpbast.EnumSet) {
	typeName := capitalize(msg.Name)

	hasMap := false
	hasOptional := false
	for _, field := range msg.Fields {
		if field.Type.Kind == xpbast.TypeMap {
			hasMap = true
		}
		if field.Optional {
			hasOptional = true
		}
	}
	if hasMap {
		// Need java.util.{Map, HashMap} when any map field is present.
		// Inserted at the bottom of the import block to avoid re-running
		// the package-line generator.
	}

	if hasOptional {
		sb.WriteString("// NOTE: schema contains `optional` fields. The XPB V2 wire format\n")
		sb.WriteString("// has no presence bit, so this codegen emits them as required.\n")
		sb.WriteString("// Callers must agree on a sentinel value (or upgrade to V3) before\n")
		sb.WriteString("// relying on optional semantics.\n")
	}

	sb.WriteString(fmt.Sprintf("public class %s {\n", typeName))

	for _, field := range msg.Fields {
		fieldType := javaType(field.Type, file)
		fieldName := lowercaseFirst(field.Name)
		if field.Repeated {
			fieldType = javaBaseType(field.Type, file) + "[]"
		}
		sb.WriteString(fmt.Sprintf("    public %s %s;\n", fieldType, fieldName))
	}

	sb.WriteString("\n")

	writeMarshalMethod(sb, msg, typeName, file, ctx)
	writeUnmarshalMethod(sb, msg, typeName, packageName, ctx)

	sb.WriteString("}\n\n")
}

func writeMarshalMethod(sb *strings.Builder, msg *xpbast.Message, typeName string, file *xpbast.File, ctx xpbast.EnumSet) {
	sb.WriteString(fmt.Sprintf("    public byte[] marshal() {\n"))
	sb.WriteString("        Encoder enc = new Encoder(64);\n")

	for _, field := range msg.Fields {
		fieldName := lowercaseFirst(field.Name)
		// Enums (which parse as TypeMessage with an enum name) write
		// their int value, not a sub-message.
		if ctx.IsEnum(field.Type) && !field.Repeated {
			sb.WriteString(fmt.Sprintf("        enc.writeInt32(%s.getValue());\n", fieldName))
			continue
		}
		if field.Repeated {
			writeJavaRepeatedFieldEncode(sb, field, fieldName, ctx)
			continue
		}
		if field.Type.Kind == xpbast.TypeMap {
			writeJavaMapFieldEncode(sb, field, fieldName)
			continue
		}
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

func writeUnmarshalMethod(sb *strings.Builder, msg *xpbast.Message, typeName string, packageName string, ctx xpbast.EnumSet) {
	// Public entry point delegates to the depth-threaded helper. Mirrors
	// the Go / TS codegen pattern: every nested call increments `depth`,
	// and the helper compares against Decoder.MAX_DECODE_DEPTH on entry.
	// Without this cap, a hand-crafted recursive payload blows the JVM
	// stack with an uncatchable StackOverflowError.
	sb.WriteString(fmt.Sprintf("    public static %s unmarshal(byte[] data) {\n", typeName))
	sb.WriteString(fmt.Sprintf("        return unmarshalAt(data, 0);\n"))
	sb.WriteString("    }\n\n")

	sb.WriteString(fmt.Sprintf("    public static %s unmarshalAt(byte[] data, int depth) {\n", typeName))
	sb.WriteString("        if (depth > Decoder.MAX_DECODE_DEPTH) {\n")
	sb.WriteString("            throw new RuntimeException(\"xpb: max decode depth exceeded\");\n")
	sb.WriteString("        }\n")
	sb.WriteString("        Decoder dec = new Decoder(data);\n")
	sb.WriteString(fmt.Sprintf("        %s m = new %s();\n", typeName, typeName))

	for _, field := range msg.Fields {
		fieldName := lowercaseFirst(field.Name)
		if ctx.IsEnum(field.Type) && !field.Repeated {
			enumName := field.Type.Message
			if field.Type.Kind == xpbast.TypeEnum {
				enumName = field.Type.Enum
			}
			sb.WriteString(fmt.Sprintf("        m.%s = %s.fromValue(dec.readInt32());\n",
				fieldName, capitalize(enumName)))
			continue
		}
		if field.Repeated {
			writeJavaRepeatedFieldDecode(sb, field, fieldName, ctx)
			continue
		}
		if field.Type.Kind == xpbast.TypeMap {
			writeJavaMapFieldDecode(sb, field, fieldName)
			continue
		}
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
			sb.WriteString(fmt.Sprintf("        m.%s = %s.unmarshalAt(dec.readMessageBytes(), depth + 1);\n",
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
	sb.WriteString("    }\n\n")

	// fromValue: resolve a wire-supplied int to the declared enum constant.
	// Throws IllegalArgumentException on unknown values so schema-drift
	// payloads fail loudly instead of leaving the field at its Java default
	// (null) — which was the silent failure mode of the original codegen.
	sb.WriteString(fmt.Sprintf("    public static %s fromValue(int value) {\n", typeName))
	sb.WriteString(fmt.Sprintf("        for (%s c : values()) if (c.value == value) return c;\n", typeName))
	sb.WriteString(fmt.Sprintf("        throw new IllegalArgumentException(\"xpb: unknown %s value: \" + value);\n", typeName))
	sb.WriteString("    }\n")
	sb.WriteString("}\n\n")
}

func javaType(t xpbast.FieldType, file *xpbast.File) string {
	if t.Kind == xpbast.TypeMap {
		return fmt.Sprintf("java.util.Map<%s, %s>",
			javaBoxedType(*t.KeyType, file), javaBoxedType(*t.ValType, file))
	}
	return javaBaseType(t, file)
}

func javaBaseType(t xpbast.FieldType, file *xpbast.File) string {
	switch t.Kind {
	case xpbast.TypeBool:
		return "boolean"
	case xpbast.TypeInt32, xpbast.TypeUint32:
		return "int"
	case xpbast.TypeInt64, xpbast.TypeUint64:
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

// Boxed forms for map type parameters (`Map<Integer, String>`).
func javaBoxedType(t xpbast.FieldType, file *xpbast.File) string {
	switch t.Kind {
	case xpbast.TypeBool:
		return "Boolean"
	case xpbast.TypeInt32, xpbast.TypeUint32:
		return "Integer"
	case xpbast.TypeInt64, xpbast.TypeUint64:
		return "Long"
	case xpbast.TypeFloat32:
		return "Float"
	case xpbast.TypeFloat64:
		return "Double"
	default:
		return javaBaseType(t, file)
	}
}

func javaReadCall(t xpbast.FieldType) string {
	switch t.Kind {
	case xpbast.TypeBool:
		return "dec.readBool()"
	case xpbast.TypeInt32, xpbast.TypeEnum:
		return "dec.readInt32()"
	case xpbast.TypeInt64:
		return "dec.readInt64()"
	case xpbast.TypeUint32:
		return "dec.readUint32()"
	case xpbast.TypeUint64:
		return "dec.readUint64()"
	case xpbast.TypeFloat32:
		return "dec.readFloat32()"
	case xpbast.TypeFloat64:
		return "dec.readFloat64()"
	case xpbast.TypeString:
		return "dec.readString()"
	case xpbast.TypeBytes:
		return "dec.readBytes()"
	}
	return "0"
}

func javaWriteCall(t xpbast.FieldType, expr string) string {
	switch t.Kind {
	case xpbast.TypeBool:
		return fmt.Sprintf("enc.writeBool(%s)", expr)
	case xpbast.TypeInt32, xpbast.TypeEnum:
		return fmt.Sprintf("enc.writeInt32(%s)", expr)
	case xpbast.TypeInt64:
		return fmt.Sprintf("enc.writeInt64(%s)", expr)
	case xpbast.TypeUint32:
		return fmt.Sprintf("enc.writeUint32(%s)", expr)
	case xpbast.TypeUint64:
		return fmt.Sprintf("enc.writeUint64(%s)", expr)
	case xpbast.TypeFloat32:
		return fmt.Sprintf("enc.writeFloat32(%s)", expr)
	case xpbast.TypeFloat64:
		return fmt.Sprintf("enc.writeFloat64(%s)", expr)
	case xpbast.TypeString:
		return fmt.Sprintf("enc.writeString(%s)", expr)
	case xpbast.TypeBytes:
		return fmt.Sprintf("enc.writeBytes(%s)", expr)
	}
	return ""
}

// javaMinWireBytes is the smallest possible on-wire size of one element of
// the given type. Used by readArrayCount to bound a wire-supplied count.
func javaMinWireBytes(t xpbast.FieldType) int {
	switch t.Kind {
	case xpbast.TypeBool:
		return 1
	case xpbast.TypeInt32, xpbast.TypeUint32, xpbast.TypeFloat32, xpbast.TypeEnum:
		return 4
	case xpbast.TypeInt64, xpbast.TypeUint64, xpbast.TypeFloat64:
		return 8
	case xpbast.TypeString, xpbast.TypeBytes, xpbast.TypeMessage:
		return 1
	}
	return 1
}

func writeJavaRepeatedFieldDecode(sb *strings.Builder, field *xpbast.Field, fieldName string, ctx xpbast.EnumSet) {
	base := javaBaseType(field.Type, nil)
	min := javaMinWireBytes(field.Type)
	if ctx.IsEnum(field.Type) {
		min = 4
	}
	sb.WriteString("        {\n")
	sb.WriteString(fmt.Sprintf("            int count = dec.readArrayCount(%d, %d);\n",
		min, common.DefaultMaxElements))
	sb.WriteString(fmt.Sprintf("            m.%s = new %s[count];\n", fieldName, base))
	sb.WriteString("            for (int i = 0; i < count; i++) {\n")
	if field.Type.Kind == xpbast.TypeMessage {
		sb.WriteString(fmt.Sprintf("                m.%s[i] = %s.unmarshalAt(dec.readMessageBytes(), depth + 1);\n",
			fieldName, capitalize(field.Type.Message)))
	} else {
		sb.WriteString(fmt.Sprintf("                m.%s[i] = %s;\n", fieldName, javaReadCall(field.Type)))
	}
	sb.WriteString("            }\n")
	sb.WriteString("        }\n")
}

func writeJavaMapFieldDecode(sb *strings.Builder, field *xpbast.Field, fieldName string) {
	keyMin := javaMinWireBytes(*field.Type.KeyType)
	valMin := javaMinWireBytes(*field.Type.ValType)
	keyType := javaBoxedType(*field.Type.KeyType, nil)
	valType := javaBoxedType(*field.Type.ValType, nil)
	sb.WriteString("        {\n")
	sb.WriteString(fmt.Sprintf("            int count = dec.readArrayCount(%d, %d);\n",
		keyMin+valMin, common.DefaultMaxElements))
	sb.WriteString(fmt.Sprintf("            m.%s = new java.util.HashMap<%s, %s>();\n", fieldName, keyType, valType))
	sb.WriteString("            for (int i = 0; i < count; i++) {\n")
	sb.WriteString(fmt.Sprintf("                %s k = %s;\n", keyType, javaReadCall(*field.Type.KeyType)))
	sb.WriteString(fmt.Sprintf("                %s v = %s;\n", valType, javaReadCall(*field.Type.ValType)))
	sb.WriteString(fmt.Sprintf("                m.%s.put(k, v);\n", fieldName))
	sb.WriteString("            }\n")
	sb.WriteString("        }\n")
}

func writeJavaRepeatedFieldEncode(sb *strings.Builder, field *xpbast.Field, fieldName string, ctx xpbast.EnumSet) {
	sb.WriteString(fmt.Sprintf("        enc.writeInt32(%s == null ? 0 : %s.length);\n", fieldName, fieldName))
	sb.WriteString(fmt.Sprintf("        if (%s != null) {\n", fieldName))
	if field.Type.Kind == xpbast.TypeMessage {
		base := capitalize(field.Type.Message)
		sb.WriteString(fmt.Sprintf("            for (%s v : %s) {\n", base, fieldName))
		sb.WriteString("                enc.writeMessage(v.marshal());\n")
		sb.WriteString("            }\n")
	} else {
		base := javaBaseType(field.Type, nil)
		sb.WriteString(fmt.Sprintf("            for (%s v : %s) {\n", base, fieldName))
		sb.WriteString(fmt.Sprintf("                %s;\n", javaWriteCall(field.Type, "v")))
		sb.WriteString("            }\n")
	}
	sb.WriteString("        }\n")
}

func writeJavaMapFieldEncode(sb *strings.Builder, field *xpbast.Field, fieldName string) {
	keyType := javaBoxedType(*field.Type.KeyType, nil)
	valType := javaBoxedType(*field.Type.ValType, nil)
	sb.WriteString(fmt.Sprintf("        enc.writeInt32(%s == null ? 0 : %s.size());\n", fieldName, fieldName))
	sb.WriteString(fmt.Sprintf("        if (%s != null) {\n", fieldName))
	sb.WriteString(fmt.Sprintf("            for (java.util.Map.Entry<%s, %s> e : %s.entrySet()) {\n",
		keyType, valType, fieldName))
	sb.WriteString(fmt.Sprintf("                %s;\n", javaWriteCall(*field.Type.KeyType, "e.getKey()")))
	sb.WriteString(fmt.Sprintf("                %s;\n", javaWriteCall(*field.Type.ValType, "e.getValue()")))
	sb.WriteString("            }\n")
	sb.WriteString("        }\n")
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
