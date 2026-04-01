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
