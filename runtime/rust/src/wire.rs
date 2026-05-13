/// Max length that fits in a single byte (1..=254).
pub const COMPACT_LENGTH_THRESHOLD: u8 = 254;

/// Marker byte indicating a 4-byte length follows.
pub const COMPACT_LENGTH_MARKER: u8 = 0xFF;

pub const SIZE_BOOL: usize = 1;
pub const SIZE_INT32: usize = 4;
pub const SIZE_INT64: usize = 8;
pub const SIZE_UINT32: usize = 4;
pub const SIZE_UINT64: usize = 8;
pub const SIZE_FLOAT32: usize = 4;
pub const SIZE_FLOAT64: usize = 8;

/// Cap on nested-message decode recursion. Mirrors xpb.MaxDecodeDepth in
/// the Go runtime / MaxDecodeDepth in TS / XPB_MAX_DECODE_DEPTH in C.
/// Generated `unmarshal_at(depth)` shims compare against this before doing
/// any work, refusing adversarial deeply-nested payloads that would blow
/// the Rust thread stack.
pub const MAX_DECODE_DEPTH: u32 = 64;
