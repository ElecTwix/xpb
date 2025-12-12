
import { Encoder, Decoder } from '../../../runtime/ts/src/index.js';

const textEncoder = new TextEncoder();

async function main() {
    console.log("╔══════════════════════════════════════════════════════════════╗");
    console.log("║           XPB Columnar (SoA) vs Row (AoS) Benchmark          ║");
    console.log("╚══════════════════════════════════════════════════════════════╝\n");

    // 1. Generate Data (100k items to make it heavy)
    const count = 100000;
    console.log(`Generating ${count.toLocaleString()} items...`);
    
    const ids = new Int32Array(count);
    const scores = new Float64Array(count);
    const names: string[] = [];
    const active: boolean[] = [];

    for (let i = 0; i < count; i++) {
        ids[i] = i;
        scores[i] = Math.random() * 100;
        names[i] = `User ${i}`;
        active[i] = i % 2 === 0;
    }

    // 2. Prepare JSON (Row vs Column)
    
    // JSON Row: [{id, score...}, ...] 
    const rowObjects = [];
    for(let i=0; i<count; i++) {
        rowObjects.push({ id: ids[i], name: names[i], score: scores[i], active: active[i] });
    }
    const jsonRowStr = JSON.stringify(rowObjects);
    const jsonRowSize = textEncoder.encode(jsonRowStr).length;

    // JSON Column: { ids: [], names: [], ... }
    const colObject = { ids: Array.from(ids), names, scores: Array.from(scores), active };
    const jsonColStr = JSON.stringify(colObject);
    const jsonColSize = textEncoder.encode(jsonColStr).length;

    console.log(`\nSizes:`);
    console.log(`JSON Row:    ${(jsonRowSize / 1024 / 1024).toFixed(2)} MB`);
    console.log(`JSON Column: ${(jsonColSize / 1024 / 1024).toFixed(2)} MB  (Savings: ${(100 - jsonColSize/jsonRowSize*100).toFixed(1)}%)`);

    // 3. Prepare XPB (Row vs Column)

    // XPB Row
    // Format: [count] [ {id, name, score, active} ... ]
    const encRow = new Encoder(jsonRowSize); // Pre-allocate approx
    encRow.writeInt32(count);
    for(let i=0; i<count; i++) {
        encRow.writeInt32(ids[i]);
        encRow.writeString(names[i]);
        encRow.writeFloat64(scores[i]);
        encRow.writeBool(active[i]);
    }
    const xpbRowData = encRow.finish();
    const xpbRowSize = xpbRowData.length;

    // XPB Column
    // Format: [ids_count][ids...] [names_count][names...] [scores_count][scores...] [active_count][active...]
    const encCol = new Encoder(jsonColSize);
    
    // IDs (Int32 Array)
    encCol.writeInt32(count);
    for(let i=0; i<count; i++) encCol.writeInt32(ids[i]);
    
    // Names (String Array)
    encCol.writeInt32(count);
    for(let i=0; i<count; i++) encCol.writeString(names[i]);

    // Scores (Float64 Array)
    encCol.writeInt32(count);
    for(let i=0; i<count; i++) encCol.writeFloat64(scores[i]);

    // Active (Bool Array)
    encCol.writeInt32(count);
    for(let i=0; i<count; i++) encCol.writeBool(active[i]);

    const xpbColData = encCol.finish();
    const xpbColSize = xpbColData.length;

    console.log(`XPB Row:     ${(xpbRowSize / 1024 / 1024).toFixed(2)} MB`);
    console.log(`XPB Column:  ${(xpbColSize / 1024 / 1024).toFixed(2)} MB  (Savings: ${(100 - xpbColSize/xpbRowSize*100).toFixed(1)}%)`);
    console.log(`vs JSON Row: ${(100 - xpbColSize/jsonRowSize*100).toFixed(1)}% smaller`);

    // 4. Benchmarks

    const iterations = 20;

    // --- Decode Time ---
    console.log(`\n1. Decode Time (Full Load):`);

    // JSON Row
    let start = performance.now();
    for(let i=0; i<iterations; i++) JSON.parse(jsonRowStr);
    const tJsonRow = (performance.now() - start) / iterations;

    // JSON Column
    start = performance.now();
    for(let i=0; i<iterations; i++) JSON.parse(jsonColStr);
    const tJsonCol = (performance.now() - start) / iterations;

    // XPB Row (Full Decode)
    start = performance.now();
    for(let i=0; i<iterations; i++) {
        const dec = new Decoder(xpbRowData);
        const cnt = dec.readInt32();
        for(let j=0; j<cnt; j++) {
            dec.readInt32();
            dec.readString();
            dec.readFloat64();
            dec.readBool();
        }
    }
    const tXpbRow = (performance.now() - start) / iterations;

    // XPB Column (Smart Decode)
    // For Int32/Float64, we can use TypedArrays!
    start = performance.now();
    for(let i=0; i<iterations; i++) {
        const dec = new Decoder(xpbColData);
        
        // IDs
        const cntIds = dec.readInt32();
        // Zero-copy attempt (requires alignment) or Fast Copy
        // Here we simulate Fast Copy reading
        const idArr = new Int32Array(cntIds);
        for(let j=0; j<cntIds; j++) idArr[j] = dec.readInt32();

        // Names
        const cntNames = dec.readInt32();
        const nameArr = new Array(cntNames);
        for(let j=0; j<cntNames; j++) nameArr[j] = dec.readString();

        // Scores
        const cntScores = dec.readInt32();
        const scoreArr = new Float64Array(cntScores);
        for(let j=0; j<cntScores; j++) scoreArr[j] = dec.readFloat64();

        // Active
        const cntActive = dec.readInt32();
        const activeArr = new Uint8Array(cntActive); // Bool as bytes
        for(let j=0; j<cntActive; j++) activeArr[j] = dec.readBool() ? 1 : 0;
    }
    const tXpbCol = (performance.now() - start) / iterations;

    console.log(`   JSON Row:    ${tJsonRow.toFixed(2)} ms`);
    console.log(`   JSON Column: ${tJsonCol.toFixed(2)} ms`);
    console.log(`   XPB Row:     ${tXpbRow.toFixed(2)} ms`);
    console.log(`   XPB Column:  ${tXpbCol.toFixed(2)} ms`);
    console.log(`   -> Columnar Speedup: ${(tXpbRow/tXpbCol).toFixed(2)}x vs Row`);
    console.log(`   -> vs JSON Row:      ${(tJsonRow/tXpbCol).toFixed(2)}x FASTER`);

    // --- Analytics Query ---
    console.log(`\n2. Analytics Query (Sum of Scores for active users):`);
    // Scenario: Calculate average score of active users.
    // Row: Must iterate objects, check .active, add .score
    // Column: Iterate active array, add score array (Cache friendly!)

    // Setup parsed data
    const dJsonRow = JSON.parse(jsonRowStr);
    const dJsonCol = JSON.parse(jsonColStr); // {ids:[], ...}
    
    // Parse XPB Column into TypedArrays for fairness (simulating "Loaded" state)
    const dec = new Decoder(xpbColData);
    const cCount = dec.readInt32();
    const cIds = new Int32Array(cCount);
    for(let j=0; j<cCount; j++) cIds[j] = dec.readInt32();
    
    dec.readInt32(); // skip name count
    for(let j=0; j<cCount; j++) dec.readString(); // skip names (lazy skip possible?)

    const sCount = dec.readInt32();
    const cScores = new Float64Array(sCount);
    for(let j=0; j<sCount; j++) cScores[j] = dec.readFloat64();

    const aCount = dec.readInt32();
    const cActive = new Uint8Array(aCount);
    for(let j=0; j<aCount; j++) cActive[j] = dec.readBool() ? 1 : 0;


    const queryIter = 1000;
    
    // JSON Row Query
    start = performance.now();
    let sum1 = 0;
    for(let k=0; k<queryIter; k++) {
        sum1 = 0;
        for(let i=0; i<count; i++) {
            if (dJsonRow[i].active) sum1 += dJsonRow[i].score;
        }
    }
    const tQueryJsonRow = (performance.now() - start) / queryIter;

    // XPB Column Query (Typed Arrays)
    start = performance.now();
    let sum2 = 0;
    for(let k=0; k<queryIter; k++) {
        sum2 = 0;
        for(let i=0; i<count; i++) {
            if (cActive[i]) sum2 += cScores[i];
        }
    }
    const tQueryXpbCol = (performance.now() - start) / queryIter;

    console.log(`   JSON Row (Object Scan):    ${tQueryJsonRow.toFixed(3)} ms`);
    console.log(`   XPB Column (TypedArray):   ${tQueryXpbCol.toFixed(3)} ms`);
    console.log(`   -> Speedup: ${(tQueryJsonRow/tQueryXpbCol).toFixed(2)}x FASTER`);
}

main().catch(console.error);
