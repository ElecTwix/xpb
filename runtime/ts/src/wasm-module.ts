/**
 * Minimal WASM module for XPB (handwritten WAT)
 * This is a text representation that can be compiled with wat2wasm
 */

// varint.wat - Compile with: wat2wasm varint.wat -o varint.wasm
const WASM_MODULE = `
(module
  ;; Import memory from JavaScript
  (import "env" "memory" (memory 1))

  ;; Decode unsigned varint at offset, return (value, bytes_read) packed as i64
  ;; Low 32 bits = value, high 32 bits = bytes_read
  (func (export "decode_varint") (param $offset i32) (result i64)
    (local $result i32)
    (local $shift i32)
    (local $byte i32)
    (local $pos i32)
    
    (local.set $pos (local.get $offset))
    (local.set $result (i32.const 0))
    (local.set $shift (i32.const 0))
    
    (block $done
      (loop $read
        ;; byte = memory[pos++]
        (local.set $byte (i32.load8_u (local.get $pos)))
        (local.set $pos (i32.add (local.get $pos) (i32.const 1)))
        
        ;; result |= (byte & 0x7f) << shift
        (local.set $result 
          (i32.or 
            (local.get $result)
            (i32.shl 
              (i32.and (local.get $byte) (i32.const 0x7f))
              (local.get $shift))))
        
        ;; if (byte & 0x80) == 0 break
        (br_if $done (i32.eqz (i32.and (local.get $byte) (i32.const 0x80))))
        
        ;; shift += 7
        (local.set $shift (i32.add (local.get $shift) (i32.const 7)))
        
        ;; continue if shift < 35
        (br_if $read (i32.lt_u (local.get $shift) (i32.const 35)))
      )
    )
    
    ;; Return packed: (bytes_read << 32) | result
    (i64.or
      (i64.extend_i32_u (local.get $result))
      (i64.shl 
        (i64.extend_i32_u (i32.sub (local.get $pos) (local.get $offset)))
        (i64.const 32)))
  )

  ;; Encode unsigned varint at offset, return bytes_written
  (func (export "encode_varint") (param $value i32) (param $offset i32) (result i32)
    (local $pos i32)
    
    (local.set $pos (local.get $offset))
    
    (block $done
      (loop $write
        ;; if value < 0x80, write final byte and exit
        (if (i32.lt_u (local.get $value) (i32.const 0x80))
          (then
            (i32.store8 (local.get $pos) (local.get $value))
            (local.set $pos (i32.add (local.get $pos) (i32.const 1)))
            (br $done)))
        
        ;; memory[pos++] = (value & 0x7f) | 0x80
        (i32.store8 (local.get $pos) 
          (i32.or (i32.and (local.get $value) (i32.const 0x7f)) (i32.const 0x80)))
        (local.set $pos (i32.add (local.get $pos) (i32.const 1)))
        
        ;; value >>= 7
        (local.set $value (i32.shr_u (local.get $value) (i32.const 7)))
        
        (br $write)
      )
    )
    
    (i32.sub (local.get $pos) (local.get $offset))
  )

  ;; Zigzag encode
  (func (export "zigzag_encode") (param $n i32) (result i32)
    (i32.xor
      (i32.shl (local.get $n) (i32.const 1))
      (i32.shr_s (local.get $n) (i32.const 31))))

  ;; Zigzag decode
  (func (export "zigzag_decode") (param $n i32) (result i32)
    (i32.xor
      (i32.shr_u (local.get $n) (i32.const 1))
      (i32.sub (i32.const 0) (i32.and (local.get $n) (i32.const 1)))))
)
`;

export default WASM_MODULE;
