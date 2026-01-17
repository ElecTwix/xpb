package xpb;

public class XpbBench {
    private static final int ITERATIONS = 100000;
    private static final int WARMUP = 1000;

    private static int currentAge = 30;

    private static double runBenchmark(Runnable fn) {
        for (int i = 0; i < WARMUP; i++) fn.run();

        long start = System.nanoTime();
        for (int i = 0; i < ITERATIONS; i++) fn.run();
        long end = System.nanoTime();

        return (end - start) / (double) ITERATIONS;
    }

    private static byte[] xpbEncode() {
        Encoder enc = new Encoder(64);
        enc.writeString("Alice Johnson");
        enc.writeInt32(30);
        enc.writeBool(true);
        return enc.finish();
    }

    private static User xpbDecode(byte[] data) {
        Decoder dec = new Decoder(data);
        String name = dec.readString();
        int age = dec.readInt32();
        boolean active = dec.readBool();
        return new User(name, age, active);
    }

    private static String jsonEncode() {
        User user = new User("Alice Johnson", 30, true);
        return String.format("{\"name\":\"%s\",\"age\":%d,\"active\":%b}", user.name, user.age, user.active);
    }

    private static User jsonDecode(String json) {
        String[] parts = json.replaceAll("[{}\"]", "").split(",");
        String name = parts[0].split(":")[1];
        int age = Integer.parseInt(parts[1].split(":")[1]);
        boolean active = Boolean.parseBoolean(parts[2].split(":")[1]);
        return new User(name, age, active);
    }

    private static class User {
        String name;
        int age;
        boolean active;
        User(String name, int age, boolean active) {
            this.name = name;
            this.age = age;
            this.active = active;
        }
    }

    public static void main(String[] args) throws Exception {
        System.out.println("===========================================");
        System.out.println("XPB V2 Java Benchmark (Simple Message)");
        System.out.println("===========================================");
        System.out.println("Iterations: " + ITERATIONS + "\n");

        System.out.println("Note: JSON operations use simple string parsing.");
        System.out.println("      For real comparison, add Jackson dependency.\n");

        double xpbEnc = runBenchmark(() -> {
            byte[] data = xpbEncode();
        });

        byte[] xpbData = xpbEncode();
        double xpbDec = runBenchmark(() -> {
            User user = xpbDecode(xpbData);
            currentAge = user.age;
        });

        double jsonEnc = runBenchmark(() -> {
            String json = jsonEncode();
        });

        String jsonData = jsonEncode();
        double jsonDec = runBenchmark(() -> {
            User user = jsonDecode(jsonData);
            currentAge = user.age;
        });

        System.out.println("Benchmark results (ns per operation):");
        System.out.printf("  XPB   encode: %.0f ns/op\n", xpbEnc);
        System.out.printf("  XPB   decode: %.0f ns/op\n", xpbDec);
        System.out.printf("  JSON  encode: %.0f ns/op\n", jsonEnc);
        System.out.printf("  JSON  decode: %.0f ns/op\n\n", jsonDec);

        System.out.println("Speedup vs JSON:");
        System.out.printf("  XPB encode: %.2fx faster\n", jsonEnc / xpbEnc);
        System.out.printf("  XPB decode: %.2fx faster\n", jsonDec / xpbDec);

        System.out.println("\n===========================================");
        System.out.println("Test passed: benchmark executed successfully");
        System.out.println("===========================================");
    }
}
