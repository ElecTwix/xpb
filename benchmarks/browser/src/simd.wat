(module
  (memory (export "mem") 100) ;; 100 pages = 6.4MB

  ;; Scalar ZigZag Decode
  ;; (n >> 1) ^ -(n & 1)
  ;; Params: ptr (i32), count (i32)
  (func $zigzag_scalar (export "zigzag_scalar") (param $ptr i32) (param $count i32)
    (local $end i32)
    (local $val i32)
    (local $res i32)
    
    ;; Calculate end ptr
    (local.set $end 
      (i32.add (local.get $ptr) (i32.mul (local.get $count) (i32.const 4)))
    )
    
    (block $done
      (loop $loop
        (br_if $done (i32.ge_u (local.get $ptr) (local.get $end)))
        
        ;; Load val
        (local.set $val (i32.load (local.get $ptr)))
        
        ;; (val >>> 1)
        (local.set $res (i32.shr_u (local.get $val) (i32.const 1)))
        
        ;; -(val & 1) -> (0 - (val & 1))
        (local.set $res 
          (i32.xor 
            (local.get $res) 
            (i32.sub (i32.const 0) (i32.and (local.get $val) (i32.const 1)))
          )
        )
        
        ;; Store
        (i32.store (local.get $ptr) (local.get $res))
        
        ;; Increment ptr
        (local.set $ptr (i32.add (local.get $ptr) (i32.const 4)))
        (br $loop)
      )
    )
  )

  ;; SIMD ZigZag Decode
  ;; Processes 4 integers at a time
  (func $zigzag_simd (export "zigzag_simd") (param $ptr i32) (param $count i32)
    (local $end i32)
    (local $v v128)
    (local $shifted v128)
    (local $mask v128)
    (local $negmask v128)
    
    ;; Calculate end ptr (count * 4)
    (local.set $end 
      (i32.add (local.get $ptr) (i32.mul (local.get $count) (i32.const 4)))
    )
    
    (block $done
      (loop $loop
        ;; Check if >= end (assuming count is multiple of 4 for simplicity)
        (br_if $done (i32.ge_u (local.get $ptr) (local.get $end)))
        
        ;; Load 128 bits (4 x i32)
        (local.set $v (v128.load (local.get $ptr)))
        
        ;; Shift Right Unsigned by 1
        (local.set $shifted (i32x4.shr_u (local.get $v) (i32.const 1)))
        
        ;; Mask: v & 1
        ;; We need a splat of 1 to AND with
        (local.set $mask (v128.and (local.get $v) (v128.const i32x4 1 1 1 1)))
        
        ;; Negate mask: 0 - mask
        ;; splat 0
        (local.set $negmask (i32x4.sub (v128.const i32x4 0 0 0 0) (local.get $mask)))
        
        ;; XOR
        (local.set $v (v128.xor (local.get $shifted) (local.get $negmask)))
        
        ;; Store
        (v128.store (local.get $ptr) (local.get $v))
        
        ;; ptr += 16
        (local.set $ptr (i32.add (local.get $ptr) (i32.const 16)))
        (br $loop)
      )
    )
  )
)
