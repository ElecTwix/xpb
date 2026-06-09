package c

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
		packageName = file.Package
	}
	guardName := strings.ToUpper(packageName) + "_H"

	sb.WriteString("#ifndef " + guardName + "\n")
	sb.WriteString("#define " + guardName + "\n\n")
	sb.WriteString("#include <stdlib.h>\n")
	sb.WriteString("#include <string.h>\n")
	sb.WriteString("#include <xpb/xpb.h>\n\n")

	// Forward declarations for every message so self-referential and
	// mutually-recursive types compile (the field declarations below use
	// pointers to peer types). Enums are emitted before messages so a
	// message field of enum type sees its definition.
	for _, msg := range file.Messages {
		typeName := capitalize(msg.Name)
		sb.WriteString(fmt.Sprintf("typedef struct %s %s;\n", typeName, typeName))
	}
	if len(file.Messages) > 0 {
		sb.WriteString("\n")
	}

	for _, enum := range file.Enums {
		writeEnum(&sb, enum)
	}

	for _, msg := range file.Messages {
		writeMessage(&sb, msg, file, ctx)
	}

	sb.WriteString("#endif // " + guardName + "\n")

	return []byte(sb.String()), nil
}

func writeMessage(sb *strings.Builder, msg *xpbast.Message, file *xpbast.File, ctx xpbast.EnumSet) {
	typeName := capitalize(msg.Name)

	hasOptional := false
	for _, f := range msg.Fields {
		if f.Optional {
			hasOptional = true
			break
		}
	}
	if hasOptional {
		sb.WriteString("/* NOTE: schema contains `optional` fields. On the wire each optional\n")
		sb.WriteString(" * field is preceded by a 1-byte presence flag (0x01 present + value,\n")
		sb.WriteString(" * 0x00 absent). Pointer-typed optionals (string/bytes/message) use\n")
		sb.WriteString(" * NULL for absence; value-typed optionals carry a companion\n")
		sb.WriteString(" * `<field>_present` bool. */\n")
	}

	sb.WriteString(fmt.Sprintf("struct %s {\n", typeName))

	for _, field := range msg.Fields {
		fieldName := lowercaseFirst(field.Name)
		writeFieldDecl(sb, field, fieldName, file, ctx)
	}

	sb.WriteString(fmt.Sprintf("};\n\n"))

	writeMarshalFunction(sb, msg, typeName, file, ctx)
	writeUnmarshalFunction(sb, msg, typeName, file, ctx)
}

// writeFieldDecl emits the struct field for `field`. Recursive types use a
// pointer to break the infinite-size struct problem (`Node next;` is
// uncompilable; `Node* next;` works). Repeated fields gain a paired
// `_count` member. Map fields are pointer-arrays of paired key/value
// arrays since C has no generic map type — the receiving application can
// build whatever container it likes from the parsed elements.
// cOptionalUsesPointer reports whether an optional field of this type already
// signals absence via a NULL pointer (string/bytes/message), so it needs no
// companion presence bool. Value-typed optionals (numbers, bool, enum) get a
// `<field>_present` bool instead.
func cOptionalUsesPointer(field *xpbast.Field, ctx xpbast.EnumSet) bool {
	if ctx.IsEnum(field.Type) {
		return false
	}
	switch field.Type.Kind {
	case xpbast.TypeString, xpbast.TypeBytes, xpbast.TypeMessage:
		return true
	default:
		return false
	}
}

func writeFieldDecl(sb *strings.Builder, field *xpbast.Field, fieldName string, file *xpbast.File, ctx xpbast.EnumSet) {
	// Optional value-typed fields gain a companion presence bool. Pointer-typed
	// optionals (string/bytes/message) use NULL for absence and need no flag.
	if field.Optional && !field.Repeated && field.Type.Kind != xpbast.TypeMap && !cOptionalUsesPointer(field, ctx) {
		sb.WriteString(fmt.Sprintf("    bool %s_present;\n", fieldName))
	}
	// Enum-typed fields are reported as TypeMessage by the parser; collapse
	// them to int32_t (matches Go and the generated C enum which is also
	// int32-compatible).
	if ctx.IsEnum(field.Type) {
		if field.Repeated {
			sb.WriteString(fmt.Sprintf("    int32_t* %s;\n", fieldName))
			sb.WriteString(fmt.Sprintf("    size_t %s_count;\n", fieldName))
			return
		}
		sb.WriteString(fmt.Sprintf("    int32_t %s;\n", fieldName))
		return
	}
	if field.Repeated {
		switch field.Type.Kind {
		case xpbast.TypeString:
			sb.WriteString(fmt.Sprintf("    char** %s;\n", fieldName))
		case xpbast.TypeBytes:
			sb.WriteString(fmt.Sprintf("    uint8_t** %s;\n", fieldName))
			sb.WriteString(fmt.Sprintf("    size_t* %s_lens;\n", fieldName))
		case xpbast.TypeMessage:
			sb.WriteString(fmt.Sprintf("    %s* %s;  /* array of values */\n",
				capitalize(field.Type.Message), fieldName))
		default:
			sb.WriteString(fmt.Sprintf("    %s* %s;\n", cBaseType(field.Type, file), fieldName))
		}
		sb.WriteString(fmt.Sprintf("    size_t %s_count;\n", fieldName))
		return
	}
	if field.Type.Kind == xpbast.TypeMap {
		// Map<K,V>: emit paired flat arrays + count.
		keyT := cBaseType(*field.Type.KeyType, file)
		valT := cBaseType(*field.Type.ValType, file)
		sb.WriteString(fmt.Sprintf("    %s* %s_keys;\n", keyT, fieldName))
		sb.WriteString(fmt.Sprintf("    %s* %s_values;\n", valT, fieldName))
		sb.WriteString(fmt.Sprintf("    size_t %s_count;\n", fieldName))
		return
	}
	if field.Type.Kind == xpbast.TypeMessage {
		// Pointer indirection breaks infinite-size struct definitions
		// (`Node next;` → `Node* next;`).
		sb.WriteString(fmt.Sprintf("    %s* %s;\n", capitalize(field.Type.Message), fieldName))
		return
	}
	if field.Type.Kind == xpbast.TypeBytes {
		sb.WriteString(fmt.Sprintf("    uint8_t* %s;\n", fieldName))
		sb.WriteString(fmt.Sprintf("    size_t %s_len;\n", fieldName))
		return
	}
	sb.WriteString(fmt.Sprintf("    %s %s;\n", cBaseType(field.Type, file), fieldName))
}

func writeMarshalFunction(sb *strings.Builder, msg *xpbast.Message, typeName string, file *xpbast.File, ctx xpbast.EnumSet) {
	sb.WriteString(fmt.Sprintf("static inline void %s_marshal(const %s* m, uint8_t** out_data, size_t* out_len) {\n",
		typeName, typeName))
	sb.WriteString("    struct xpb_encoder* enc = xpb_encoder_create(64);\n")
	sb.WriteString("    if (enc == NULL) { if (out_data) *out_data = NULL; if (out_len) *out_len = 0; return; }\n")

	for _, field := range msg.Fields {
		fieldName := lowercaseFirst(field.Name)
		if field.Optional && !field.Repeated && field.Type.Kind != xpbast.TypeMap {
			writeCOptionalEncode(sb, field, fieldName, ctx)
			continue
		}
		if ctx.IsEnum(field.Type) && !field.Repeated {
			sb.WriteString(fmt.Sprintf("    xpb_encoder_write_int32(enc, (int32_t)m->%s);\n", fieldName))
			continue
		}
		if field.Repeated {
			writeCRepeatedEncode(sb, field, fieldName, ctx)
			continue
		}
		if field.Type.Kind == xpbast.TypeMap {
			writeCMapEncode(sb, field, fieldName)
			continue
		}
		writeCScalarEncode(sb, field, "m->"+fieldName)
	}

	sb.WriteString("    if (out_data) *out_data = xpb_encoder_finish(enc, out_len);\n")
	sb.WriteString("    else if (out_len) *out_len = 0;\n")
	sb.WriteString("    xpb_encoder_destroy(enc);\n")
	sb.WriteString("}\n\n")
}

func writeCScalarEncode(sb *strings.Builder, field *xpbast.Field, ref string) {
	switch field.Type.Kind {
	case xpbast.TypeBool:
		sb.WriteString(fmt.Sprintf("    xpb_encoder_write_bool(enc, %s);\n", ref))
	case xpbast.TypeInt32:
		sb.WriteString(fmt.Sprintf("    xpb_encoder_write_int32(enc, %s);\n", ref))
	case xpbast.TypeInt64:
		sb.WriteString(fmt.Sprintf("    xpb_encoder_write_int64(enc, %s);\n", ref))
	case xpbast.TypeUint32:
		sb.WriteString(fmt.Sprintf("    xpb_encoder_write_uint32(enc, %s);\n", ref))
	case xpbast.TypeUint64:
		sb.WriteString(fmt.Sprintf("    xpb_encoder_write_uint64(enc, %s);\n", ref))
	case xpbast.TypeFloat32:
		sb.WriteString(fmt.Sprintf("    xpb_encoder_write_float32(enc, %s);\n", ref))
	case xpbast.TypeFloat64:
		sb.WriteString(fmt.Sprintf("    xpb_encoder_write_float64(enc, %s);\n", ref))
	case xpbast.TypeString:
		sb.WriteString(fmt.Sprintf("    xpb_encoder_write_string(enc, %s);\n", ref))
	case xpbast.TypeBytes:
		baseRef := strings.TrimPrefix(ref, "m->")
		sb.WriteString(fmt.Sprintf("    xpb_encoder_write_bytes(enc, %s, m->%s_len);\n", ref, baseRef))
	case xpbast.TypeEnum:
		sb.WriteString(fmt.Sprintf("    xpb_encoder_write_int32(enc, (int32_t)%s);\n", ref))
	case xpbast.TypeMessage:
		sb.WriteString("    {\n")
		sb.WriteString(fmt.Sprintf("        if (%s != NULL) {\n", ref))
		sb.WriteString(fmt.Sprintf("            size_t nested_len = 0;\n"))
		sb.WriteString(fmt.Sprintf("            uint8_t* nested_data = NULL;\n"))
		sb.WriteString(fmt.Sprintf("            %s_marshal(%s, &nested_data, &nested_len);\n",
			capitalize(field.Type.Message), ref))
		sb.WriteString(fmt.Sprintf("            xpb_encoder_write_message(enc, nested_data, nested_len);\n"))
		sb.WriteString(fmt.Sprintf("            free(nested_data);\n"))
		sb.WriteString(fmt.Sprintf("        } else {\n"))
		sb.WriteString(fmt.Sprintf("            xpb_encoder_write_message(enc, NULL, 0);\n"))
		sb.WriteString(fmt.Sprintf("        }\n"))
		sb.WriteString("    }\n")
	}
}

// writeCOptionalEncode emits the 1-byte presence flag (see docs/WIRE_FORMAT.md)
// followed by the value only when present. Pointer-typed fields use NULL for
// absence; value-typed fields use their companion `<field>_present` bool.
func writeCOptionalEncode(sb *strings.Builder, field *xpbast.Field, fieldName string, ctx xpbast.EnumSet) {
	var presentExpr string
	if cOptionalUsesPointer(field, ctx) {
		presentExpr = fmt.Sprintf("(m->%s != NULL)", fieldName)
	} else {
		presentExpr = fmt.Sprintf("m->%s_present", fieldName)
	}
	sb.WriteString(fmt.Sprintf("    xpb_encoder_write_bool(enc, %s);\n", presentExpr))
	sb.WriteString(fmt.Sprintf("    if (%s) {\n", presentExpr))
	if ctx.IsEnum(field.Type) {
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_int32(enc, (int32_t)m->%s);\n", fieldName))
	} else {
		// Reuse the scalar encoder for the value (handles string/bytes/message
		// and all numeric kinds).
		writeCScalarEncode(sb, field, "m->"+fieldName)
	}
	sb.WriteString("    }\n")
}

func writeCRepeatedEncode(sb *strings.Builder, field *xpbast.Field, fieldName string, ctx xpbast.EnumSet) {
	sb.WriteString(fmt.Sprintf("    xpb_encoder_write_int32(enc, (int32_t)m->%s_count);\n", fieldName))
	sb.WriteString(fmt.Sprintf("    for (size_t i = 0; i < m->%s_count; i++) {\n", fieldName))
	if ctx.IsEnum(field.Type) {
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_int32(enc, (int32_t)m->%s[i]);\n", fieldName))
		sb.WriteString("    }\n")
		return
	}
	switch field.Type.Kind {
	case xpbast.TypeMessage:
		sb.WriteString(fmt.Sprintf("        size_t nested_len = 0;\n"))
		sb.WriteString(fmt.Sprintf("        uint8_t* nested_data = NULL;\n"))
		sb.WriteString(fmt.Sprintf("        %s_marshal(&m->%s[i], &nested_data, &nested_len);\n",
			capitalize(field.Type.Message), fieldName))
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_message(enc, nested_data, nested_len);\n"))
		sb.WriteString(fmt.Sprintf("        free(nested_data);\n"))
	case xpbast.TypeString:
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_string(enc, m->%s[i]);\n", fieldName))
	case xpbast.TypeBytes:
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_bytes(enc, m->%s[i], m->%s_lens[i]);\n",
			fieldName, fieldName))
	case xpbast.TypeBool:
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_bool(enc, m->%s[i]);\n", fieldName))
	case xpbast.TypeInt32:
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_int32(enc, m->%s[i]);\n", fieldName))
	case xpbast.TypeInt64:
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_int64(enc, m->%s[i]);\n", fieldName))
	case xpbast.TypeUint32:
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_uint32(enc, m->%s[i]);\n", fieldName))
	case xpbast.TypeUint64:
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_uint64(enc, m->%s[i]);\n", fieldName))
	case xpbast.TypeFloat32:
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_float32(enc, m->%s[i]);\n", fieldName))
	case xpbast.TypeFloat64:
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_float64(enc, m->%s[i]);\n", fieldName))
	case xpbast.TypeEnum:
		sb.WriteString(fmt.Sprintf("        xpb_encoder_write_int32(enc, (int32_t)m->%s[i]);\n", fieldName))
	}
	sb.WriteString("    }\n")
}

func writeCMapEncode(sb *strings.Builder, field *xpbast.Field, fieldName string) {
	sb.WriteString(fmt.Sprintf("    xpb_encoder_write_int32(enc, (int32_t)m->%s_count);\n", fieldName))
	sb.WriteString(fmt.Sprintf("    for (size_t i = 0; i < m->%s_count; i++) {\n", fieldName))
	sb.WriteString(fmt.Sprintf("        %s\n", cWriteCall(*field.Type.KeyType, fmt.Sprintf("m->%s_keys[i]", fieldName))))
	sb.WriteString(fmt.Sprintf("        %s\n", cWriteCall(*field.Type.ValType, fmt.Sprintf("m->%s_values[i]", fieldName))))
	sb.WriteString("    }\n")
}

func cWriteCall(t xpbast.FieldType, expr string) string {
	switch t.Kind {
	case xpbast.TypeBool:
		return fmt.Sprintf("xpb_encoder_write_bool(enc, %s);", expr)
	case xpbast.TypeInt32, xpbast.TypeEnum:
		return fmt.Sprintf("xpb_encoder_write_int32(enc, %s);", expr)
	case xpbast.TypeInt64:
		return fmt.Sprintf("xpb_encoder_write_int64(enc, %s);", expr)
	case xpbast.TypeUint32:
		return fmt.Sprintf("xpb_encoder_write_uint32(enc, %s);", expr)
	case xpbast.TypeUint64:
		return fmt.Sprintf("xpb_encoder_write_uint64(enc, %s);", expr)
	case xpbast.TypeFloat32:
		return fmt.Sprintf("xpb_encoder_write_float32(enc, %s);", expr)
	case xpbast.TypeFloat64:
		return fmt.Sprintf("xpb_encoder_write_float64(enc, %s);", expr)
	case xpbast.TypeString:
		return fmt.Sprintf("xpb_encoder_write_string(enc, %s);", expr)
	}
	return ""
}

func cReadCall(t xpbast.FieldType) string {
	switch t.Kind {
	case xpbast.TypeBool:
		return "xpb_decoder_read_bool(dec)"
	case xpbast.TypeInt32, xpbast.TypeEnum:
		return "xpb_decoder_read_int32(dec)"
	case xpbast.TypeInt64:
		return "xpb_decoder_read_int64(dec)"
	case xpbast.TypeUint32:
		return "xpb_decoder_read_uint32(dec)"
	case xpbast.TypeUint64:
		return "xpb_decoder_read_uint64(dec)"
	case xpbast.TypeFloat32:
		return "xpb_decoder_read_float32(dec)"
	case xpbast.TypeFloat64:
		return "xpb_decoder_read_float64(dec)"
	case xpbast.TypeString:
		return "xpb_decoder_read_string(dec)"
	}
	return "0"
}

func cMinWireBytes(t xpbast.FieldType) int {
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

func writeUnmarshalFunction(sb *strings.Builder, msg *xpbast.Message, typeName string, file *xpbast.File, ctx xpbast.EnumSet) {
	// Internal helper threads `depth` so the generated code can refuse
	// adversarial deeply-nested payloads. Public Type_unmarshal delegates
	// to Type_unmarshal_at(out, data, len, 0). The depth is checked at the
	// top of the helper; XPB_MAX_DECODE_DEPTH is defined in xpb.h.
	sb.WriteString(fmt.Sprintf(
		"static inline bool %s_unmarshal_at(%s* out, const uint8_t* data, size_t len, int depth);\n\n",
		typeName, typeName))

	sb.WriteString(fmt.Sprintf("static inline bool %s_unmarshal(%s* out, const uint8_t* data, size_t len) {\n",
		typeName, typeName))
	sb.WriteString(fmt.Sprintf("    return %s_unmarshal_at(out, data, len, 0);\n", typeName))
	sb.WriteString("}\n\n")

	sb.WriteString(fmt.Sprintf("static inline bool %s_unmarshal_at(%s* out, const uint8_t* data, size_t len, int depth) {\n",
		typeName, typeName))
	sb.WriteString("    if (out == NULL) return false;\n")
	sb.WriteString("    if (depth > XPB_MAX_DECODE_DEPTH) return false;\n")
	sb.WriteString(fmt.Sprintf("    %s zero = {0};\n", typeName))
	sb.WriteString("    *out = zero;\n")
	sb.WriteString("    struct xpb_decoder* dec = xpb_decoder_create(data, len);\n")
	sb.WriteString("    if (dec == NULL) return false;\n")
	sb.WriteString("    bool nested_ok = true;\n")

	for _, field := range msg.Fields {
		fieldName := lowercaseFirst(field.Name)
		if field.Optional && !field.Repeated && field.Type.Kind != xpbast.TypeMap {
			writeCOptionalDecode(sb, field, fieldName, ctx)
			continue
		}
		if ctx.IsEnum(field.Type) && !field.Repeated {
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_int32(dec);\n", fieldName))
			continue
		}
		if field.Repeated {
			writeCRepeatedDecode(sb, field, fieldName, ctx)
			continue
		}
		if field.Type.Kind == xpbast.TypeMap {
			writeCMapDecode(sb, field, fieldName)
			continue
		}
		switch field.Type.Kind {
		case xpbast.TypeBool:
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_bool(dec);\n", fieldName))
		case xpbast.TypeInt32:
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_int32(dec);\n", fieldName))
		case xpbast.TypeInt64:
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_int64(dec);\n", fieldName))
		case xpbast.TypeUint32:
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_uint32(dec);\n", fieldName))
		case xpbast.TypeUint64:
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_uint64(dec);\n", fieldName))
		case xpbast.TypeFloat32:
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_float32(dec);\n", fieldName))
		case xpbast.TypeFloat64:
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_float64(dec);\n", fieldName))
		case xpbast.TypeString:
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_string(dec);\n", fieldName))
		case xpbast.TypeBytes:
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_bytes(dec, &out->%s_len);\n",
				fieldName, fieldName))
		case xpbast.TypeMessage:
			sb.WriteString("    {\n")
			sb.WriteString(fmt.Sprintf("        size_t %s_blen = 0;\n", fieldName))
			sb.WriteString(fmt.Sprintf("        uint8_t* %s_data = xpb_decoder_read_message_bytes(dec, &%s_blen);\n",
				fieldName, fieldName))
			sb.WriteString("        if (xpb_decoder_ok(dec)) {\n")
			sb.WriteString(fmt.Sprintf("            out->%s = (%s*)malloc(sizeof(%s));\n",
				fieldName, capitalize(field.Type.Message), capitalize(field.Type.Message)))
			sb.WriteString(fmt.Sprintf("            if (out->%s == NULL) {\n", fieldName))
			sb.WriteString("                nested_ok = false;\n")
			sb.WriteString("            } else {\n")
			sb.WriteString(fmt.Sprintf("                if (!%s_unmarshal_at(out->%s, %s_data, %s_blen, depth + 1)) {\n",
				capitalize(field.Type.Message), fieldName, fieldName, fieldName))
			sb.WriteString("                    nested_ok = false;\n")
			sb.WriteString("                }\n")
			sb.WriteString("            }\n")
			sb.WriteString("        }\n")
			sb.WriteString(fmt.Sprintf("        free(%s_data);\n", fieldName))
			sb.WriteString("    }\n")
		case xpbast.TypeEnum:
			sb.WriteString(fmt.Sprintf("    out->%s = xpb_decoder_read_int32(dec);\n", fieldName))
		}
	}

	sb.WriteString("    bool ok = xpb_decoder_ok(dec) && nested_ok;\n")
	sb.WriteString("    xpb_decoder_destroy(dec);\n")
	sb.WriteString("    return ok;\n")
	sb.WriteString("}\n\n")
}

// writeCOptionalDecode reads the 1-byte presence flag (see docs/WIRE_FORMAT.md)
// and only reads the value when present. On absence value-typed fields keep
// `_present = false` (struct is zero-initialized) and pointer-typed fields keep
// NULL; either way exactly 1 byte is consumed so the next field decodes fine.
func writeCOptionalDecode(sb *strings.Builder, field *xpbast.Field, fieldName string, ctx xpbast.EnumSet) {
	sb.WriteString("    if (xpb_decoder_read_bool(dec)) {\n")
	if ctx.IsEnum(field.Type) {
		sb.WriteString(fmt.Sprintf("        out->%s = xpb_decoder_read_int32(dec);\n", fieldName))
		sb.WriteString(fmt.Sprintf("        out->%s_present = true;\n", fieldName))
		sb.WriteString("    }\n")
		return
	}
	switch field.Type.Kind {
	case xpbast.TypeString:
		sb.WriteString(fmt.Sprintf("        out->%s = xpb_decoder_read_string(dec);\n", fieldName))
	case xpbast.TypeBytes:
		sb.WriteString(fmt.Sprintf("        out->%s = xpb_decoder_read_bytes(dec, &out->%s_len);\n",
			fieldName, fieldName))
	case xpbast.TypeMessage:
		sub := capitalize(field.Type.Message)
		sb.WriteString(fmt.Sprintf("        size_t %s_blen = 0;\n", fieldName))
		sb.WriteString(fmt.Sprintf("        uint8_t* %s_data = xpb_decoder_read_message_bytes(dec, &%s_blen);\n",
			fieldName, fieldName))
		sb.WriteString("        if (xpb_decoder_ok(dec)) {\n")
		sb.WriteString(fmt.Sprintf("            out->%s = (%s*)malloc(sizeof(%s));\n", fieldName, sub, sub))
		sb.WriteString(fmt.Sprintf("            if (out->%s == NULL) {\n", fieldName))
		sb.WriteString("                nested_ok = false;\n")
		sb.WriteString("            } else {\n")
		sb.WriteString(fmt.Sprintf("                if (!%s_unmarshal_at(out->%s, %s_data, %s_blen, depth + 1)) nested_ok = false;\n",
			sub, fieldName, fieldName, fieldName))
		sb.WriteString("            }\n")
		sb.WriteString("        }\n")
		sb.WriteString(fmt.Sprintf("        free(%s_data);\n", fieldName))
	case xpbast.TypeBool:
		sb.WriteString(fmt.Sprintf("        out->%s = xpb_decoder_read_bool(dec);\n", fieldName))
		sb.WriteString(fmt.Sprintf("        out->%s_present = true;\n", fieldName))
	default:
		// Numeric scalar.
		sb.WriteString(fmt.Sprintf("        out->%s = %s;\n", fieldName, cReadCall(field.Type)))
		sb.WriteString(fmt.Sprintf("        out->%s_present = true;\n", fieldName))
	}
	sb.WriteString("    }\n")
}

func writeCRepeatedDecode(sb *strings.Builder, field *xpbast.Field, fieldName string, ctx xpbast.EnumSet) {
	min := cMinWireBytes(field.Type)
	if ctx.IsEnum(field.Type) {
		min = 4
	}
	sb.WriteString("    {\n")
	sb.WriteString("        size_t _count = 0;\n")
	sb.WriteString(fmt.Sprintf("        if (xpb_decoder_validate_array_count(dec, %d, %d, &_count)) {\n",
		min, common.DefaultMaxElements))
	sb.WriteString(fmt.Sprintf("            out->%s_count = _count;\n", fieldName))
	sb.WriteString(fmt.Sprintf("            if (_count > 0) {\n"))

	if ctx.IsEnum(field.Type) {
		sb.WriteString(fmt.Sprintf("                out->%s = (int32_t*)calloc(_count, sizeof(int32_t));\n", fieldName))
		sb.WriteString(fmt.Sprintf("                if (out->%s == NULL) { nested_ok = false; }\n", fieldName))
		sb.WriteString("                else {\n")
		sb.WriteString("                    for (size_t i = 0; i < _count; i++) {\n")
		sb.WriteString(fmt.Sprintf("                        out->%s[i] = xpb_decoder_read_int32(dec);\n", fieldName))
		sb.WriteString("                    }\n")
		sb.WriteString("                }\n")
		sb.WriteString("            }\n")
		sb.WriteString("        } else { nested_ok = false; }\n")
		sb.WriteString("    }\n")
		return
	}

	switch field.Type.Kind {
	case xpbast.TypeString:
		sb.WriteString(fmt.Sprintf("                out->%s = (char**)calloc(_count, sizeof(char*));\n", fieldName))
		sb.WriteString(fmt.Sprintf("                if (out->%s == NULL) { nested_ok = false; }\n", fieldName))
		sb.WriteString("                else {\n")
		sb.WriteString("                    for (size_t i = 0; i < _count; i++) {\n")
		sb.WriteString(fmt.Sprintf("                        out->%s[i] = xpb_decoder_read_string(dec);\n", fieldName))
		sb.WriteString("                    }\n")
		sb.WriteString("                }\n")
	case xpbast.TypeBytes:
		sb.WriteString(fmt.Sprintf("                out->%s = (uint8_t**)calloc(_count, sizeof(uint8_t*));\n", fieldName))
		sb.WriteString(fmt.Sprintf("                out->%s_lens = (size_t*)calloc(_count, sizeof(size_t));\n", fieldName))
		sb.WriteString(fmt.Sprintf("                if (out->%s == NULL || out->%s_lens == NULL) { nested_ok = false; }\n",
			fieldName, fieldName))
		sb.WriteString("                else {\n")
		sb.WriteString("                    for (size_t i = 0; i < _count; i++) {\n")
		sb.WriteString(fmt.Sprintf("                        out->%s[i] = xpb_decoder_read_bytes(dec, &out->%s_lens[i]);\n",
			fieldName, fieldName))
		sb.WriteString("                    }\n")
		sb.WriteString("                }\n")
	case xpbast.TypeMessage:
		sub := capitalize(field.Type.Message)
		sb.WriteString(fmt.Sprintf("                out->%s = (%s*)calloc(_count, sizeof(%s));\n", fieldName, sub, sub))
		sb.WriteString(fmt.Sprintf("                if (out->%s == NULL) { nested_ok = false; }\n", fieldName))
		sb.WriteString("                else {\n")
		sb.WriteString("                    for (size_t i = 0; i < _count; i++) {\n")
		sb.WriteString(fmt.Sprintf("                        size_t nlen = 0;\n"))
		sb.WriteString(fmt.Sprintf("                        uint8_t* ndata = xpb_decoder_read_message_bytes(dec, &nlen);\n"))
		sb.WriteString(fmt.Sprintf("                        if (xpb_decoder_ok(dec)) {\n"))
		sb.WriteString(fmt.Sprintf("                            if (!%s_unmarshal_at(&out->%s[i], ndata, nlen, depth + 1)) nested_ok = false;\n",
			sub, fieldName))
		sb.WriteString(fmt.Sprintf("                        }\n"))
		sb.WriteString(fmt.Sprintf("                        free(ndata);\n"))
		sb.WriteString("                    }\n")
		sb.WriteString("                }\n")
	default:
		base := cBaseType(field.Type, nil)
		sb.WriteString(fmt.Sprintf("                out->%s = (%s*)calloc(_count, sizeof(%s));\n", fieldName, base, base))
		sb.WriteString(fmt.Sprintf("                if (out->%s == NULL) { nested_ok = false; }\n", fieldName))
		sb.WriteString("                else {\n")
		sb.WriteString("                    for (size_t i = 0; i < _count; i++) {\n")
		sb.WriteString(fmt.Sprintf("                        out->%s[i] = %s;\n", fieldName, cReadCall(field.Type)))
		sb.WriteString("                    }\n")
		sb.WriteString("                }\n")
	}

	sb.WriteString("            }\n")
	sb.WriteString("        } else { nested_ok = false; }\n")
	sb.WriteString("    }\n")
}

func writeCMapDecode(sb *strings.Builder, field *xpbast.Field, fieldName string) {
	keyMin := cMinWireBytes(*field.Type.KeyType)
	valMin := cMinWireBytes(*field.Type.ValType)
	keyType := cBaseType(*field.Type.KeyType, nil)
	valType := cBaseType(*field.Type.ValType, nil)
	sb.WriteString("    {\n")
	sb.WriteString("        size_t _count = 0;\n")
	sb.WriteString(fmt.Sprintf("        if (xpb_decoder_validate_array_count(dec, %d, %d, &_count)) {\n",
		keyMin+valMin, common.DefaultMaxElements))
	sb.WriteString(fmt.Sprintf("            out->%s_count = _count;\n", fieldName))
	sb.WriteString("            if (_count > 0) {\n")
	sb.WriteString(fmt.Sprintf("                out->%s_keys = (%s*)calloc(_count, sizeof(%s));\n", fieldName, keyType, keyType))
	sb.WriteString(fmt.Sprintf("                out->%s_values = (%s*)calloc(_count, sizeof(%s));\n", fieldName, valType, valType))
	sb.WriteString(fmt.Sprintf("                if (out->%s_keys == NULL || out->%s_values == NULL) { nested_ok = false; }\n",
		fieldName, fieldName))
	sb.WriteString("                else {\n")
	sb.WriteString("                    for (size_t i = 0; i < _count; i++) {\n")
	sb.WriteString(fmt.Sprintf("                        out->%s_keys[i] = %s;\n", fieldName, cReadCall(*field.Type.KeyType)))
	sb.WriteString(fmt.Sprintf("                        out->%s_values[i] = %s;\n", fieldName, cReadCall(*field.Type.ValType)))
	sb.WriteString("                    }\n")
	sb.WriteString("                }\n")
	sb.WriteString("            }\n")
	sb.WriteString("        } else { nested_ok = false; }\n")
	sb.WriteString("    }\n")
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

	sb.WriteString(fmt.Sprintf("} %s;\n\n", typeName))
}

func cType(t xpbast.FieldType, file *xpbast.File) string {
	return cBaseType(t, file)
}

func cBaseType(t xpbast.FieldType, file *xpbast.File) string {
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
