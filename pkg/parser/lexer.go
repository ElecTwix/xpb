// Package parser implements a lexer and parser for XPB schema files.
package parser

import (
	"fmt"
	"strings"
	"unicode"
)

// TokenKind represents the kind of a token.
type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenIdent
	TokenNumber
	TokenColon
	TokenLBrace
	TokenRBrace
	TokenLBracket // [
	TokenRBracket // ]
	TokenLAngle   // <
	TokenRAngle   // >
	TokenComma    // ,
	TokenEquals   // =
	TokenQuestion
	TokenPackage
	TokenMessage
	TokenEnum
	TokenMap
	TokenComment
	TokenNewline
)

// Token represents a lexical token.
type Token struct {
	Kind   TokenKind
	Value  string
	Line   int
	Column int
}

// Lexer tokenizes XPB schema input.
type Lexer struct {
	input  string
	pos    int
	line   int
	column int
}

// NewLexer creates a new lexer for the given input.
func NewLexer(input string) *Lexer {
	return &Lexer{
		input:  input,
		pos:    0,
		line:   1,
		column: 1,
	}
}

// Next returns the next token.
func (l *Lexer) Next() Token {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return Token{Kind: TokenEOF, Line: l.line, Column: l.column}
	}

	ch := l.input[l.pos]
	startLine := l.line
	startColumn := l.column

	// Single character tokens
	switch ch {
	case ':':
		l.advance()
		return Token{Kind: TokenColon, Value: ":", Line: startLine, Column: startColumn}
	case '{':
		l.advance()
		return Token{Kind: TokenLBrace, Value: "{", Line: startLine, Column: startColumn}
	case '}':
		l.advance()
		return Token{Kind: TokenRBrace, Value: "}", Line: startLine, Column: startColumn}
	case '[':
		l.advance()
		return Token{Kind: TokenLBracket, Value: "[", Line: startLine, Column: startColumn}
	case ']':
		l.advance()
		return Token{Kind: TokenRBracket, Value: "]", Line: startLine, Column: startColumn}
	case '<':
		l.advance()
		return Token{Kind: TokenLAngle, Value: "<", Line: startLine, Column: startColumn}
	case '>':
		l.advance()
		return Token{Kind: TokenRAngle, Value: ">", Line: startLine, Column: startColumn}
	case ',':
		l.advance()
		return Token{Kind: TokenComma, Value: ",", Line: startLine, Column: startColumn}
	case '=':
		l.advance()
		return Token{Kind: TokenEquals, Value: "=", Line: startLine, Column: startColumn}
	case '?':
		l.advance()
		return Token{Kind: TokenQuestion, Value: "?", Line: startLine, Column: startColumn}
	case '\n':
		l.advance()
		l.line++
		l.column = 1
		return Token{Kind: TokenNewline, Value: "\n", Line: startLine, Column: startColumn}
	}

	// Comments
	if ch == '/' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '/' {
		return l.readComment(startLine, startColumn)
	}

	// Numbers (including negative)
	if unicode.IsDigit(rune(ch)) || (ch == '-' && l.pos+1 < len(l.input) && unicode.IsDigit(rune(l.input[l.pos+1]))) {
		return l.readNumber(startLine, startColumn)
	}

	// Identifiers and keywords
	if unicode.IsLetter(rune(ch)) || ch == '_' {
		return l.readIdent(startLine, startColumn)
	}

	// Unknown character - skip it
	l.advance()
	return l.Next()
}

func (l *Lexer) advance() {
	if l.pos < len(l.input) {
		l.pos++
		l.column++
	}
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
		} else {
			break
		}
	}
}

func (l *Lexer) readComment(startLine, startColumn int) Token {
	start := l.pos
	for l.pos < len(l.input) && l.input[l.pos] != '\n' {
		l.advance()
	}
	return Token{Kind: TokenComment, Value: l.input[start:l.pos], Line: startLine, Column: startColumn}
}

func (l *Lexer) readNumber(startLine, startColumn int) Token {
	start := l.pos
	// Handle negative sign
	if l.input[l.pos] == '-' {
		l.advance()
	}
	for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
		l.advance()
	}
	return Token{Kind: TokenNumber, Value: l.input[start:l.pos], Line: startLine, Column: startColumn}
}

func (l *Lexer) readIdent(startLine, startColumn int) Token {
	start := l.pos
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_' {
			l.advance()
		} else {
			break
		}
	}

	value := l.input[start:l.pos]

	// Check for keywords
	switch value {
	case "package":
		return Token{Kind: TokenPackage, Value: value, Line: startLine, Column: startColumn}
	case "message":
		return Token{Kind: TokenMessage, Value: value, Line: startLine, Column: startColumn}
	case "enum":
		return Token{Kind: TokenEnum, Value: value, Line: startLine, Column: startColumn}
	case "map":
		return Token{Kind: TokenMap, Value: value, Line: startLine, Column: startColumn}
	default:
		return Token{Kind: TokenIdent, Value: value, Line: startLine, Column: startColumn}
	}
}

// Peek returns the next token without consuming it.
func (l *Lexer) Peek() Token {
	savedPos := l.pos
	savedLine := l.line
	savedColumn := l.column
	tok := l.Next()
	l.pos = savedPos
	l.line = savedLine
	l.column = savedColumn
	return tok
}

// String returns a human-readable description of the token kind.
func (k TokenKind) String() string {
	switch k {
	case TokenEOF:
		return "EOF"
	case TokenIdent:
		return "identifier"
	case TokenNumber:
		return "number"
	case TokenColon:
		return ":"
	case TokenLBrace:
		return "{"
	case TokenRBrace:
		return "}"
	case TokenLBracket:
		return "["
	case TokenRBracket:
		return "]"
	case TokenLAngle:
		return "<"
	case TokenRAngle:
		return ">"
	case TokenComma:
		return ","
	case TokenEquals:
		return "="
	case TokenQuestion:
		return "?"
	case TokenPackage:
		return "package"
	case TokenMessage:
		return "message"
	case TokenEnum:
		return "enum"
	case TokenMap:
		return "map"
	case TokenComment:
		return "comment"
	case TokenNewline:
		return "newline"
	default:
		return fmt.Sprintf("token(%d)", k)
	}
}

// FormatError returns a formatted error message with location.
func FormatError(line, column int, format string, args ...any) string {
	msg := fmt.Sprintf(format, args...)
	return fmt.Sprintf("line %d, column %d: %s", line, column, msg)
}

// TokensFromString is a helper to get all tokens from input (for testing).
func TokensFromString(input string) []Token {
	lexer := NewLexer(input)
	var tokens []Token
	for {
		tok := lexer.Next()
		tokens = append(tokens, tok)
		if tok.Kind == TokenEOF {
			break
		}
	}
	return tokens
}

// PrintTokens returns a string representation of tokens (for debugging).
func PrintTokens(tokens []Token) string {
	var sb strings.Builder
	for _, tok := range tokens {
		sb.WriteString(fmt.Sprintf("[%s: %q] ", tok.Kind, tok.Value))
	}
	return sb.String()
}
