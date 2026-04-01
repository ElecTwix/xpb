use crate::wire;

pub struct Encoder {
    buf: Vec<u8>,
}

impl Encoder {
    pub fn new(capacity: usize) -> Self {
        Self { buf: Vec::with_capacity(capacity) }
    }

    pub fn reset(&mut self) {
        self.buf.clear();
    }

    pub fn finish(self) -> Vec<u8> {
        self.buf
    }

    pub fn as_bytes(&self) -> &[u8] {
        &self.buf
    }

    pub fn len(&self) -> usize {
        self.buf.len()
    }

    pub fn is_empty(&self) -> bool {
        self.buf.is_empty()
    }

    pub fn write_bool(&mut self, v: bool) {
        self.buf.push(if v { 1 } else { 0 });
    }

    pub fn write_int32(&mut self, v: i32) {
        self.buf.extend_from_slice(&v.to_le_bytes());
    }

    pub fn write_int64(&mut self, v: i64) {
        self.buf.extend_from_slice(&v.to_le_bytes());
    }

    pub fn write_uint32(&mut self, v: u32) {
        self.buf.extend_from_slice(&v.to_le_bytes());
    }

    pub fn write_uint64(&mut self, v: u64) {
        self.buf.extend_from_slice(&v.to_le_bytes());
    }

    pub fn write_float32(&mut self, v: f32) {
        self.buf.extend_from_slice(&v.to_le_bytes());
    }

    pub fn write_float64(&mut self, v: f64) {
        self.buf.extend_from_slice(&v.to_le_bytes());
    }

    pub fn write_string(&mut self, v: &str) {
        self.write_compact_length(v.len());
        self.buf.extend_from_slice(v.as_bytes());
    }

    pub fn write_bytes(&mut self, v: &[u8]) {
        self.write_compact_length(v.len());
        self.buf.extend_from_slice(v);
    }

    pub fn write_message(&mut self, data: &[u8]) {
        self.write_compact_length(data.len());
        self.buf.extend_from_slice(data);
    }

    pub fn append_raw(&mut self, data: &[u8]) {
        self.buf.extend_from_slice(data);
    }

    fn write_compact_length(&mut self, len: usize) {
        if len <= wire::COMPACT_LENGTH_THRESHOLD as usize {
            self.buf.push(len as u8);
        } else {
            self.buf.push(wire::COMPACT_LENGTH_MARKER);
            self.buf.extend_from_slice(&(len as u32).to_le_bytes());
        }
    }
}
