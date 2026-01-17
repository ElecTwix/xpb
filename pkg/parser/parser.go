package parser

import (
	"errors"
	"strconv"

	"github.com/anthropic/xpb/pkg/ast"
)

// Parser parses XPB schema files into an AST.
type Parser struct {
	lexer *Lexer
	curr  Token
}

// NewParser creates a new parser for the given input.
func NewParser(input string) *Parser {
	p := &Parser{
		lexer: NewLexer(input),
	}
	p.advance() // Prime the parser
	return p
}

// Parse parses the input and returns the AST.
func (p *Parser) Parse() (*ast.File, error) {
	file := &ast.File{}

	for p.curr.Kind != TokenEOF {
		switch p.curr.Kind {
		case TokenPackage:
			pkg, err := p.parsePackage()
			if err != nil {
				return nil, err
			}
			file.Package = pkg
		case TokenMessage:
			msg, err := p.parseMessage()
			if err != nil {
				return nil, err
			}
			file.Messages = append(file.Messages, msg)
		case TokenEnum:
			enum, err := p.parseEnum()
			if err != nil {
				return nil, err
			}
			file.Enums = append(file.Enums, enum)
		case TokenComment, TokenNewline:
			p.advance() // Skip comments and newlines
		default:
			return nil, errors.New(FormatError(p.curr.Line, p.curr.Column,
				"unexpected token %s, expected 'package', 'message', or 'enum'", p.curr.Kind))
		}
	}

	return file, nil
}

func (p *Parser) advance() {
	p.curr = p.lexer.Next()
	// Skip comments and newlines in most contexts
	for p.curr.Kind == TokenComment || p.curr.Kind == TokenNewline {
		p.curr = p.lexer.Next()
	}
}

func (p *Parser) expect(kind TokenKind) error {
	if p.curr.Kind != kind {
		return errors.New(FormatError(p.curr.Line, p.curr.Column,
			"expected %s, got %s", kind, p.curr.Kind))
	}
	return nil
}

func (p *Parser) parsePackage() (string, error) {
	p.advance() // consume 'package'

	if err := p.expect(TokenIdent); err != nil {
		return "", err
	}
	name := p.curr.Value
	p.advance()

	return name, nil
}

func (p *Parser) parseMessage() (*ast.Message, error) {
	p.advance() // consume 'message'

	if err := p.expect(TokenIdent); err != nil {
		return nil, err
	}
	msg := &ast.Message{Name: p.curr.Value}
	p.advance()

	if err := p.expect(TokenLBrace); err != nil {
		return nil, err
	}
	p.advance()

	// Parse fields
	for p.curr.Kind != TokenRBrace && p.curr.Kind != TokenEOF {
		field, err := p.parseField()
		if err != nil {
			return nil, err
		}
		msg.Fields = append(msg.Fields, field)
	}

	if err := p.expect(TokenRBrace); err != nil {
		return nil, err
	}
	p.advance()

	return msg, nil
}

func (p *Parser) parseField() (*ast.Field, error) {
	field := &ast.Field{}

	// Field number
	if err := p.expect(TokenNumber); err != nil {
		return nil, err
	}
	num, err := strconv.ParseUint(p.curr.Value, 10, 32)
	if err != nil {
		return nil, errors.New(FormatError(p.curr.Line, p.curr.Column,
			"invalid field number: %s", p.curr.Value))
	}
	field.Number = uint32(num)
	p.advance()

	// Colon
	if err := p.expect(TokenColon); err != nil {
		return nil, err
	}
	p.advance()

	// Optional marker
	if p.curr.Kind == TokenQuestion {
		field.Optional = true
		p.advance()
	}

	// Check for repeated (array) type: []Type
	if p.curr.Kind == TokenLBracket {
		p.advance() // consume '['
		if err := p.expect(TokenRBracket); err != nil {
			return nil, err
		}
		p.advance() // consume ']'
		field.Repeated = true
	}

	// Check for map type: map<KeyType, ValueType>
	if p.curr.Kind == TokenMap {
		p.advance() // consume 'map'
		if err := p.expect(TokenLAngle); err != nil {
			return nil, err
		}
		p.advance() // consume '<'

		// Key type
		if err := p.expect(TokenIdent); err != nil {
			return nil, err
		}
		keyType, _ := ast.ParseTypeName(p.curr.Value)
		p.advance()

		// Comma
		if err := p.expect(TokenComma); err != nil {
			return nil, err
		}
		p.advance()

		// Value type
		if err := p.expect(TokenIdent); err != nil {
			return nil, err
		}
		valType, _ := ast.ParseTypeName(p.curr.Value)
		p.advance()

		// Close angle
		if err := p.expect(TokenRAngle); err != nil {
			return nil, err
		}
		p.advance()

		field.Type = ast.FieldType{
			Kind:    ast.TypeMap,
			KeyType: &keyType,
			ValType: &valType,
		}
	} else {
		// Regular type
		if err := p.expect(TokenIdent); err != nil {
			return nil, err
		}
		fieldType, ok := ast.ParseTypeName(p.curr.Value)
		if !ok {
			return nil, errors.New(FormatError(p.curr.Line, p.curr.Column,
				"unknown type: %s", p.curr.Value))
		}
		field.Type = fieldType
		p.advance()
	}

	// Field name
	if err := p.expect(TokenIdent); err != nil {
		return nil, err
	}
	field.Name = p.curr.Value
	p.advance()

	return field, nil
}

func (p *Parser) parseEnum() (*ast.Enum, error) {
	p.advance() // consume 'enum'

	if err := p.expect(TokenIdent); err != nil {
		return nil, err
	}
	enum := &ast.Enum{Name: p.curr.Value}
	p.advance()

	if err := p.expect(TokenLBrace); err != nil {
		return nil, err
	}
	p.advance()

	// Parse enum values
	for p.curr.Kind != TokenRBrace && p.curr.Kind != TokenEOF {
		value, err := p.parseEnumValue()
		if err != nil {
			return nil, err
		}
		enum.Values = append(enum.Values, value)
	}

	if err := p.expect(TokenRBrace); err != nil {
		return nil, err
	}
	p.advance()

	return enum, nil
}

func (p *Parser) parseEnumValue() (*ast.EnumValue, error) {
	// Name
	if err := p.expect(TokenIdent); err != nil {
		return nil, err
	}
	name := p.curr.Value
	p.advance()

	// Equals
	if err := p.expect(TokenEquals); err != nil {
		return nil, err
	}
	p.advance()

	// Number
	if err := p.expect(TokenNumber); err != nil {
		return nil, err
	}
	num, err := strconv.ParseInt(p.curr.Value, 10, 32)
	if err != nil {
		return nil, errors.New(FormatError(p.curr.Line, p.curr.Column,
			"invalid enum value: %s", p.curr.Value))
	}
	p.advance()

	return &ast.EnumValue{
		Name:   name,
		Number: int32(num),
	}, nil
}

// ParseFile is a convenience function to parse a schema file.
func ParseFile(input string) (*ast.File, error) {
	return NewParser(input).Parse()
}
