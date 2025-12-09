var __defProp = Object.defineProperty;
var __defNormalProp = (obj, key, value) => key in obj ? __defProp(obj, key, { enumerable: true, configurable: true, writable: true, value }) : obj[key] = value;
var __publicField = (obj, key, value) => {
  __defNormalProp(obj, typeof key !== "symbol" ? key + "" : key, value);
  return value;
};

// src/xpb-browser.ts
var textEncoder = new TextEncoder();
var textDecoder = new TextDecoder();
var Encoder = class {
  constructor(initialSize = 256) {
    __publicField(this, "buf");
    __publicField(this, "view");
    __publicField(this, "pos", 0);
    this.buf = new Uint8Array(initialSize);
    this.view = new DataView(this.buf.buffer);
  }
  ensureCapacity(needed) {
    if (this.pos + needed > this.buf.length) {
      const newSize = Math.max(this.buf.length * 2, this.pos + needed);
      const newBuf = new Uint8Array(newSize);
      newBuf.set(this.buf);
      this.buf = newBuf;
      this.view = new DataView(this.buf.buffer);
    }
  }
  finish() {
    return this.buf.subarray(0, this.pos);
  }
  reset() {
    this.pos = 0;
  }
  writeBool(v) {
    this.ensureCapacity(1);
    this.buf[this.pos++] = v ? 1 : 0;
  }
  writeInt32(v) {
    this.ensureCapacity(4);
    this.view.setInt32(this.pos, v, true);
    this.pos += 4;
  }
  writeString(v) {
    this.ensureCapacity(v.length * 3 + 5);
    const lenPos = this.pos++;
    const strLen = v.length;
    if (strLen < 40) {
      let isAscii = true;
      for (let i = 0; i < strLen; i++) {
        const c = v.charCodeAt(i);
        if (c > 127) {
          isAscii = false;
          break;
        }
        this.buf[this.pos + i] = c;
      }
      if (isAscii) {
        this.buf[lenPos] = strLen;
        this.pos += strLen;
        return;
      }
    }
    const result = textEncoder.encodeInto(v, this.buf.subarray(this.pos));
    this.buf[lenPos] = result.written;
    this.pos += result.written;
  }
};
var Decoder = class {
  constructor(data) {
    __publicField(this, "data");
    __publicField(this, "pos", 0);
    __publicField(this, "view");
    this.data = data;
    this.view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  }
  readBool() {
    return this.data[this.pos++] !== 0;
  }
  readInt32() {
    const v = this.view.getInt32(this.pos, true);
    this.pos += 4;
    return v;
  }
  readString() {
    const len = this.data[this.pos++];
    const str = textDecoder.decode(this.data.subarray(this.pos, this.pos + len));
    this.pos += len;
    return str;
  }
};
var FieldType = /* @__PURE__ */ ((FieldType2) => {
  FieldType2[FieldType2["Bool"] = 0] = "Bool";
  FieldType2[FieldType2["Int32"] = 1] = "Int32";
  FieldType2[FieldType2["String"] = 2] = "String";
  return FieldType2;
})(FieldType || {});
var SlabAllocator = class {
  constructor(size = 65536) {
    __publicField(this, "buf");
    __publicField(this, "view");
    __publicField(this, "pos", 0);
    this.buf = new Uint8Array(size);
    this.view = new DataView(this.buf.buffer);
  }
  reset() {
    this.pos = 0;
  }
};
function compileEncoder(schema) {
  const lines = [`
    var buf = slab.buf;
    var view = slab.view;
    var pos = slab.pos;
    var val, str, strLen, i, c, isAscii, lenPos;
  `];
  for (const field of schema.fields) {
    const access = `obj.${field.name}`;
    switch (field.type) {
      case 0 /* Bool */:
        lines.push(`buf[pos++] = ${access} ? 1 : 0;`);
        break;
      case 1 /* Int32 */:
        lines.push(`
          val = ${access};
          buf[pos++] = val;
          buf[pos++] = val >> 8;
          buf[pos++] = val >> 16;
          buf[pos++] = val >> 24;
        `);
        break;
      case 2 /* String */:
        lines.push(`
          str = ${access} || '';
          strLen = str.length;
          lenPos = pos++;
          
          if (strLen < 40) {
            isAscii = true;
            for (i = 0; i < strLen; i++) {
              c = str.charCodeAt(i);
              if (c > 127) { isAscii = false; break; }
              buf[pos + i] = c;
            }
            if (isAscii) {
              buf[lenPos] = strLen;
              pos += strLen;
            } else {
              var enc = textEncoder.encodeInto(str, buf.subarray(pos));
              buf[lenPos] = enc.written;
              pos += enc.written;
            }
          } else {
            var enc = textEncoder.encodeInto(str, buf.subarray(pos));
            buf[lenPos] = enc.written;
            pos += enc.written;
          }
        `);
        break;
    }
  }
  lines.push(`slab.pos = pos;`);
  return new Function("textEncoder", "slab", "obj", lines.join("\n")).bind(null, textEncoder);
}
function compileDecoder(schema) {
  const lines = [`
    var pos = 0;
    var obj = {};
    var val, len;
  `];
  for (const field of schema.fields) {
    switch (field.type) {
      case 0 /* Bool */:
        lines.push(`obj.${field.name} = buf[pos++] !== 0;`);
        break;
      case 1 /* Int32 */:
        lines.push(`
          obj.${field.name} = buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24);
          pos += 4;
        `);
        break;
      case 2 /* String */:
        lines.push(`
          len = buf[pos++];
          obj.${field.name} = textDecoder.decode(buf.subarray(pos, pos + len));
          pos += len;
        `);
        break;
    }
  }
  lines.push(`return obj;`);
  return new Function("textDecoder", "buf", "end", lines.join("\n")).bind(null, textDecoder);
}
window.XPB = {
  Encoder,
  Decoder,
  SlabAllocator,
  compileEncoder,
  compileDecoder,
  FieldType
};
export {
  Decoder,
  Encoder,
  FieldType,
  SlabAllocator,
  compileDecoder,
  compileEncoder
};
