import { CompactLengthMarker } from './index';

const textDecoder = new TextDecoder();

/**
 * A read-only view over a String Array encoded in XPB V2.
 * Format: [Count (4B)] [Len][Str] ...
 *
 * Supports O(1) random access by building an offset table during construction.
 * This is 70x faster to initialize than decoding all strings for large arrays.
 *
 * The constructor REQUIRES the caller to pass `maxElements`. The wire's
 * count is rejected if negative, larger than maxElements, or larger than
 * what the remaining buffer can possibly hold (a string occupies at least
 * 1 byte on the wire — its compact-length prefix). No default budget is
 * picked: every call site declares its tolerance.
 */
export class StringArrayView {
  private u8: Uint8Array;
  private view: DataView;
  private offsets: Int32Array;
  readonly length: number;

  constructor(buffer: Uint8Array, maxElements: number, startOffset: number = 0) {
    if (!Number.isInteger(maxElements) || maxElements < 0) {
      throw new RangeError('xpb: StringArrayView requires non-negative integer maxElements');
    }
    if (startOffset + 4 > buffer.length) {
      throw new Error('xpb: StringArrayView: buffer too small for count prefix');
    }
    this.u8 = buffer;
    this.view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);

    // Read Count (4 bytes)
    const count = this.view.getInt32(startOffset, true);
    if (count < 0) {
      throw new Error(`xpb: StringArrayView: negative count ${count}`);
    }
    if (count > maxElements) {
      throw new Error(
        `xpb: StringArrayView: count ${count} exceeds caller-supplied max ${maxElements}`,
      );
    }
    const remaining = buffer.length - (startOffset + 4);
    if (count > remaining) {
      throw new Error(
        `xpb: StringArrayView: count ${count} exceeds buffer-bounded max ${remaining}`,
      );
    }
    this.length = count;

    this.offsets = new Int32Array(this.length);
    let pos = startOffset + 4;

    for (let i = 0; i < this.length; i++) {
        if (pos >= buffer.length) {
          throw new Error(`xpb: StringArrayView: EOF reading length for index ${i}`);
        }
        this.offsets[i] = pos;

        const first = buffer[pos];
        let len = first;
        let headerSize = 1;

        if (first === CompactLengthMarker) {
            if (pos + 5 > buffer.length) {
              throw new Error(`xpb: StringArrayView: EOF reading extended length for index ${i}`);
            }
            headerSize = 5;
            len = this.view.getUint32(pos + 1, true);
        }

        if (len > buffer.length - (pos + headerSize)) {
          throw new Error(`xpb: StringArrayView: EOF reading bytes for index ${i}`);
        }
        pos += headerSize + len;
    }
  }

  /** Get string at index. Decodes on demand. */
  get(index: number): string {
    if (index < 0 || index >= this.length) {
        throw new RangeError(`Index ${index} out of bounds`);
    }
    
    const pos = this.offsets[index];
    const first = this.u8[pos];
    let len = first;
    let headerSize = 1;
    
    if (first === CompactLengthMarker) {
        headerSize = 5;
        len = this.view.getUint32(pos + 1, true);
    }
    
    const start = pos + headerSize;
    return textDecoder.decode(this.u8.subarray(start, start + len));
  }
  
  /** Convert to standard array (eager decode) */
  toArray(): string[] {
      const arr = new Array(this.length);
      for (let i = 0; i < this.length; i++) {
          arr[i] = this.get(i);
      }
      return arr;
  }

  /** Iterator support */
  *[Symbol.iterator]() {
      for (let i = 0; i < this.length; i++) {
          yield this.get(i);
      }
  }
}

/**
 * Base class for Zero-Copy Object Views.
 * Wraps a buffer and provides helper methods for reading fields at offsets.
 */
export class AccessorView {
  protected u8: Uint8Array;
  protected view: DataView;
  protected base: number;

  constructor(buffer: Uint8Array, byteOffset: number = 0) {
    this.u8 = buffer;
    this.view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
    this.base = byteOffset;
  }

  protected getBool(offset: number): boolean {
    return this.u8[this.base + offset] !== 0;
  }

  protected getInt32(offset: number): number {
    return this.view.getInt32(this.base + offset, true);
  }

  protected getFloat64(offset: number): number {
    return this.view.getFloat64(this.base + offset, true);
  }

  /**
   * Reads a string at a known offset with variable length.
   * NOTE: For objects, string offsets are usually variable, so this
   * requires the parent to calculate the offset or use a table.
   */
  protected getString(offset: number): string {
    const pos = this.base + offset;
    if (pos < 0 || pos >= this.u8.length) {
      throw new RangeError(`xpb: AccessorView.getString: offset ${offset} out of bounds`);
    }
    const first = this.u8[pos];
    let len = first;
    let headerSize = 1;

    if (first === CompactLengthMarker) {
        if (pos + 5 > this.u8.length) {
          throw new Error('xpb: AccessorView.getString: EOF reading extended length');
        }
        headerSize = 5;
        len = this.view.getUint32(pos + 1, true);
    }

    const start = pos + headerSize;
    if (start + len > this.u8.length) {
      throw new Error('xpb: AccessorView.getString: EOF reading string bytes');
    }
    return textDecoder.decode(this.u8.subarray(start, start + len));
  }
}
