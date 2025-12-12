/**
 * XPB V2 Node.js Worker Script
 * 
 * Handles high-performance parallel decoding using Transferable Objects (worker_threads).
 */

import { parentPort } from 'worker_threads';
import { Buffer } from 'node:buffer';

class FastDecoder {
  private data: Buffer;
  private pos = 0;

  constructor(data: Buffer) {
    this.data = data;
  }

  readInt32(): number {
    const v = this.data.readInt32LE(this.pos);
    this.pos += 4;
    return v;
  }
  
  readStringLength(): number {
    const len = this.data[this.pos++];
    if (len === 255) {
         const v = this.data.readUInt32LE(this.pos);
         this.pos += 4;
         return v;
    }
    return len;
  }
  
  readRawStringBytes(len: number): Buffer {
      const bytes = this.data.subarray(this.pos, this.pos + len);
      this.pos += len;
      return bytes;
  }
}

function decodeInt32Array(buffer: Buffer) {
  const decoder = new FastDecoder(buffer);
  const count = decoder.readInt32();
  
  const result = new Int32Array(count);
  for (let i = 0; i < count; i++) {
    result[i] = decoder.readInt32();
  }
  
  return { result, transfer: [result.buffer] };
}

function decodeStringArray(buffer: Buffer) {
  const decoder = new FastDecoder(buffer);
  const count = decoder.readInt32();
  
  const offsets = new Int32Array(count + 1);
  const data = new Uint8Array(buffer.length); 
  let dataPos = 0;
  
  for (let i = 0; i < count; i++) {
      offsets[i] = dataPos;
      const len = decoder.readStringLength();
      const bytes = decoder.readRawStringBytes(len);
      data.set(bytes, dataPos);
      dataPos += len;
  }
  offsets[count] = dataPos;
  
  const finalData = data.slice(0, dataPos);
  
  return { 
      result: { offsets, data: finalData }, 
      transfer: [offsets.buffer, finalData.buffer] 
  };
}

if (parentPort) {
  parentPort.on('message', (msg) => {
    const { id, type, buffer } = msg;
    const buf = Buffer.from(buffer); 
    
    try {
      let response;
      if (type === 'int32Array') {
        response = decodeInt32Array(buf);
      } else if (type === 'stringArray') {
        response = decodeStringArray(buf);
      }
      
      if (response) {
        parentPort!.postMessage({ id, result: response.result }, response.transfer as any);
      }
    } catch (e) {
      parentPort!.postMessage({ id, error: (e as Error).message });
    }
  });
}
