package xpb;

/**
 * XPB V2 Decoder - tagless, fixed-width, compact lengths.
 */
public class Decoder {
    private final byte[] data;
    private final int length;
    private int pos;

    public Decoder(byte[] data) {
        this.data = data;
        this.length = data.length;
        this.pos = 0;
    }

    public boolean eof() {
        return pos >= length;
    }

    public int remaining() {
        return length - pos;
    }

    /** Read bool from 1 byte */
    public boolean readBool() {
        if (pos >= length) {
            throw new RuntimeException("xpb: unexpected EOF reading bool");
        }
        return data[pos++] != 0;
    }

    /** Read int32 from 4 bytes (little-endian, two's complement) */
    public int readInt32() {
        if (pos + 4 > length) {
            throw new RuntimeException("xpb: unexpected EOF reading int32");
        }
        int v = (data[pos] & 0xFF) |
                ((data[pos + 1] & 0xFF) << 8) |
                ((data[pos + 2] & 0xFF) << 16) |
                ((data[pos + 3] & 0xFF) << 24);
        pos += 4;
        return v;
    }

    /** Read int64 from 8 bytes (little-endian, two's complement) */
    public long readInt64() {
        if (pos + 8 > length) {
            throw new RuntimeException("xpb: unexpected EOF reading int64");
        }
        long lo = (data[pos] & 0xFFL) |
                  ((data[pos + 1] & 0xFFL) << 8) |
                  ((data[pos + 2] & 0xFFL) << 16) |
                  ((data[pos + 3] & 0xFFL) << 24);
        long hi = (data[pos + 4] & 0xFFL) |
                  ((data[pos + 5] & 0xFFL) << 8) |
                  ((data[pos + 6] & 0xFFL) << 16) |
                  ((data[pos + 7] & 0xFFL) << 24);
        pos += 8;
        return lo | (hi << 32);
    }

    /** Read uint32 from 4 bytes (little-endian) */
    public int readUint32() {
        return readInt32();
    }

    /** Read uint64 from 8 bytes (little-endian) */
    public long readUint64() {
        return readInt64();
    }

    /** Read float32 from 4 bytes */
    public float readFloat32() {
        int bits = readInt32();
        return Float.intBitsToFloat(bits);
    }

    /** Read float64 from 8 bytes */
    public double readFloat64() {
        long bits = readInt64();
        return Double.longBitsToDouble(bits);
    }

    /** Read compact length prefix. Rejects negative long-form lengths
     *  (the 4-byte int32 path can decode values larger than INT_MAX,
     *  which Java interprets as negative). */
    private int readCompactLength() {
        if (pos >= length) {
            throw new RuntimeException("xpb: unexpected EOF reading length");
        }
        int first = data[pos++] & 0xFF;
        if (first != COMPACT_LENGTH_MARKER) {
            return first;
        }
        if (pos + 4 > length) {
            throw new RuntimeException("xpb: unexpected EOF reading extended length");
        }
        int len = readInt32();
        if (len < 0) {
            throw new RuntimeException("xpb: negative or oversized compact length");
        }
        return len;
    }

    /** Read string with compact length prefix */
    public String readString() {
        int len = readCompactLength();
        if (len > length - pos) {
            throw new RuntimeException("xpb: unexpected EOF reading string");
        }
        String v = new String(data, pos, len, java.nio.charset.StandardCharsets.UTF_8);
        pos += len;
        return v;
    }

    /** Read bytes with compact length prefix */
    public byte[] readBytes() {
        int len = readCompactLength();
        if (len > length - pos) {
            throw new RuntimeException("xpb: unexpected EOF reading bytes");
        }
        byte[] v = new byte[len];
        System.arraycopy(data, pos, v, 0, len);
        pos += len;
        return v;
    }

    /** Read nested message bytes */
    public byte[] readMessageBytes() {
        return readBytes();
    }

    /** Skip n bytes */
    public void skip(int n) {
        if (pos + n > length) {
            throw new RuntimeException("xpb: unexpected EOF during skip");
        }
        pos += n;
    }

    /**
     * Validate and return an array length read from the wire. The decoder
     * does NOT pick a default — the caller MUST pass an explicit maxElements
     * budget so application-level policy is visible at every call site. A
     * count exceeding maxElements, or one that cannot fit in the remaining
     * buffer at the per-element minimum size, is rejected before any
     * allocation happens.
     *
     * @param elementMinBytes smallest on-wire size of one element (e.g. 4
     *   for int32, 1 for bool / variable-length). Pass 0 to skip the buffer
     *   bound (only safe for fully trusted input).
     * @param maxElements caller's hard cap on how many elements this call
     *   site is willing to allocate. MUST be &gt;= 0.
     */
    public int readArrayCount(int elementMinBytes, int maxElements) {
        if (maxElements < 0) {
            throw new IllegalArgumentException("xpb: maxElements must be >= 0");
        }
        int n = readInt32();
        if (n < 0) {
            throw new RuntimeException("xpb: negative array count: " + n);
        }
        if (n > maxElements) {
            throw new RuntimeException("xpb: array count " + n
                + " exceeds caller-supplied max " + maxElements);
        }
        if (elementMinBytes > 0) {
            int max = (length - pos) / elementMinBytes;
            if (n > max) {
                throw new RuntimeException("xpb: array count " + n
                    + " exceeds buffer-bounded max " + max);
            }
        }
        return n;
    }

    /** Read array of int32. Caller MUST provide maxElements. */
    public int[] readArrayInt32(int maxElements) {
        int count = readArrayCount(4, maxElements);
        int[] arr = new int[count];
        for (int i = 0; i < count; i++) {
            arr[i] = readInt32();
        }
        return arr;
    }

    /** Read array of int64. Caller MUST provide maxElements. */
    public long[] readArrayInt64(int maxElements) {
        int count = readArrayCount(8, maxElements);
        long[] arr = new long[count];
        for (int i = 0; i < count; i++) {
            arr[i] = readInt64();
        }
        return arr;
    }

    /** Read array of uint32. Caller MUST provide maxElements. */
    public int[] readArrayUint32(int maxElements) {
        return readArrayInt32(maxElements);
    }

    /** Read array of uint64. Caller MUST provide maxElements. */
    public long[] readArrayUint64(int maxElements) {
        return readArrayInt64(maxElements);
    }

    /** Read array of float32. Caller MUST provide maxElements. */
    public float[] readArrayFloat32(int maxElements) {
        int count = readArrayCount(4, maxElements);
        float[] arr = new float[count];
        for (int i = 0; i < count; i++) {
            arr[i] = readFloat32();
        }
        return arr;
    }

    /** Read array of float64. Caller MUST provide maxElements. */
    public double[] readArrayFloat64(int maxElements) {
        int count = readArrayCount(8, maxElements);
        double[] arr = new double[count];
        for (int i = 0; i < count; i++) {
            arr[i] = readFloat64();
        }
        return arr;
    }

    /** Read array of bool. Caller MUST provide maxElements. */
    public boolean[] readArrayBool(int maxElements) {
        int count = readArrayCount(1, maxElements);
        boolean[] arr = new boolean[count];
        for (int i = 0; i < count; i++) {
            arr[i] = readBool();
        }
        return arr;
    }

    /** Read array of String. Caller MUST provide maxElements (each element
     *  has a 1-byte minimum on the wire — the compact-length prefix for an
     *  empty string). */
    public String[] readArrayString(int maxElements) {
        int count = readArrayCount(1, maxElements);
        String[] arr = new String[count];
        for (int i = 0; i < count; i++) {
            arr[i] = readString();
        }
        return arr;
    }

    public static final int COMPACT_LENGTH_THRESHOLD = 254;
    public static final int COMPACT_LENGTH_MARKER = 0xFF;

    /**
     * Cap on nested-message decode recursion. Matches the Go/TS runtimes
     * (xpb.MaxDecodeDepth / MaxDecodeDepth) so a hand-crafted recursive
     * payload can't blow the JVM stack — the generated unmarshalAt(depth)
     * shim trips on this constant.
     */
    public static final int MAX_DECODE_DEPTH = 64;
}
