package sqllexer

import (
	"unicode/utf8"
)

type TokenType int

const (
	ERROR TokenType = iota
	EOF
	SPACE                  // space or newline
	STRING                 // string literal
	INCOMPLETE_STRING      // incomplete string literal so that we can obfuscate it, e.g. 'abc
	NUMBER                 // number literal
	IDENT                  // identifier
	QUOTED_IDENT           // quoted identifier
	OPERATOR               // operator
	WILDCARD               // wildcard *
	COMMENT                // comment
	MULTILINE_COMMENT      // multiline comment
	PUNCTUATION            // punctuation
	DOLLAR_QUOTED_FUNCTION // dollar quoted function
	DOLLAR_QUOTED_STRING   // dollar quoted string
	POSITIONAL_PARAMETER   // numbered parameter
	BIND_PARAMETER         // bind parameter
	FUNCTION               // function
	SYSTEM_VARIABLE        // system variable
	UNKNOWN                // unknown token
	COMMAND                // SQL commands like SELECT, INSERT
	KEYWORD                // Other SQL keywords
	JSON_OP                // JSON operators
	BOOLEAN                // boolean literal
	NULL                   // null literal
	PROC_INDICATOR         // procedure indicator
	CTE_INDICATOR          // CTE indicator
	ALIAS_INDICATOR        // alias indicator
)

// Token represents a SQL token with its type and value.
type Token struct {
	Type             TokenType
	Value            string
	isTableIndicator bool // true if the token is a table indicator
	hasDigits        bool
	hasQuotes        bool           // private - only used by trimQuotes
	lastValueToken   LastValueToken // private - internal state
}

type LastValueToken struct {
	Type             TokenType
	Value            string
	isTableIndicator bool
}

// getLastValueToken can be private since it's only used internally
func (t *Token) getLastValueToken() *LastValueToken {
	t.lastValueToken.Type = t.Type
	t.lastValueToken.Value = t.Value
	t.lastValueToken.isTableIndicator = t.isTableIndicator
	return &t.lastValueToken
}

type LexerConfig struct {
	DBMS DBMSType `json:"dbms,omitempty"`
}

type lexerOption func(*LexerConfig)

func WithDBMS(dbms DBMSType) lexerOption {
	dbms = getDBMSFromAlias(dbms)
	return func(c *LexerConfig) {
		c.DBMS = dbms
	}
}

type trieNode struct {
	children         map[rune]*trieNode
	isEnd            bool
	tokenType        TokenType
	isTableIndicator bool
}

// SQL Lexer inspired from Rob Pike's talk on Lexical Scanning in Go
type Lexer struct {
	src              string // the input src string
	cursor           int    // the current position of the cursor
	start            int    // the start position of the current token
	config           *LexerConfig
	token            *Token
	hasQuotes        bool // true if any quotes in token
	hasDigits        bool // true if the token has digits
	isTableIndicator bool // true if the token is a table indicator
}

func New(input string, opts ...lexerOption) *Lexer {
	lexer := &Lexer{
		src:    input,
		config: &LexerConfig{},
		token:  &Token{},
	}
	for _, opt := range opts {
		opt(lexer.config)
	}
	return lexer
}

// Scan scans the next token and returns it.
func (s *Lexer) Scan() *Token {
	ch := s.peek()
	switch {
	case isSpace(ch):
		return s.scanWhitespace()
	case isLetter(ch):
		return s.scanIdentifier(ch)
	case isDoubleQuote(ch):
		return s.scanDoubleQuotedIdentifier('"')
	case isSingleQuote(ch):
		return s.scanString()
	case isSingleLineComment(ch, s.lookAhead(1)):
		return s.scanSingleLineComment(ch)
	case isMultiLineComment(ch, s.lookAhead(1)):
		return s.scanMultiLineComment()
	case isLeadingSign(ch):
		// if the leading sign is followed by a digit, then it's a number
		// although this is not strictly true, it's good enough for our purposes
		nextCh := s.lookAhead(1)
		if isDigit(nextCh) || nextCh == '.' {
			return s.scanNumberWithLeadingSign()
		}
		return s.scanOperator(ch)
	case isDigit(ch):
		return s.scanNumber(ch)
	case isWildcard(ch):
		return s.scanWildcard()
	case ch == '$':
		if isDigit(s.lookAhead(1)) {
			// if the dollar sign is followed by a digit, then it's a numbered parameter
			return s.scanPositionalParameter()
		}
		if s.config.DBMS == DBMSSQLServer && isLetter(s.lookAhead(1)) {
			return s.scanIdentifier(ch)
		}
		return s.scanDollarQuotedString()
	case ch == ':':
		if s.config.DBMS == DBMSOracle && isAlphaNumeric(s.lookAhead(1)) {
			return s.scanBindParameter()
		}
		return s.scanOperator(ch)
	case ch == '`':
		if s.config.DBMS == DBMSMySQL {
			return s.scanDoubleQuotedIdentifier('`')
		}
		return s.scanUnknown() // backtick is only valid in mysql
	case ch == '#':
		if s.config.DBMS == DBMSSQLServer {
			return s.scanIdentifier(ch)
		} else if s.config.DBMS == DBMSMySQL {
			// MySQL treats # as a comment
			return s.scanSingleLineComment(ch)
		}
		return s.scanOperator(ch)
	case ch == '@':
		if s.lookAhead(1) == '@' {
			if isAlphaNumeric(s.lookAhead(2)) {
				return s.scanSystemVariable()
			}
			s.start = s.cursor
			s.nextBy(2) // consume @@
			return s.emit(JSON_OP)
		}
		if isAlphaNumeric(s.lookAhead(1)) {
			if s.config.DBMS == DBMSSnowflake {
				return s.scanIdentifier(ch)
			}
			return s.scanBindParameter()
		}
		if s.lookAhead(1) == '?' || s.lookAhead(1) == '>' {
			s.start = s.cursor
			s.nextBy(2) // consume @? or @>
			return s.emit(JSON_OP)
		}
		fallthrough
	case isOperator(ch):
		return s.scanOperator(ch)
	case isPunctuation(ch):
		if ch == '[' && s.config.DBMS == DBMSSQLServer {
			return s.scanDoubleQuotedIdentifier('[')
		}
		return s.scanPunctuation()
	case isEOF(ch):
		return s.emit(EOF)
	default:
		return s.scanUnknown()
	}
}

// lookAhead returns the rune n positions ahead of the cursor.
func (s *Lexer) lookAhead(n int) rune {
	pos := s.cursor + n
	if pos >= len(s.src) || pos < 0 {
		return 0
	}
	// Fast path for ASCII
	b := s.src[pos]
	if b < utf8.RuneSelf {
		return rune(b)
	}
	// Slow path for non-ASCII
	r, _ := utf8.DecodeRuneInString(s.src[pos:])
	return r
}

// peek returns the rune at the cursor position.
func (s *Lexer) peek() rune {
	return s.lookAhead(0)
}

// nextBy advances the cursor by n positions and returns the rune at the cursor position.
func (s *Lexer) nextBy(n int) rune {
	// advance the cursor by n and return the rune at the cursor position
	if s.cursor+n > len(s.src) {
		return 0
	}
	s.cursor += n
	if s.cursor >= len(s.src) {
		return 0
	}
	// Fast path for ASCII
	b := s.src[s.cursor]
	if b < utf8.RuneSelf {
		return rune(b)
	}
	// Slow path for non-ASCII
	r, _ := utf8.DecodeRuneInString(s.src[s.cursor:])
	return r
}

// next advances the cursor by 1 position and returns the rune at the cursor position.
func (s *Lexer) next() rune {
	return s.nextBy(1)
}

func (s *Lexer) matchAt(match []rune) bool {
	if s.cursor+len(match) > len(s.src) {
		return false
	}
	for i, ch := range match {
		if s.src[s.cursor+i] != byte(ch) {
			return false
		}
	}
	return true
}

func (s *Lexer) scanNumberWithLeadingSign() *Token {
	s.start = s.cursor
	ch := s.next() // consume the leading sign
	return s.scanDecimalNumber(ch)
}

func (s *Lexer) scanNumber(ch rune) *Token {
	s.start = s.cursor
	return s.scanNumberic(ch)
}

func (s *Lexer) scanNumberic(ch rune) *Token {
	s.start = s.cursor
	if ch == '0' {
		nextCh := s.lookAhead(1)
		if nextCh == 'x' || nextCh == 'X' {
			return s.scanHexNumber()
		} else if nextCh >= '0' && nextCh <= '7' {
			return s.scanOctalNumber()
		}
	}

	ch = s.next() // consume first digit
	return s.scanDecimalNumber(ch)
}

func (s *Lexer) scanDecimalNumber(ch rune) *Token {
	// scan digits
	for isDigit(ch) || ch == '.' || isExpontent(ch) {
		if isExpontent(ch) {
			ch = s.next()
			if isLeadingSign(ch) {
				ch = s.next()
			}
		} else {
			ch = s.next()
		}
	}
	return s.emit(NUMBER)
}

func (s *Lexer) scanHexNumber() *Token {
	ch := s.nextBy(2) // consume 0x or 0X

	for isDigit(ch) || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F') {
		ch = s.next()
	}
	return s.emit(NUMBER)
}

func (s *Lexer) scanOctalNumber() *Token {
	ch := s.nextBy(2) // consume the leading 0 and number

	for '0' <= ch && ch <= '7' {
		ch = s.next()
	}
	return s.emit(NUMBER)
}

func (s *Lexer) scanString() *Token {
	s.start = s.cursor
	escaped := false
	escapedQuote := false

	ch := s.next() // consume opening quote

	for ; !isEOF(ch); ch = s.next() {
		if escaped {
			escaped = false
			escapedQuote = ch == '\''
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '\'' {
			s.next() // consume the closing quote
			return s.emit(STRING)
		}
	}
	// Special case: if we ended with an escaped quote (e.g. ESCAPE '\')
	if escapedQuote {
		return s.emit(STRING)
	}
	// If we get here, we hit EOF before finding closing quote
	return s.emit(INCOMPLETE_STRING)
}

func (s *Lexer) scanIdentifier(ch rune) *Token {
	s.start = s.cursor
	node := keywordRoot
	pos := s.cursor

	// If first character is Unicode, skip trie lookup
	if ch > 127 {
		for isIdentifier(ch) {
			s.hasDigits = s.hasDigits || isDigit(ch)
			ch = s.nextBy(utf8.RuneLen(ch))
		}
		if s.start == s.cursor {
			return s.scanUnknown()
		}
		return s.emit(IDENT)
	}

	// ASCII characters - try keyword matching
	for isAsciiLetter(ch) || ch == '_' {
		// Convert to uppercase for case-insensitive matching
		upperCh := ch
		if ch >= 'a' && ch <= 'z' {
			upperCh -= 32
		}

		// Try to follow trie path
		if next, exists := node.children[upperCh]; exists {
			node = next
			pos = s.cursor
			ch = s.next()
		} else {
			// No more matches possible in trie
			// Reset node for next potential keyword
			// and continue scanning identifier
			node = keywordRoot
			ch = s.next()
			break
		}
	}

	// If we found a complete keyword and next char is whitespace
	if node.isEnd && (isPunctuation(ch) || isSpace(ch) || isEOF(ch)) {
		s.cursor = pos + 1 // Include the last matched character
		s.isTableIndicator = node.isTableIndicator
		return s.emit(node.tokenType)
	}

	// Continue scanning identifier if no keyword match
	for isIdentifier(ch) {
		s.hasDigits = s.hasDigits || isDigit(ch)
		ch = s.nextBy(utf8.RuneLen(ch))
	}

	if s.start == s.cursor {
		return s.scanUnknown()
	}

	if ch == '(' {
		return s.emit(FUNCTION)
	}
	return s.emit(IDENT)
}

func (s *Lexer) scanDoubleQuotedIdentifier(delimiter rune) *Token {
	closingDelimiter := delimiter
	if delimiter == '[' {
		closingDelimiter = ']'
	}

	s.start = s.cursor
	s.hasQuotes = true
	ch := s.next() // consume the opening quote
	for {
		// encountered the closing quote
		// BUT if it's followed by .", then we should keep going
		// e.g. postgres "foo"."bar"
		// e.g. sqlserver [foo].[bar]
		if ch == closingDelimiter {
			specialCase := []rune{closingDelimiter, '.', delimiter}
			if s.matchAt([]rune(specialCase)) {
				ch = s.nextBy(3) // consume the "."
				continue
			}
			break
		}
		if isEOF(ch) {
			s.hasQuotes = false // if we hit EOF, we clear the quotes
			return s.emit(ERROR)
		}
		s.hasDigits = s.hasDigits || isDigit(ch)
		ch = s.next()
	}
	s.next() // consume the closing quote
	return s.emit(QUOTED_IDENT)
}

func (s *Lexer) scanWhitespace() *Token {
	// scan whitespace, tab, newline, carriage return
	s.start = s.cursor
	ch := s.next()
	for isSpace(ch) {
		ch = s.next()
	}
	return s.emit(SPACE)
}

func (s *Lexer) scanOperator(lastCh rune) *Token {
	s.start = s.cursor
	ch := s.next() // consume the first character

	// Check for json operators
	switch lastCh {
	case '-':
		if ch == '>' {
			ch = s.next()
			if ch == '>' {
				s.next()
				return s.emit(JSON_OP) // ->>
			}
			return s.emit(JSON_OP) // ->
		}
	case '#':
		if ch == '>' {
			ch = s.next()
			if ch == '>' {
				s.next()
				return s.emit(JSON_OP) // #>>
			}
			return s.emit(JSON_OP) // #>
		} else if ch == '-' {
			s.next()
			return s.emit(JSON_OP) // #-
		}
	case '?':
		if ch == '|' {
			s.next()
			return s.emit(JSON_OP) // ?|
		} else if ch == '&' {
			s.next()
			return s.emit(JSON_OP) // ?&
		}
	case '<':
		if ch == '@' {
			s.next()
			return s.emit(JSON_OP) // <@
		}
	}

	for isOperator(ch) && !(lastCh == '=' && (ch == '?' || ch == '@')) {
		// hack: we don't want to treat "=?" as an single operator
		lastCh = ch
		ch = s.next()
	}

	return s.emit(OPERATOR)
}

func (s *Lexer) scanWildcard() *Token {
	s.start = s.cursor
	s.next()
	return s.emit(WILDCARD)
}

func (s *Lexer) scanSingleLineComment(ch rune) *Token {
	s.start = s.cursor
	if ch == '#' {
		ch = s.next() // consume the opening #
	} else {
		ch = s.nextBy(2) // consume the opening dashes
	}
	for ch != '\n' && !isEOF(ch) {
		ch = s.next()
	}
	return s.emit(COMMENT)
}

func (s *Lexer) scanMultiLineComment() *Token {
	s.start = s.cursor
	ch := s.nextBy(2) // consume the opening slash and asterisk
	for {
		if ch == '*' && s.lookAhead(1) == '/' {
			s.nextBy(2) // consume the closing asterisk and slash
			break
		}
		if isEOF(ch) {
			// encountered EOF before closing comment
			// this usually happens when the comment is truncated
			return s.emit(ERROR)
		}
		ch = s.next()
	}
	return s.emit(MULTILINE_COMMENT)
}

func (s *Lexer) scanPunctuation() *Token {
	s.start = s.cursor
	s.next()
	return s.emit(PUNCTUATION)
}

func (s *Lexer) scanDollarQuotedString() *Token {
	s.start = s.cursor
	ch := s.next() // consume the dollar sign
	tagStart := s.cursor

	for s.cursor < len(s.src) && ch != '$' {
		ch = s.next()
	}
	s.next()                            // consume the closing dollar sign of the tag
	tag := s.src[tagStart-1 : s.cursor] // include the opening and closing dollar sign e.g. $tag$

	for s.cursor < len(s.src) {
		if s.matchAt([]rune(tag)) {
			s.nextBy(len(tag)) // consume the closing tag
			if tag == "$func$" {
				return s.emit(DOLLAR_QUOTED_FUNCTION)
			}
			return s.emit(DOLLAR_QUOTED_STRING)
		}
		s.next()
	}
	return s.emit(ERROR)
}

func (s *Lexer) scanPositionalParameter() *Token {
	s.start = s.cursor
	ch := s.nextBy(2) // consume the dollar sign and the number
	for {
		if !isDigit(ch) {
			break
		}
		ch = s.next()
	}
	return s.emit(POSITIONAL_PARAMETER)
}

func (s *Lexer) scanBindParameter() *Token {
	s.start = s.cursor
	ch := s.nextBy(2) // consume the (colon|at sign) and the char
	for {
		if !isAlphaNumeric(ch) {
			break
		}
		ch = s.next()
	}
	return s.emit(BIND_PARAMETER)
}

func (s *Lexer) scanSystemVariable() *Token {
	s.start = s.cursor
	ch := s.nextBy(2) // consume @@
	// Must be followed by at least one alphanumeric character
	if !isAlphaNumeric(ch) {
		return s.emit(ERROR)
	}
	for isAlphaNumeric(ch) {
		ch = s.next()
	}
	return s.emit(SYSTEM_VARIABLE)
}

func (s *Lexer) scanUnknown() *Token {
	// When we see an unknown token, we advance the cursor until we see something that looks like a token boundary.
	s.start = s.cursor
	s.next()
	return s.emit(UNKNOWN)
}

// Modify emit function to use positions and maintain links
func (s *Lexer) emit(t TokenType) *Token {
	tok := s.token
	lastValueToken := tok.lastValueToken

	// Zero other fields
	*tok = Token{
		Type:             t,
		Value:            s.src[s.start:s.cursor],
		isTableIndicator: s.isTableIndicator,
		lastValueToken:   lastValueToken,
	}

	tok.hasDigits = s.hasDigits
	tok.hasQuotes = s.hasQuotes

	// Reset lexer state
	s.start = s.cursor
	s.isTableIndicator = false
	s.hasDigits = false

	return tok
}
