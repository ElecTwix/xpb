use std::fmt;

#[derive(Debug, Clone, PartialEq)]
pub enum XpbError {
    /// Not enough data in the buffer.
    UnexpectedEof,
    /// Invalid data encountered during decode.
    InvalidData(String),
}

impl fmt::Display for XpbError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            XpbError::UnexpectedEof => write!(f, "xpb: unexpected end of data"),
            XpbError::InvalidData(msg) => write!(f, "xpb: invalid data: {}", msg),
        }
    }
}

impl std::error::Error for XpbError {}

pub type Result<T> = std::result::Result<T, XpbError>;
