use crate::error::{XpbError, Result};
use crate::wire;

pub struct Decoder<'a> {
    buf: &'a [u8],
    pos: usize,
}

impl<'a> Decoder<'a> {
    pub fn new(buf: &'a [u8]) -> Self {
        Self { buf, pos: 0 }
    }

    pub fn reset(&mut self, buf: &'a [u8]) {
        self.buf = buf;
        self.pos = 0;
    }

    pub fn remaining(&self) -> usize {
        self.buf.len() - self.pos
    }

    pub fn eof(&self) -> bool {
        self.pos >= self.buf.len()
    }

    pub fn skip(&mut self, n: usize) -> Result<()> {
        if self.pos + n > self.buf.len() {
            return Err(XpbError::UnexpectedEof);
        }
        self.pos += n;
        Ok(())
    }

    pub fn read_bool(&mut self) -> Result<bool> {
        if self.pos + 1 > self.buf.len() {
            return Err(XpbError::UnexpectedEof);
        }
        let v = self.buf[self.pos];
        self.pos += 1;
        Ok(v != 0)
    }

    pub fn read_int32(&mut self) -> Result<i32> {
        let bytes = self.read_fixed(wire::SIZE_INT32)?;
        Ok(i32::from_le_bytes(bytes.try_into().unwrap()))
    }

    pub fn read_int64(&mut self) -> Result<i64> {
        let bytes = self.read_fixed(wire::SIZE_INT64)?;
        Ok(i64::from_le_bytes(bytes.try_into().unwrap()))
    }

    pub fn read_uint32(&mut self) -> Result<u32> {
        let bytes = self.read_fixed(wire::SIZE_UINT32)?;
        Ok(u32::from_le_bytes(bytes.try_into().unwrap()))
    }

    pub fn read_uint64(&mut self) -> Result<u64> {
        let bytes = self.read_fixed(wire::SIZE_UINT64)?;
        Ok(u64::from_le_bytes(bytes.try_into().unwrap()))
    }

    pub fn read_float32(&mut self) -> Result<f32> {
        let bytes = self.read_fixed(wire::SIZE_FLOAT32)?;
        Ok(f32::from_le_bytes(bytes.try_into().unwrap()))
    }

    pub fn read_float64(&mut self) -> Result<f64> {
        let bytes = self.read_fixed(wire::SIZE_FLOAT64)?;
        Ok(f64::from_le_bytes(bytes.try_into().unwrap()))
    }

    pub fn read_string(&mut self) -> Result<String> {
        let len = self.read_compact_length()?;
        let bytes = self.read_n(len)?;
        String::from_utf8(bytes.to_vec())
            .map_err(|e| XpbError::InvalidData(format!("invalid utf8: {}", e)))
    }

    pub fn read_bytes(&mut self) -> Result<Vec<u8>> {
        let len = self.read_compact_length()?;
        let bytes = self.read_n(len)?;
        Ok(bytes.to_vec())
    }

    pub fn read_message_bytes(&mut self) -> Result<Vec<u8>> {
        self.read_bytes()
    }

    /// Validate and return an array length read from the wire. The caller
    /// MUST supply `max_elements` — the runtime does not pick a default,
    /// so application-level allocation policy is visible at every call
    /// site. A count that is negative, exceeds `max_elements`, or cannot
    /// fit in the remaining buffer (each element occupies at least
    /// `element_min_bytes` on the wire) is rejected before any
    /// allocation. Pass `element_min_bytes = 1` for variable-length
    /// elements (string, bytes, message). Pass `element_min_bytes = 0` to
    /// skip the buffer bound (only safe for fully trusted input).
    pub fn read_array_count(
        &mut self,
        element_min_bytes: usize,
        max_elements: usize,
    ) -> Result<usize> {
        let n = self.read_int32()?;
        if n < 0 {
            return Err(XpbError::InvalidData(format!(
                "negative array count: {}",
                n
            )));
        }
        let n_usize = n as usize;
        if n_usize > max_elements {
            return Err(XpbError::InvalidData(format!(
                "array count {} exceeds caller-supplied max {}",
                n, max_elements
            )));
        }
        if element_min_bytes > 0 {
            let max = self.remaining() / element_min_bytes;
            if n_usize > max {
                return Err(XpbError::InvalidData(format!(
                    "array count {} exceeds buffer-bounded max {}",
                    n, max
                )));
            }
        }
        Ok(n_usize)
    }

    fn read_fixed(&mut self, n: usize) -> Result<&'a [u8]> {
        self.read_n(n)
    }

    fn read_n(&mut self, n: usize) -> Result<&'a [u8]> {
        if self.pos + n > self.buf.len() {
            return Err(XpbError::UnexpectedEof);
        }
        let slice = &self.buf[self.pos..self.pos + n];
        self.pos += n;
        Ok(slice)
    }

    fn read_compact_length(&mut self) -> Result<usize> {
        if self.pos >= self.buf.len() {
            return Err(XpbError::UnexpectedEof);
        }
        let first = self.buf[self.pos];
        self.pos += 1;
        if first != wire::COMPACT_LENGTH_MARKER {
            return Ok(first as usize);
        }
        // 4-byte length follows
        let bytes = self.read_fixed(4)?;
        Ok(u32::from_le_bytes(bytes.try_into().unwrap()) as usize)
    }
}
