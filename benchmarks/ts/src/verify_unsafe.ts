
import { Encoder, Decoder } from '../../../runtime/ts/src/index.js';
import { UnsafeEncoder, UnsafeDecoder } from '../../../runtime/ts/src/unsafe.js';
import { SlabAllocator, compileEncoder, compileDecoder, FieldType } from '../../../runtime/ts/src/jit.js';
import assert from 'assert';

const testData = {
  name: "Alice Johnson",
  age: 30,
  active: true,
  bio: "This is a test bio with some utf8: 🚀✨",
  scores: [1, 2, 3, 4, 5],
  largeNum: BigInt("9007199254740991") // MAX_SAFE_INTEGER
};

function verify() {
  console.log("Verifying Unsafe Runtime...");

  // 1. Verify Encode Parity
  const stdEnc = new Encoder();
  stdEnc.writeString(1, testData.name);
  stdEnc.writeInt32(2, testData.age);
  stdEnc.writeBool(3, testData.active);
  stdEnc.writeString(4, testData.bio);
  for (const s of testData.scores) stdEnc.writeInt32(5, s);
  stdEnc.writeInt64(6, testData.largeNum);
  const stdBytes = stdEnc.finish();

  const unsafeEnc = new UnsafeEncoder();
  unsafeEnc.writeString(1, testData.name);
  unsafeEnc.writeInt32(2, testData.age);
  unsafeEnc.writeBool(3, testData.active);
  unsafeEnc.writeString(4, testData.bio);
  for (const s of testData.scores) unsafeEnc.writeInt32(5, s);
  unsafeEnc.writeInt64(6, testData.largeNum);
  const unsafeBytes = unsafeEnc.finish();

  try {
    assert.deepStrictEqual(unsafeBytes, stdBytes, "Encoding mismatch!");
    console.log("✅ Encode Output matches Standard Runtime");
  } catch (e) {
    console.error("Encode Mismatch:");
    console.error("Standard:", stdBytes);
    console.error("Unsafe:  ", unsafeBytes);
    process.exit(1);
  }

  // 2. Verify Decode Correctness (Unsafe Decoder reading Standard Bytes)
  const dec = new UnsafeDecoder(stdBytes);
  const res: any = { scores: [] };
  
  while (!dec.eof()) {
    const [fn, wt] = dec.readTag();
    switch (fn) {
      case 1: res.name = dec.readString(); break;
      case 2: res.age = dec.readInt32(); break;
      case 3: res.active = dec.readBool(); break;
      case 4: res.bio = dec.readString(); break;
      case 5: res.scores.push(dec.readInt32()); break;
      case 6: res.largeNum = dec.readInt64(); break;
      default: dec.skip(wt);
    }
  }

  assert.strictEqual(res.name, testData.name);
  assert.strictEqual(res.age, testData.age);
  assert.strictEqual(res.active, testData.active);
  assert.strictEqual(res.bio, testData.bio);
  assert.deepStrictEqual(res.scores, testData.scores);
  assert.strictEqual(res.largeNum, testData.largeNum);
  
  console.log("✅ Decode Correctness verified");

  // 3. Verify Float32/64
  const fEnc = new UnsafeEncoder();
  fEnc.writeFloat32(1, 1.234);
  fEnc.writeFloat64(2, 123.456);
  const fBytes = fEnc.finish();

  const fDec = new UnsafeDecoder(fBytes);
  // manual read
  fDec.readTag(); // 1
  const f32 = fDec.readFloat32();
  fDec.readTag(); // 2
  const f64 = fDec.readFloat64();

  assert(Math.abs(f32 - 1.234) < 0.0001, "Float32 mismatch");
  assert(Math.abs(f64 - 123.456) < 0.0001, "Float64 mismatch");
  
  console.log("✅ Float support verified");

  // 4. Verify JIT output (Slab) matches Standard
  console.log("Verifying JIT Runtime...");
  
  const schema = {
     fields: [
       { tag: 1, type: FieldType.String, name: 'name' },
       { tag: 2, type: FieldType.Int32, name: 'age' },
       { tag: 3, type: FieldType.Bool, name: 'active' },
       { tag: 4, type: FieldType.String, name: 'bio' },
       { tag: 5, type: FieldType.Int32, name: 'scores', repeated: true },
       { tag: 6, type: FieldType.Int64, name: 'largeNum' }
     ]
  };

  const jitEncode = compileEncoder<typeof testData>(schema);
  const jitDecode = compileDecoder<typeof testData>(schema);
  const slab = new SlabAllocator();
  
  jitEncode(slab, testData);
  const jitBytes = slab.buf.subarray(0, slab.pos);

  try {
     assert.deepStrictEqual(jitBytes, stdBytes, "JIT Encoding mismatch!");
     console.log("✅ JIT Encode matches Standard Runtime");
  } catch(e) {
     console.error("JIT Mismatch:");
     console.error("Standard:", stdBytes);
     console.error("JIT:     ", jitBytes);
     process.exit(1);
  }

  // 5. Verify JIT Decode
  const resJit = jitDecode(jitBytes, jitBytes.length);
  assert.strictEqual(resJit.name, testData.name);
  assert.strictEqual(resJit.age, testData.age);
  // Note: JIT decoder I implemented returns zig-zagged ints, let's check
  // Actually my JIT decoder implementation does the zigzag decode.
  assert.deepStrictEqual(resJit.scores, testData.scores); 
  
  console.log("✅ JIT Decode verification passed");
}

verify();
