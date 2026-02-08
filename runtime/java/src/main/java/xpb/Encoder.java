package xpb;

/**
 * XPB V2 Encoder - tagless, fixed-width, compact lengths.
 */
public class Encoder {
    private byte[] buf;
    private int pos;
    private int capacity;

    public Encoder(int initialCapacity) {
        this.capacity = initialCapacity;
        this.buf = new byte[initialCapacity];
        this.pos = 0;
    }

    private void ensureCapacity(int needed) {
        if (pos + needed > capacity) {
            int newCapacity = Math.max(capacity * 2, pos + needed);
            byte[] newBuf = new byte[newCapacity];
            System.arraycopy(buf, 0, newBuf, 0, pos);
            buf = newBuf;
            capacity = newCapacity;
        }
    }

    public byte[] finish() {
        byte[] result = new byte[pos];
        System.arraycopy(buf, 0, result, 0, pos);
        return result;
    }

    public void reset() {
        pos = 0;
    }

    /** Write bool as 1 byte */
    public void writeBool(boolean v) {
        ensureCapacity(1);
        buf[pos++] = (byte) (v ? 1 : 0);
    }

    /** Write int32 as 4 bytes (little-endian, two's complement) */
    public void writeInt32(int v) {
        ensureCapacity(4);
        buf[pos++] = (byte) (v & 0xFF);
        buf[pos++] = (byte) ((v >> 8) & 0xFF);
        buf[pos++] = (byte) ((v >> 16) & 0xFF);
        buf[pos++] = (byte) ((v >> 24) & 0xFF);
    }

    /** Write int64 as 8 bytes (little-endian, two's complement) */
    public void writeInt64(long v) {
        ensureCapacity(8);
        buf[pos++] = (byte) (v & 0xFF);
        buf[pos++] = (byte) ((v >> 8) & 0xFF);
        buf[pos++] = (byte) ((v >> 16) & 0xFF);
        buf[pos++] = (byte) ((v >> 24) & 0xFF);
        buf[pos++] = (byte) ((v >> 32) & 0xFF);
        buf[pos++] = (byte) ((v >> 40) & 0xFF);
        buf[pos++] = (byte) ((v >> 48) & 0xFF);
        buf[pos++] = (byte) ((v >> 56) & 0xFF);
    }

    /** Write uint32 as 4 bytes (little-endian) */
    public void writeUint32(int v) {
        writeInt32(v);
    }

    /** Write uint64 as 8 bytes (little-endian) */
    public void writeUint64(long v) {
        writeInt64(v);
    }

    /** Write float32 as 4 bytes */
    public void writeFloat32(float v) {
        int bits = Float.floatToIntBits(v);
        writeInt32(bits);
    }

    /** Write float64 as 8 bytes */
    public void writeFloat64(double v) {
        long bits = Double.doubleToLongBits(v);
        writeInt64(bits);
    }

    /** Write compact length prefix */
    private void writeCompactLength(int len) {
        if (len <= COMPACT_LENGTH_THRESHOLD) {
            ensureCapacity(1);
            buf[pos++] = (byte) len;
        } else {
            ensureCapacity(5);
            buf[pos++] = (byte) COMPACT_LENGTH_MARKER;
            writeInt32(len);
        }
    }

    /** Write string with compact length prefix */
    public void writeString(String v) {
        byte[] bytes = v.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        writeCompactLength(bytes.length);
        ensureCapacity(bytes.length);
        System.arraycopy(bytes, 0, buf, pos, bytes.length);
        pos += bytes.length;
    }

    /** Write bytes with compact length prefix */
    public void writeBytes(byte[] v) {
        writeCompactLength(v.length);
        ensureCapacity(v.length);
        System.arraycopy(v, 0, buf, pos, v.length);
        pos += v.length;
    }

    /** Write nested message (already encoded) with compact length prefix */
    public void writeMessage(byte[] data) {
        writeBytes(data);
    }

    /** Write array of int32 - format: count (int32) + elements */
    public void writeArrayInt32(int[] arr) {
        writeInt32(arr.length);
        for (int v : arr) {
            writeInt32(v);
        }
    }

    /** Write array of int64 - format: count (int32) + elements */
    public void writeArrayInt64(long[] arr) {
        writeInt32(arr.length);
        for (long v : arr) {
            writeInt64(v);
        }
    }

    /** Write array of uint32 - format: count (int32) + elements */
    public void writeArrayUint32(int[] arr) {
        writeInt32(arr.length);
        for (int v : arr) {
            writeUint32(v);
        }
    }

    /** Write array of uint64 - format: count (int32) + elements */
    public void writeArrayUint64(long[] arr) {
        writeInt32(arr.length);
        for (long v : arr) {
            writeUint64(v);
        }
    }

    /** Write array of float32 - format: count (int32) + elements */
    public void writeArrayFloat32(float[] arr) {
        writeInt32(arr.length);
        for (float v : arr) {
            writeFloat32(v);
        }
    }

    /** Write array of float64 - format: count (int32) + elements */
    public void writeArrayFloat64(double[] arr) {
        writeInt32(arr.length);
        for (double v : arr) {
            writeFloat64(v);
        }
    }

    /** Write array of bool - format: count (int32) + elements */
    public void writeArrayBool(boolean[] arr) {
        writeInt32(arr.length);
        for (boolean v : arr) {
            writeBool(v);
        }
    }

    /** Write array of String - format: count (int32) + elements */
    public void writeArrayString(String[] arr) {
        writeInt32(arr.length);
        for (String v : arr) {
            writeString(v);
        }
    }

    public static final int COMPACT_LENGTH_THRESHOLD = 254;
    public static final int COMPACT_LENGTH_MARKER = 0xFF;
}
