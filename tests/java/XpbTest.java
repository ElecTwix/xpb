package xpb;

public class XpbTest {
    private static int testsPassed = 0;
    private static int testsFailed = 0;

    private static void test(String name, boolean cond) {
        if (cond) {
            System.out.println("  [PASS] " + name);
            testsPassed++;
        } else {
            System.out.println("  [FAIL] " + name);
            testsFailed++;
        }
    }

    private static void testBool() {
        System.out.println("\n=== Test Bool ===");
        Encoder enc = new Encoder(64);
        enc.writeBool(true);
        enc.writeBool(false);
        byte[] data = enc.finish();

        Decoder dec = new Decoder(data);
        test("bool true", dec.readBool() == true);
        test("bool false", dec.readBool() == false);
    }

    private static void testInt32() {
        System.out.println("\n=== Test Int32 ===");
        int[] values = {0, 1, -1, 100, -100, 2147483647, -2147483648};
        Encoder enc = new Encoder(256);
        for (int v : values) {
            enc.writeInt32(v);
        }
        byte[] data = enc.finish();

        Decoder dec = new Decoder(data);
        for (int i = 0; i < values.length; i++) {
            test("int32 value " + values[i], dec.readInt32() == values[i]);
        }
    }

    private static void testInt64() {
        System.out.println("\n=== Test Int64 ===");
        long[] values = {0L, 1L, -1L, 1000000000L, -1000000000L, 9223372036854775807L, -9223372036854775807L};
        Encoder enc = new Encoder(256);
        for (long v : values) {
            enc.writeInt64(v);
        }
        byte[] data = enc.finish();

        Decoder dec = new Decoder(data);
        for (int i = 0; i < values.length; i++) {
            test("int64 value " + values[i], dec.readInt64() == values[i]);
        }
    }

    private static void testFloat32() {
        System.out.println("\n=== Test Float32 ===");
        float[] values = {0.0f, 1.0f, -1.0f, 3.14159f, -273.15f};
        Encoder enc = new Encoder(256);
        for (float v : values) {
            enc.writeFloat32(v);
        }
        byte[] data = enc.finish();

        Decoder dec = new Decoder(data);
        for (int i = 0; i < values.length; i++) {
            float decoded = dec.readFloat32();
            test("float32 value " + values[i], Math.abs(decoded - values[i]) < 0.0001f);
        }
    }

    private static void testFloat64() {
        System.out.println("\n=== Test Float64 ===");
        double[] values = {0.0, 1.0, -1.0, 3.14159265358979, -273.15, 1e100};
        Encoder enc = new Encoder(256);
        for (double v : values) {
            enc.writeFloat64(v);
        }
        byte[] data = enc.finish();

        Decoder dec = new Decoder(data);
        for (int i = 0; i < values.length; i++) {
            double decoded = dec.readFloat64();
            test("float64 value " + values[i], Math.abs(decoded - values[i]) < 1e-10);
        }
    }

    private static void testString() {
        System.out.println("\n=== Test String ===");
        String[] values = {"", "a", "hello", "hello world", "1234567890", "This is a longer string with many characters"};
        for (String v : values) {
            Encoder enc = new Encoder(256);
            enc.writeString(v);
            byte[] data = enc.finish();

            Decoder dec = new Decoder(data);
            String decoded = dec.readString();
            test("string '" + v + "'", decoded.equals(v));
        }

        // Test long string (>254 chars)
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < 300; i++) {
            sb.append('x');
        }
        String longStr = sb.toString();

        Encoder enc = new Encoder(512);
        enc.writeString(longStr);
        byte[] data = enc.finish();

        Decoder dec = new Decoder(data);
        String decoded = dec.readString();
        test("long string (>254 chars) length", decoded.length() == 300);
        test("long string content", decoded.equals(longStr));
    }

    private static void testBytes() {
        System.out.println("\n=== Test Bytes ===");
        byte[] data1 = new byte[] {0x01, 0x02, 0x03, 0x04, 0x05};
        byte[] data2 = new byte[256];
        for (int i = 0; i < 256; i++) {
            data2[i] = (byte)i;
        }

        Encoder enc = new Encoder(512);
        enc.writeBytes(data1);
        enc.writeBytes(data2);
        byte[] data = enc.finish();

        Decoder dec = new Decoder(data);
        byte[] decoded1 = dec.readBytes();
        byte[] decoded2 = dec.readBytes();

        test("small bytes length", decoded1.length == data1.length);
        test("small bytes content", java.util.Arrays.equals(decoded1, data1));
        test("large bytes length", decoded2.length == data2.length);
        test("large bytes content", java.util.Arrays.equals(decoded2, data2));
    }

    private static void testNestedMessage() {
        System.out.println("\n=== Test Nested Message ===");

        // Encode inner message
        Encoder innerEnc = new Encoder(64);
        innerEnc.writeString("inner_value");
        innerEnc.writeInt32(42);
        byte[] innerData = innerEnc.finish();

        // Encode outer message
        Encoder outerEnc = new Encoder(256);
        outerEnc.writeString("outer_name");
        outerEnc.writeMessage(innerData);
        byte[] outerData = outerEnc.finish();

        // Decode
        Decoder dec = new Decoder(outerData);
        String name = dec.readString();
        byte[] innerOut = dec.readMessageBytes();

        test("outer string", name.equals("outer_name"));
        test("inner message length", innerOut.length == innerData.length);

        Decoder innerDec = new Decoder(innerOut);
        String innerStr = innerDec.readString();
        int innerInt = innerDec.readInt32();

        test("inner string", innerStr.equals("inner_value"));
        test("inner int", innerInt == 42);
    }

    private static void testAllTypes() {
        System.out.println("\n=== Test All Types Combined ===");
        Encoder enc = new Encoder(256);
        enc.writeBool(true);
        enc.writeInt32(-12345);
        enc.writeInt64(9876543210L);
        enc.writeFloat32(3.14f);
        enc.writeFloat64(2.718281828);
        enc.writeString("test string");
        enc.writeBytes(new byte[] {(byte)0xDE, (byte)0xAD, (byte)0xBE, (byte)0xEF});
        byte[] data = enc.finish();

        Decoder dec = new Decoder(data);
        test("bool", dec.readBool() == true);
        test("int32", dec.readInt32() == -12345);
        test("int64", dec.readInt64() == 9876543210L);
        test("float32", Math.abs(dec.readFloat32() - 3.14f) < 0.001f);
        test("float64", Math.abs(dec.readFloat64() - 2.718281828) < 1e-9);
        test("string", dec.readString().equals("test string"));
        byte[] bytes = dec.readBytes();
        test("bytes length", bytes.length == 4);
        test("bytes content", java.util.Arrays.equals(bytes, new byte[] {(byte)0xDE, (byte)0xAD, (byte)0xBE, (byte)0xEF}));
    }

    public static void main(String[] args) {
        System.out.println("===========================================");
        System.out.println("XPB V2 Java Runtime Tests");
        System.out.println("===========================================");

        testBool();
        testInt32();
        testInt64();
        testFloat32();
        testFloat64();
        testString();
        testBytes();
        testNestedMessage();
        testAllTypes();

        System.out.println("\n===========================================");
        System.out.println("Results: " + testsPassed + " passed, " + testsFailed + " failed");
        System.out.println("===========================================");

        if (testsFailed > 0) {
            System.exit(1);
        }
    }
}
