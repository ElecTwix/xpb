package xpb;

import java.io.IOException;
import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * XPB V2 Java cross-language conformance test.
 *
 * <p>Reads the shared {@code .bin} vectors and {@code vectors.json} manifest
 * produced by the Go reference encoder (testdata/conformance/), decodes each
 * with the Java runtime ({@link Encoder} / {@link Decoder}), asserts the decoded
 * values match the manifest, then re-encodes and asserts the bytes are
 * byte-identical to the {@code .bin} file. Mirrors the Go, Rust and TS
 * conformance harnesses.
 *
 * <p>Value model (see manifest "format" field):
 * <ul>
 *   <li>int32/uint32: JSON number</li>
 *   <li>int64/uint64: decimal string</li>
 *   <li>float32/float64: hex bit-pattern string (e.g. "0x7FF0000000000000")</li>
 *   <li>bytes: lowercase hex string</li>
 *   <li>array: {elemType, elems:[...]} -&gt; int32 count + elements</li>
 *   <li>map: {keyType, valType, entries:[{k,v}]} -&gt; int32 count + k/v pairs</li>
 *   <li>message: {ops:[...]} -&gt; length-prefixed nested ops</li>
 * </ul>
 *
 * <p>Floats are compared by IEEE-754 bit pattern, so NaN, -0.0 and +/-inf are
 * verified exactly. The byte-identity re-encode check is the ultimate
 * cross-language guarantee. No external JSON dependency: a minimal parser is
 * embedded below.
 */
public class ConformanceTest {

    private static int passed = 0;
    private static int failed = 0;

    // --------------------------------------------------------------------
    // Minimal JSON parser (objects -> Map, arrays -> List, strings, numbers,
    // bool, null). Sufficient for the conformance manifest.
    // --------------------------------------------------------------------
    private static final class Json {
        private final String s;
        private int pos;

        Json(String s) { this.s = s; }

        static Object parse(String s) {
            Json j = new Json(s);
            j.skipWs();
            Object v = j.value();
            return v;
        }

        private void skipWs() {
            while (pos < s.length()) {
                char c = s.charAt(pos);
                if (c == ' ' || c == '\t' || c == '\n' || c == '\r') {
                    pos++;
                } else {
                    break;
                }
            }
        }

        private Object value() {
            skipWs();
            char c = s.charAt(pos);
            switch (c) {
                case '{': return object();
                case '[': return array();
                case '"': return string();
                case 't': expect("true"); return Boolean.TRUE;
                case 'f': expect("false"); return Boolean.FALSE;
                case 'n': expect("null"); return null;
                default:  return number();
            }
        }

        private void expect(String lit) {
            if (!s.startsWith(lit, pos)) {
                throw new RuntimeException("json: expected " + lit + " at " + pos);
            }
            pos += lit.length();
        }

        private Map<String, Object> object() {
            Map<String, Object> obj = new LinkedHashMap<>();
            pos++; // {
            skipWs();
            if (s.charAt(pos) == '}') { pos++; return obj; }
            while (true) {
                skipWs();
                String key = string();
                skipWs();
                if (s.charAt(pos) != ':') {
                    throw new RuntimeException("json: expected : at " + pos);
                }
                pos++;
                obj.put(key, value());
                skipWs();
                char c = s.charAt(pos);
                if (c == ',') { pos++; }
                else if (c == '}') { pos++; return obj; }
                else { throw new RuntimeException("json: expected , or } at " + pos); }
            }
        }

        private List<Object> array() {
            List<Object> arr = new ArrayList<>();
            pos++; // [
            skipWs();
            if (s.charAt(pos) == ']') { pos++; return arr; }
            while (true) {
                arr.add(value());
                skipWs();
                char c = s.charAt(pos);
                if (c == ',') { pos++; }
                else if (c == ']') { pos++; return arr; }
                else { throw new RuntimeException("json: expected , or ] at " + pos); }
            }
        }

        private String string() {
            if (s.charAt(pos) != '"') {
                throw new RuntimeException("json: expected string at " + pos);
            }
            pos++;
            StringBuilder sb = new StringBuilder();
            while (pos < s.length()) {
                char c = s.charAt(pos++);
                if (c == '"') {
                    return sb.toString();
                } else if (c == '\\') {
                    char e = s.charAt(pos++);
                    switch (e) {
                        case 'n': sb.append('\n'); break;
                        case 't': sb.append('\t'); break;
                        case 'r': sb.append('\r'); break;
                        case 'b': sb.append('\b'); break;
                        case 'f': sb.append('\f'); break;
                        case '/': sb.append('/'); break;
                        case '\\': sb.append('\\'); break;
                        case '"': sb.append('"'); break;
                        case 'u':
                            int cp = Integer.parseInt(s.substring(pos, pos + 4), 16);
                            pos += 4;
                            sb.append((char) cp);
                            break;
                        default:
                            throw new RuntimeException("json: bad escape \\" + e);
                    }
                } else {
                    sb.append(c);
                }
            }
            throw new RuntimeException("json: unterminated string");
        }

        private Object number() {
            int start = pos;
            while (pos < s.length()) {
                char c = s.charAt(pos);
                if ((c >= '0' && c <= '9') || c == '-' || c == '+'
                        || c == '.' || c == 'e' || c == 'E') {
                    pos++;
                } else {
                    break;
                }
            }
            String num = s.substring(start, pos);
            if (num.indexOf('.') >= 0 || num.indexOf('e') >= 0 || num.indexOf('E') >= 0) {
                return Double.parseDouble(num);
            }
            // Use BigInteger so values up to uint32 max (4294967295) survive,
            // then narrow at the use site.
            return new BigInteger(num);
        }
    }

    @SuppressWarnings("unchecked")
    private static Map<String, Object> asObj(Object o) { return (Map<String, Object>) o; }

    @SuppressWarnings("unchecked")
    private static List<Object> asList(Object o) { return (List<Object>) o; }

    // --------------------------------------------------------------------
    // Helpers
    // --------------------------------------------------------------------
    private static byte[] hexToBytes(String hex) {
        if (hex == null || hex.isEmpty()) {
            return new byte[0];
        }
        int n = hex.length() / 2;
        byte[] out = new byte[n];
        for (int i = 0; i < n; i++) {
            out[i] = (byte) Integer.parseInt(hex.substring(i * 2, i * 2 + 2), 16);
        }
        return out;
    }

    private static String bytesToHex(byte[] b) {
        StringBuilder sb = new StringBuilder(b.length * 2);
        for (byte x : b) {
            sb.append(String.format("%02x", x & 0xFF));
        }
        return sb.toString();
    }

    private static String stripHexPrefix(String s) {
        if (s.startsWith("0x") || s.startsWith("0X")) {
            return s.substring(2);
        }
        return s;
    }

    private static long parseLongBits(String hex) {
        // Up to 16 hex digits => use BigInteger then take the low 64 bits as a
        // two's-complement long (e.g. 0xFFFFFFFFFFFFFFFF -> -1).
        return new BigInteger(stripHexPrefix(hex), 16).longValue();
    }

    private static long parseDecimalToLong(String dec) {
        // Decimal string possibly exceeding Long range (uint64 max); take the
        // low 64 bits as a two's-complement long, matching the wire encoding.
        return new BigInteger(dec).longValue();
    }

    // Verification context: accumulates failure messages per vector.
    private static final class Ctx {
        boolean ok = true;
        final List<String> msgs = new ArrayList<>();
        void fail(String path, String msg) {
            ok = false;
            msgs.add("    " + path + ": " + msg);
        }
        void check(String path, boolean cond, String msg) {
            if (!cond) {
                fail(path, msg);
            }
        }
    }

    // --------------------------------------------------------------------
    // Encode / verify ops (recursive, matching the reference harnesses)
    // --------------------------------------------------------------------
    private static void encodeOps(Encoder enc, List<Object> ops) {
        if (ops == null) {
            return;
        }
        for (Object o : ops) {
            encodeOp(enc, asObj(o));
        }
    }

    private static void encodeOp(Encoder enc, Map<String, Object> op) {
        String ty = (String) op.get("type");
        switch (ty) {
            case "bool":
                enc.writeBool(Boolean.TRUE.equals(op.get("bool")));
                break;
            case "int32":
                enc.writeInt32(((BigInteger) op.get("int32")).intValue());
                break;
            case "uint32":
                enc.writeUint32((int) ((BigInteger) op.get("uint32")).longValue());
                break;
            case "int64":
                enc.writeInt64(parseDecimalToLong((String) op.get("int64")));
                break;
            case "uint64":
                enc.writeUint64(parseDecimalToLong((String) op.get("uint64")));
                break;
            case "float32":
                enc.writeFloat32(Float.intBitsToFloat(
                        (int) Long.parseLong(stripHexPrefix((String) op.get("floatBits")), 16)));
                break;
            case "float64":
                enc.writeFloat64(Double.longBitsToDouble(
                        parseLongBits((String) op.get("floatBits"))));
                break;
            case "string": {
                String v = (String) op.get("string");
                enc.writeString(v == null ? "" : v);
                break;
            }
            case "bytes":
                enc.writeBytes(hexToBytes((String) op.get("bytes")));
                break;
            case "array": {
                List<Object> elems = op.containsKey("elems") ? asList(op.get("elems")) : new ArrayList<>();
                enc.writeInt32(elems.size());
                for (Object el : elems) {
                    encodeOp(enc, asObj(el));
                }
                break;
            }
            case "map": {
                List<Object> entries = op.containsKey("entries") ? asList(op.get("entries")) : new ArrayList<>();
                enc.writeInt32(entries.size());
                for (Object e : entries) {
                    Map<String, Object> ent = asObj(e);
                    encodeOp(enc, asObj(ent.get("k")));
                    encodeOp(enc, asObj(ent.get("v")));
                }
                break;
            }
            case "message": {
                Encoder inner = new Encoder(64);
                encodeOps(inner, op.containsKey("ops") ? asList(op.get("ops")) : null);
                enc.writeMessage(inner.finish());
                break;
            }
            default:
                throw new RuntimeException("unknown op type: " + ty);
        }
    }

    private static void verifyOps(Decoder dec, List<Object> ops, String path, Ctx ctx) {
        if (ops == null) {
            return;
        }
        for (int i = 0; i < ops.size(); i++) {
            verifyOp(dec, asObj(ops.get(i)), path + "[" + i + "]", ctx);
        }
    }

    private static void verifyOp(Decoder dec, Map<String, Object> op, String path, Ctx ctx) {
        String ty = (String) op.get("type");
        switch (ty) {
            case "bool":
                ctx.check(path, dec.readBool() == Boolean.TRUE.equals(op.get("bool")), "bool mismatch");
                break;
            case "int32": {
                int got = dec.readInt32();
                int want = ((BigInteger) op.get("int32")).intValue();
                ctx.check(path, got == want, "int32 got " + got + " want " + want);
                break;
            }
            case "uint32": {
                int got = dec.readUint32();
                int want = (int) ((BigInteger) op.get("uint32")).longValue();
                ctx.check(path, got == want, "uint32 bits got "
                        + Integer.toHexString(got) + " want " + Integer.toHexString(want));
                break;
            }
            case "int64": {
                long got = dec.readInt64();
                long want = parseDecimalToLong((String) op.get("int64"));
                ctx.check(path, got == want, "int64 got " + got + " want " + want);
                break;
            }
            case "uint64": {
                long got = dec.readUint64();
                long want = parseDecimalToLong((String) op.get("uint64"));
                ctx.check(path, got == want, "uint64 bit mismatch");
                break;
            }
            case "float32": {
                int got = Float.floatToRawIntBits(dec.readFloat32());
                int want = (int) Long.parseLong(stripHexPrefix((String) op.get("floatBits")), 16);
                ctx.check(path, got == want, "float32 bits got "
                        + String.format("%08x", got) + " want " + String.format("%08x", want));
                break;
            }
            case "float64": {
                long got = Double.doubleToRawLongBits(dec.readFloat64());
                long want = parseLongBits((String) op.get("floatBits"));
                ctx.check(path, got == want, "float64 bits got "
                        + String.format("%016x", got) + " want " + String.format("%016x", want));
                break;
            }
            case "string": {
                String got = dec.readString();
                String want = (String) op.get("string");
                if (want == null) {
                    want = "";
                }
                ctx.check(path, got.equals(want), "string mismatch");
                break;
            }
            case "bytes": {
                byte[] got = dec.readBytes();
                byte[] want = hexToBytes((String) op.get("bytes"));
                ctx.check(path, java.util.Arrays.equals(got, want), "bytes mismatch");
                break;
            }
            case "array": {
                int count = dec.readInt32();
                List<Object> elems = op.containsKey("elems") ? asList(op.get("elems")) : new ArrayList<>();
                ctx.check(path, count == elems.size(), "array count got " + count + " want " + elems.size());
                for (int i = 0; i < elems.size(); i++) {
                    verifyOp(dec, asObj(elems.get(i)), path + ".elem[" + i + "]", ctx);
                }
                break;
            }
            case "map": {
                int count = dec.readInt32();
                List<Object> entries = op.containsKey("entries") ? asList(op.get("entries")) : new ArrayList<>();
                ctx.check(path, count == entries.size(), "map count got " + count + " want " + entries.size());
                for (int i = 0; i < entries.size(); i++) {
                    Map<String, Object> ent = asObj(entries.get(i));
                    verifyOp(dec, asObj(ent.get("k")), path + ".key[" + i + "]", ctx);
                    verifyOp(dec, asObj(ent.get("v")), path + ".val[" + i + "]", ctx);
                }
                break;
            }
            case "message": {
                byte[] msg = dec.readMessageBytes();
                Decoder inner = new Decoder(msg);
                verifyOps(inner, op.containsKey("ops") ? asList(op.get("ops")) : null, path + ".msg", ctx);
                ctx.check(path, inner.eof(), "nested message trailing bytes");
                break;
            }
            default:
                ctx.fail(path, "unknown op type: " + ty);
        }
    }

    // --------------------------------------------------------------------
    // Main
    // --------------------------------------------------------------------
    public static void main(String[] args) throws IOException {
        System.out.println("===========================================");
        System.out.println("XPB V2 Java Conformance (shared vectors)");
        System.out.println("===========================================");
        System.out.println("Java: " + System.getProperty("java.version"));

        // Allow override via -DxpbConformanceDir, else derive from a system
        // property or default to the repo layout relative to cwd.
        String dirProp = System.getProperty("xpbConformanceDir");
        Path dataDir = dirProp != null
                ? Paths.get(dirProp)
                : Paths.get("testdata", "conformance");
        Path manifestPath = dataDir.resolve("vectors.json");

        if (!Files.exists(manifestPath)) {
            System.out.println("[SKIP] manifest not found: " + manifestPath.toAbsolutePath());
            System.out.println("       run `XPB_GEN=1 go test ./tests/conformance/ "
                    + "-run TestGenerateVectors` first, or pass -DxpbConformanceDir=<path>");
            return; // exit 0 - graceful skip
        }

        String raw = new String(Files.readAllBytes(manifestPath), StandardCharsets.UTF_8);
        Map<String, Object> manifest = asObj(Json.parse(raw));
        List<Object> vectors = asList(manifest.get("vectors"));
        if (vectors == null || vectors.isEmpty()) {
            System.out.println("[FAIL] manifest has no vectors");
            System.exit(1);
        }

        for (Object vo : vectors) {
            Map<String, Object> v = asObj(vo);
            String name = (String) v.get("name");
            String file = (String) v.get("file");
            List<Object> ops = asList(v.get("ops"));
            Ctx ctx = new Ctx();

            Path binPath = dataDir.resolve(file);
            byte[] fileBytes;
            try {
                fileBytes = Files.readAllBytes(binPath);
            } catch (IOException e) {
                ctx.fail(name, "missing .bin file: " + file);
                report(name, ctx);
                continue;
            }

            // 1. Manifest hex must equal the .bin bytes.
            byte[] wantHex = hexToBytes((String) v.get("hex"));
            ctx.check(name, java.util.Arrays.equals(fileBytes, wantHex),
                    "manifest hex != .bin bytes\n      hex:  " + v.get("hex")
                            + "\n      file: " + bytesToHex(fileBytes));

            // 2. Decode + verify values bit-exactly.
            try {
                Decoder dec = new Decoder(fileBytes);
                verifyOps(dec, ops, name, ctx);
                ctx.check(name, dec.eof(), "trailing bytes after decode");
            } catch (RuntimeException e) {
                ctx.fail(name, "decode error: " + e);
            }

            // 3. Re-encode from ops and assert byte-identity.
            try {
                Encoder enc = new Encoder(256);
                encodeOps(enc, ops);
                byte[] reencoded = enc.finish();
                ctx.check(name, java.util.Arrays.equals(reencoded, fileBytes),
                        "re-encode mismatch\n      got:  " + bytesToHex(reencoded)
                                + "\n      want: " + bytesToHex(fileBytes));
            } catch (RuntimeException e) {
                ctx.fail(name, "encode error: " + e);
            }

            report(name, ctx);
        }

        System.out.println("\n===========================================");
        System.out.println("Conformance: " + passed + " passed, " + failed
                + " failed (" + (passed + failed) + " vectors)");
        System.out.println("===========================================");

        if (failed > 0) {
            System.exit(1);
        }
    }

    private static void report(String name, Ctx ctx) {
        if (ctx.ok) {
            System.out.println("  [PASS] " + name);
            passed++;
        } else {
            System.out.println("  [FAIL] " + name);
            for (String m : ctx.msgs) {
                System.out.println(m);
            }
            failed++;
        }
    }
}
