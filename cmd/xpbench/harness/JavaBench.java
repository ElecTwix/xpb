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
 * Timed cross-runtime benchmark harness for the Java runtime (xpbench / T-17).
 *
 * <p>This is the Java arm of the cross-runtime benchmark TABLE driven by
 * {@code cmd/xpbench}. It is the timed analogue of the proven differential
 * runner ({@code tests/diff/DiffRunner.java}): it reads the shared
 * {@code vectors.json} manifest + {@code .bin} corpus the Go reference encoder
 * wrote, then for every vector times an encode loop (re-encode the ops with the
 * Java {@link Encoder}) and a decode loop (decode the {@code .bin} bytes with the
 * Java {@link Decoder}) over a per-vector iteration count, and prints a JSON
 * array of {@code {name, encodeNs, decodeNs, wireSize}} to stdout for the Go
 * driver to normalize into table rows.
 *
 * <p>Usage: {@code java -DxpbCorpusDir=<dir> xpb.JavaBench}
 *
 * <p>The JSON parser + op encode mirror the differential runner; only the driver
 * (timing loops + JSON emit) differs.
 */
public final class JavaBench {

    private JavaBench() {}

    // --------------------------------------------------------------------
    // Minimal JSON parser (objects -> Map, arrays -> List, strings, numbers,
    // bool, null).
    // --------------------------------------------------------------------
    private static final class Json {
        private final String s;
        private int pos;

        Json(String s) { this.s = s; }

        static Object parse(String s) {
            Json j = new Json(s);
            j.skipWs();
            return j.value();
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

    private static String stripHexPrefix(String s) {
        if (s.startsWith("0x") || s.startsWith("0X")) {
            return s.substring(2);
        }
        return s;
    }

    private static long parseLongBits(String hex) {
        return new BigInteger(stripHexPrefix(hex), 16).longValue();
    }

    private static long parseDecimalToLong(String dec) {
        return new BigInteger(dec).longValue();
    }

    private static long asLong(Object o) {
        if (o instanceof BigInteger) {
            return ((BigInteger) o).longValue();
        }
        return ((Number) o).longValue();
    }

    // --------------------------------------------------------------------
    // Encode / decode ops (the timed work)
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

    // decodeOps reads every value (decode-only, matching the Go driver's
    // decode-only path and the other harnesses) and accumulates a small number
    // so the read cannot be elided.
    private static long decodeOps(Decoder dec, List<Object> ops) {
        long acc = 0;
        if (ops == null) {
            return acc;
        }
        for (Object o : ops) {
            acc += decodeOp(dec, asObj(o));
        }
        return acc;
    }

    private static long decodeOp(Decoder dec, Map<String, Object> op) {
        String ty = (String) op.get("type");
        switch (ty) {
            case "bool":
                return dec.readBool() ? 1 : 0;
            case "int32":
                return dec.readInt32() & 0xFFFFFFFFL;
            case "uint32":
                return dec.readUint32() & 0xFFFFFFFFL;
            case "int64":
                return dec.readInt64() & 0xFF;
            case "uint64":
                return dec.readUint64() & 0xFF;
            case "float32":
                return Float.floatToRawIntBits(dec.readFloat32()) & 0xFF;
            case "float64":
                return Double.doubleToRawLongBits(dec.readFloat64()) & 0xFF;
            case "string":
                return dec.readString().length();
            case "bytes":
                return dec.readBytes().length;
            case "array": {
                List<Object> elems = op.containsKey("elems") ? asList(op.get("elems")) : new ArrayList<>();
                long acc = dec.readInt32();
                for (Object el : elems) {
                    acc += decodeOp(dec, asObj(el));
                }
                return acc;
            }
            case "map": {
                List<Object> entries = op.containsKey("entries") ? asList(op.get("entries")) : new ArrayList<>();
                long acc = dec.readInt32();
                for (Object e : entries) {
                    Map<String, Object> ent = asObj(e);
                    acc += decodeOp(dec, asObj(ent.get("k")));
                    acc += decodeOp(dec, asObj(ent.get("v")));
                }
                return acc;
            }
            case "message": {
                byte[] msg = dec.readMessageBytes();
                Decoder inner = new Decoder(msg);
                return decodeOps(inner, op.containsKey("ops") ? asList(op.get("ops")) : null) + msg.length;
            }
            default:
                throw new RuntimeException("unknown op type: " + ty);
        }
    }

    // --------------------------------------------------------------------
    // Main
    // --------------------------------------------------------------------
    public static void main(String[] args) throws IOException {
        String dirProp = System.getProperty("xpbCorpusDir");
        if (dirProp == null) {
            System.err.println("-DxpbCorpusDir=<path> required");
            System.exit(2);
        }

        Path dataDir = Paths.get(dirProp);
        Path manifestPath = dataDir.resolve("vectors.json");
        if (!Files.exists(manifestPath)) {
            System.err.println("manifest not found: " + manifestPath.toAbsolutePath());
            System.exit(2);
        }

        String raw = new String(Files.readAllBytes(manifestPath), StandardCharsets.UTF_8);
        Map<String, Object> manifest = asObj(Json.parse(raw));
        List<Object> vectors = asList(manifest.get("vectors"));
        if (vectors == null || vectors.isEmpty()) {
            System.err.println("manifest has no vectors");
            System.exit(1);
        }

        long sink = 0;
        StringBuilder out = new StringBuilder("[");
        boolean first = true;
        for (Object vo : vectors) {
            Map<String, Object> v = asObj(vo);
            String name = (String) v.get("name");
            String file = (String) v.get("file");
            List<Object> ops = asList(v.get("ops"));
            long iters = Math.max(1, asLong(v.get("iters")));
            long warm = Math.max(1, iters / 10);

            byte[] fileBytes = Files.readAllBytes(dataDir.resolve(file));
            int wire = fileBytes.length;

            // Encode timing.
            for (long i = 0; i < warm; i++) {
                Encoder enc = new Encoder(256);
                encodeOps(enc, ops);
                sink += enc.finish().length;
            }
            long t0 = System.nanoTime();
            for (long i = 0; i < iters; i++) {
                Encoder enc = new Encoder(256);
                encodeOps(enc, ops);
                sink += enc.finish().length;
            }
            double encNs = (double) (System.nanoTime() - t0) / iters;

            // Decode timing.
            for (long i = 0; i < warm; i++) {
                sink += decodeOps(new Decoder(fileBytes), ops);
            }
            t0 = System.nanoTime();
            for (long i = 0; i < iters; i++) {
                sink += decodeOps(new Decoder(fileBytes), ops);
            }
            double decNs = (double) (System.nanoTime() - t0) / iters;

            if (!first) {
                out.append(',');
            }
            first = false;
            out.append(String.format(
                    java.util.Locale.ROOT,
                    "{\"name\":\"%s\",\"encodeNs\":%.3f,\"decodeNs\":%.3f,\"wireSize\":%d}",
                    name, encNs, decNs, wire));
        }
        out.append(']');
        System.out.println(out);
        // Reference the sink so the timed loops are not optimized away.
        if (sink == Long.MIN_VALUE) {
            System.err.println("sink=" + sink);
        }
    }
}
