import { LazyArrayDecoder, FieldType } from '../../../runtime/ts/src/lazy.js';
import { Encoder, Decoder } from '../../../runtime/ts/src/index.js';

// Re-implement simplified compileEncoder/Slab since they aren't exported from index.ts
class SlabAllocator {
    constructor(public buf: Uint8Array, public pos = 0, public view = new DataView(buf.buffer)) {}
}

const textEncoder = new TextEncoder();

function compileEncoder(schema: any) {
    return (slab: SlabAllocator, obj: any) => {
        const enc = new Encoder(1024);
        // Quick hack: use the standard Encoder methods
        // In a real bench we'd use the JIT one, but for data prep this is fine.
        for (const field of schema.fields) {
            const val = obj[field.name];
            switch (field.type) {
                case FieldType.Bool: enc.writeBool(val); break;
                case FieldType.Int32: enc.writeInt32(val); break;
                case FieldType.Float64: enc.writeFloat64(val); break;
                case FieldType.String: enc.writeString(val); break;
            }
        }
        const data = enc.finish();
        // Copy to slab
        if (slab.pos + data.length > slab.buf.length) {
             const newBuf = new Uint8Array(Math.max(slab.buf.length * 2, slab.pos + data.length));
             newBuf.set(slab.buf);
             slab.buf = newBuf;
        }
        slab.buf.set(data, slab.pos);
        slab.pos += data.length;
    };
}

async function main() {
    console.log("╔══════════════════════════════════════════════════════════════╗");
    console.log("║              XPB Lazy Decoder Benchmark                      ║");
    console.log("╚══════════════════════════════════════════════════════════════╝\n");

    const schema = {
        fields: [
            { type: FieldType.Int32, name: "id" },
            { type: FieldType.String, name: "name" },
            { type: FieldType.String, name: "email" },
            { type: FieldType.Int32, name: "age" },
            { type: FieldType.Float64, name: "score" },
            { type: FieldType.Bool, name: "active" },
            { type: FieldType.String, name: "description" },
        ]
    };

    // Generate Data (10k items)
    const count = 10000;
    const items = [];
    for (let i = 0; i < count; i++) {
        items.push({
            id: i,
            name: `User ${i}`,
            email: `user${i}@example.com`,
            age: 20 + (i % 50),
            score: i * 1.5,
            active: i % 2 === 0,
            description: "A reasonably long description string to simulate real world data."
        });
    }

    // Encode XPB
    console.log(`Encoding ${count} items...`);
    // Manual array encoding
    const enc = new Encoder(1024 * 1024);
    enc.writeInt32(count);
    for (const item of items) {
        enc.writeInt32(item.id);
        enc.writeString(item.name);
        enc.writeString(item.email);
        enc.writeInt32(item.age);
        enc.writeFloat64(item.score);
        enc.writeBool(item.active);
        enc.writeString(item.description);
    }
    const xpbData = enc.finish();
    console.log(`XPB Size: ${(xpbData.length / 1024).toFixed(2)} KB`);

    // Encode JSON
    const jsonStr = JSON.stringify(items);
    const jsonData = textEncoder.encode(jsonStr); // Bytes
    console.log(`JSON Size: ${(jsonData.length / 1024).toFixed(2)} KB`);
    console.log("");

    const iterations = 50;

    // Benchmark 1: Initial Load (Parsing)
    // For Lazy: Scan time
    // For JSON: JSON.parse time
    
    let start = performance.now();
    for (let i = 0; i < iterations; i++) {
        JSON.parse(jsonStr);
    }
    const jsonParseTime = (performance.now() - start) / iterations;

    start = performance.now();
    let lazyArr;
    for (let i = 0; i < iterations; i++) {
        lazyArr = new LazyArrayDecoder(xpbData, schema);
    }
    const lazyScanTime = (performance.now() - start) / iterations;

    console.log("1. Initial Load (Parse/Scan) Time:");
    console.log(`   JSON.parse:   ${jsonParseTime.toFixed(3)} ms`);
    console.log(`   XPB Lazy Scan: ${lazyScanTime.toFixed(3)} ms`);
    console.log(`   Speedup:      ${(jsonParseTime / lazyScanTime).toFixed(2)}x FASTER`);
    console.log("");

    // Benchmark 2: Access 10 Random Items (Virtual Scroll scenario)
    
    // JSON needs to parse FIRST, then access.
    // We assume JSON is already parsed for this test to be fair to "access speed", 
    // but in reality you pay the parse cost.
    
    const parsedJSON = JSON.parse(jsonStr);
    lazyArr = new LazyArrayDecoder(xpbData, schema); // Reset

    start = performance.now();
    for (let i = 0; i < iterations * 100; i++) {
        const idx = Math.floor(Math.random() * count);
        const item = parsedJSON[idx];
        const val = item.name;
    }
    const jsonAccessTime = (performance.now() - start) / (iterations * 100);

    start = performance.now();
    for (let i = 0; i < iterations * 100; i++) {
        const idx = Math.floor(Math.random() * count);
        const item = lazyArr!.get(idx);
        const val = item.name; // Access field to trigger lazy decode
    }
    const lazyAccessTime = (performance.now() - start) / (iterations * 100);

    console.log("2. Random Access (1 Item):");
    console.log(`   JSON Access:   ${(jsonAccessTime * 1000).toFixed(0)} ns (Cached Object)`);
    console.log(`   XPB Lazy:      ${(lazyAccessTime * 1000).toFixed(0)} ns (On-demand Decode)`);
    console.log("");

    // Benchmark 3: Total "Time to First Paint" (Parse + Access 20 items)
    // This simulates a UI loading a list and rendering the first viewport.
    
    start = performance.now();
    for (let i = 0; i < iterations; i++) {
        const arr = JSON.parse(jsonStr);
        for(let j=0; j<20; j++) {
            const v = arr[j].name;
        }
    }
    const jsonTFP = (performance.now() - start) / iterations;

    start = performance.now();
    for (let i = 0; i < iterations; i++) {
        const arr = new LazyArrayDecoder(xpbData, schema);
        for(let j=0; j<20; j++) {
            const v = arr.get(j).name;
        }
    }
    const lazyTFP = (performance.now() - start) / iterations;

    console.log("3. Time to First Data (Parse + Read 20 items):");
    console.log(`   JSON:     ${jsonTFP.toFixed(3)} ms`);
    console.log(`   XPB Lazy: ${lazyTFP.toFixed(3)} ms`);
    console.log(`   Winner:   ${lazyTFP < jsonTFP ? "XPB Lazy" : "JSON"} (${(jsonTFP/lazyTFP).toFixed(2)}x speedup)`);
}

main().catch(console.error);
