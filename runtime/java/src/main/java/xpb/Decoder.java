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

    /** Read compact length prefix */
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
        return len;
    }

    /** Read string with compact length prefix */
    public String readString() {
        int len = readCompactLength();
        if (pos + len > length) {
            throw new RuntimeException("xpb: unexpected EOF reading string");
        }
        String v = new String(data, pos, len, java.nio.charset.StandardCharsets.UTF_8);
        pos += len;
        return v;
    }

    /** Read bytes with compact length prefix */
    public byte[] readBytes() {
        int len = readCompactLength();
        if (pos + len > length) {
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

    public static final int COMPACT_LENGTH_THRESHOLD = 254;
    public static final int COMPACT_LENGTH_MARKER = 0xFF;
}
