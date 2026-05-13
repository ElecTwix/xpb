package xpb;

import java.io.ByteArrayOutputStream;

/**
 * XPB V2 Java Runtime Security Validation (post-hardening).
 *
 * Each test exercises a class of input that pre-hardening either succeeded
 * silently or hit an uncatchable JVM allocation. After the runtime gained
 * readArrayCount(elementMinBytes, maxElements), every one of these inputs
 * now raises a clear RuntimeException. The tests assert that the
 * exception fires — a future change that removes the guard takes a test
 * from "rejected" back to "silently allocated GB of int[]" and fails.
 */
public class XpbSecurityTest {
    private static int passed = 0;
    private static int failed = 0;

    private static void expectThrow(String name, Runnable fn) {
        try {
            fn.run();
            System.out.println("  [FAIL] " + name + " — call succeeded; expected an exception");
            failed++;
        } catch (RuntimeException e) {
            System.out.println("  [PASS] " + name + " — threw: " + e.getMessage());
            passed++;
        }
    }

    private static byte[] int32LE(int v) {
        byte[] out = new byte[4];
        out[0] = (byte) (v & 0xFF);
        out[1] = (byte) ((v >> 8) & 0xFF);
        out[2] = (byte) ((v >> 16) & 0xFF);
        out[3] = (byte) ((v >> 24) & 0xFF);
        return out;
    }

    public static void main(String[] args) {
        System.out.println("===========================================");
        System.out.println("XPB V2 Java Runtime Security Validation");
        System.out.println("===========================================");

        // XPB-107: readArrayInt32 requires explicit maxElements. The old
        // signature took no max — the new one takes one and rejects an
        // oversized wire-supplied count up front.
        System.out.println("\n=== XPB-107: oversized count rejected against caller's max ===");
        expectThrow("readArrayInt32 with count > max", () -> {
            byte[] payload = int32LE(1000); // claims 1000 elements
            Decoder dec = new Decoder(payload);
            dec.readArrayInt32(64); // caller's budget is 64
        });

        // XPB-107: negative count rejected.
        System.out.println("\n=== XPB-107: negative count rejected ===");
        expectThrow("readArrayInt32 with negative count", () -> {
            byte[] payload = int32LE(-1);
            Decoder dec = new Decoder(payload);
            dec.readArrayInt32(1024);
        });

        // XPB-107: even within the max, a count that can't fit in the
        // remaining buffer is rejected (buffer-bound check still fires).
        System.out.println("\n=== XPB-107: count exceeding buffer is rejected ===");
        expectThrow("readArrayInt32 with count > remaining buffer", () -> {
            byte[] payload = int32LE(1000);
            Decoder dec = new Decoder(payload);
            dec.readArrayInt32(1 << 20); // huge max, but buffer is 4 bytes
        });

        // XPB-107: max < 0 is itself an error (caller bug).
        System.out.println("\n=== XPB-107: negative max rejected with IllegalArgumentException ===");
        try {
            byte[] payload = int32LE(0);
            Decoder dec = new Decoder(payload);
            dec.readArrayInt32(-5);
            System.out.println("  [FAIL] negative max accepted");
            failed++;
        } catch (IllegalArgumentException e) {
            System.out.println("  [PASS] negative max rejected: " + e.getMessage());
            passed++;
        }

        // Sanity: legitimate decode round-trips under an honest max.
        System.out.println("\n=== Regression: legitimate array round-trip works ===");
        try {
            Encoder enc = new Encoder(64);
            int[] in = {10, 20, 30, 40};
            enc.writeInt32(in.length);
            for (int v : in) enc.writeInt32(v);
            byte[] data = enc.finish();

            Decoder dec = new Decoder(data);
            int[] out = dec.readArrayInt32(16); // caller's max
            if (out.length == 4 && out[0] == 10 && out[3] == 40) {
                System.out.println("  [PASS] round-trip preserved");
                passed++;
            } else {
                System.out.println("  [FAIL] round-trip corrupted");
                failed++;
            }
        } catch (Exception e) {
            System.out.println("  [FAIL] round-trip threw: " + e.getMessage());
            failed++;
        }

        System.out.println("\nResults: " + passed + " passed, " + failed + " failed");
        if (failed > 0) System.exit(1);
    }
}
