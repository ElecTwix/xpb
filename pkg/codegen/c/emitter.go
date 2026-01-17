package c

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
	guardName := strings.ToUpper(packageName) + "_H"

	sb.WriteString("#ifndef " + guardName + "\n")
	sb.WriteString("#define " + guardName + "\n\n")
	sb.WriteString("#include <xpb/xpb.h>\n\n")

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

	sb.WriteString("typedef struct {\n")

	for _, field := range msg.Fields {
		fieldType := cType(field.Type, file)
		fieldName := lowercaseFirst(field.Name)
		sb.WriteString(fmt.Sprintf("    %s %s;\n", fieldType, fieldName))
	}

	sb.WriteString(fmt.Sprintf("}} %s;\n\n", typeName))

	writeMarshalFunction(sb, msg, typeName, file)
	writeUnmarshalFunction(sb, msg, typeName, file)
}

func writeMarshalFunction(sb *strings.Builder, msg *xpbast.Message, typeName string, file *xpbast.File) {
	sb.WriteString(fmt.Sprintf("void %s_marshal(const %s* m, uint8_t** out_data, size_t* out_len) {\n", typeName, typeName))
	sb.WriteString("    struct xpb_encoder* enc = xpb_encoder_create(64);\n")

	for _, field := range msg.Fields {
		fieldName := lowercaseFirst(field.Name)
		switch field.Type.Kind {
		case xpbast.TypeBool:
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_bool(enc, m->%s);\n", fieldName))
		case xpbast.TypeInt32:
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_int32(enc, m->%s);\n", fieldName))
		case xpbast.TypeInt64:
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_int64(enc, m->%s);\n", fieldName))
		case xpbast.TypeUint32:
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_uint32(enc, m->%s);\n", fieldName))
		case xpbast.TypeUint64:
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_uint64(enc, m->%s);\n", fieldName))
		case xpbast.TypeFloat32:
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_float32(enc, m->%s);\n", fieldName))
		case xpbast.TypeFloat64:
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_float64(enc, m->%s);\n", fieldName))
		case xpbast.TypeString:
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_string(enc, m->%s);\n", fieldName))
		case xpbast.TypeBytes:
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_bytes(enc, m->%s, m->%s_len);\n", fieldName, fieldName))
		case xpbast.TypeMessage:
			sb.WriteString(fmt.Sprintf("    size_t %s_len;\n", fieldName))
			sb.WriteString(fmt.Sprintf("    uint8_t* %s_data;\n", fieldName))
			sb.WriteString(fmt.Sprintf("    %s_marshal(m->%s, &%s_data, &%s_len);\n", capitalize(field.Type.Message), fieldName, fieldName, fieldName))
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_message(enc, %s_data, %s_len);\n", fieldName, fieldName))
			sb.WriteString(fmt.Sprintf("    free(%s_data);\n", fieldName))
		case xpbast.TypeEnum:
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_int32(enc, m->%s);\n", fieldName))
		}
	}

	sb.WriteString("    *out_data = xpb_encoder_finish(enc, out_len);\n")
	sb.WriteString("    xpb_encoder_destroy(enc);\n")
	sb.WriteString("}\n\n")
}

func writeUnmarshalFunction(sb *strings.Builder, msg *xpbast.Message, typeName string, file *xpbast.File) {
	sb.WriteString(fmt.Sprintf("%s %s_unmarshal(const uint8_t* data, size_t len) {\n", typeName, typeName))
	sb.WriteString(fmt.Sprintf("    %s m = {0};\n", typeName))
	sb.WriteString("    struct xpb_decoder* dec = xpb_decoder_create(data, len);\n")

	for _, field := range msg.Fields {
		fieldName := lowercaseFirst(field.Name)
		switch field.Type.Kind {
		case xpbast.TypeBool:
			sb.WriteString(fmt.Sprintf("    m.%s = xpb_decoder_read_bool(dec);\n", fieldName))
		case xpbast.TypeInt32:
			sb.WriteString(fmt.Sprintf("    m.%s = xpb_decoder_read_int32(dec);\n", fieldName))
		case xpbast.TypeInt64:
			sb.WriteString(fmt.Sprintf("    m.%s = xpb_decoder_read_int64(dec);\n", fieldName))
		case xpbast.TypeUint32:
			sb.WriteString(fmt.Sprintf("    m.%s = xpb_decoder_read_uint32(dec);\n", fieldName))
		case xpbast.TypeUint64:
			sb.WriteString(fmt.Sprintf("    m.%s = xpb_decoder_read_uint64(dec);\n", fieldName))
		case xpbast.TypeFloat32:
			sb.WriteString(fmt.Sprintf("    m.%s = xpb_decoder_read_float32(dec);\n", fieldName))
		case xpbast.TypeFloat64:
			sb.WriteString(fmt.Sprintf("    m.%s = xpb_decoder_read_float64(dec);\n", fieldName))
		case xpbast.TypeString:
			sb.WriteString(fmt.Sprintf("    m.%s = xpb_decoder_read_string(dec);\n", fieldName))
		case xpbast.TypeBytes:
			sb.WriteString(fmt.Sprintf("    m.%s = xpb_decoder_read_bytes(dec, &m.%s_len);\n", fieldName, fieldName))
		case xpbast.TypeMessage:
			sb.WriteString(fmt.Sprintf("    size_t %s_len;\n", fieldName))
			sb.WriteString(fmt.Sprintf("    uint8_t* %s_data = xpb_decoder_read_message_bytes(dec, &%s_len);\n", fieldName, fieldName))
			sb.WriteString(fmt.Sprintf("    m.%s = %s_unmarshal(%s_data, %s_len);\n", fieldName, capitalize(field.Type.Message), fieldName, fieldName))
			sb.WriteString(fmt.Sprintf("    free(%s_data);\n", fieldName))
		case xpbast.TypeEnum:
			sb.WriteString(fmt.Sprintf("    m.%s = xpb_decoder_read_int32(dec);\n", fieldName))
		}
	}

	sb.WriteString("    xpb_decoder_destroy(dec);\n")
	sb.WriteString("    return m;\n")
	sb.WriteString("}\n\n")
}

func writeEnum(sb *strings.Builder, enum *xpbast.Enum) {
	typeName := capitalize(enum.Name)
	sb.WriteString("typedef enum {\n")

	for i, v := range enum.Values {
		if i < len(enum.Values)-1 {
			sb.WriteString(fmt.Sprintf("    %s_%s = %d,\n", typeName, uppercaseFirst(v.Name), v.Number))
		} else {
			sb.WriteString(fmt.Sprintf("    %s_%s = %d\n", typeName, uppercaseFirst(v.Name), v.Number))
		}
	}

	sb.WriteString(fmt.Sprintf("}} %s;\n\n", typeName))
}

func cType(t xpbast.FieldType, file *xpbast.File) string {
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
		return "char*"
	case xpbast.TypeBytes:
		return "uint8_t*"
	case xpbast.TypeMessage:
		return capitalize(t.Message)
	case xpbast.TypeEnum:
		return "int32_t"
	default:
		return "int32_t"
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
