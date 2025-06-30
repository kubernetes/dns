package sqllexer

import (
	"strings"
	"unicode"
)

type DBMSType string

const (
	// DBMSSQLServer is a MS SQL
	DBMSSQLServer       DBMSType = "mssql"
	DBMSSQLServerAlias1 DBMSType = "sql-server" // .Net tracer
	DBMSSQLServerAlias2 DBMSType = "sqlserver"  // Java tracer
	// DBMSPostgres is a PostgreSQL Server
	DBMSPostgres       DBMSType = "postgresql"
	DBMSPostgresAlias1 DBMSType = "postgres" // Ruby, JavaScript tracers
	// DBMSMySQL is a MySQL Server
	DBMSMySQL DBMSType = "mysql"
	// DBMSOracle is a Oracle Server
	DBMSOracle DBMSType = "oracle"
	// DBMSSnowflake is a Snowflake Server
	DBMSSnowflake DBMSType = "snowflake"
)

var dbmsAliases = map[DBMSType]DBMSType{
	DBMSSQLServerAlias1: DBMSSQLServer,
	DBMSSQLServerAlias2: DBMSSQLServer,
	DBMSPostgresAlias1:  DBMSPostgres,
}

func getDBMSFromAlias(alias DBMSType) DBMSType {
	if canonical, exists := dbmsAliases[alias]; exists {
		return canonical
	}
	return alias
}

var commands = []string{
	"SELECT",
	"INSERT",
	"UPDATE",
	"DELETE",
	"CREATE",
	"ALTER",
	"DROP",
	"JOIN",
	"GRANT",
	"REVOKE",
	"COMMIT",
	"BEGIN",
	"TRUNCATE",
	"MERGE",
	"EXECUTE",
	"EXEC",
	"EXPLAIN",
	"STRAIGHT_JOIN",
	"USE",
	"CLONE",
}

var tableIndicatorCommands = []string{
	"JOIN",
	"UPDATE",
	"STRAIGHT_JOIN", // MySQL
	"CLONE",         // Snowflake
}

var tableIndicatorKeywords = []string{
	"FROM",
	"INTO",
	"TABLE",
	"EXISTS", // Drop Table If Exists
	"ONLY",   // PostgreSQL
}

var keywords = []string{
	"ADD",
	"ALL",
	"AND",
	"ANY",
	"ASC",
	"BETWEEN",
	"BY",
	"CASE",
	"CHECK",
	"COLUMN",
	"CONSTRAINT",
	"DATABASE",
	"DECLARE",
	"DEFAULT",
	"DESC",
	"DISTINCT",
	"ELSE",
	"END",
	"ESCAPE",
	"EXISTS",
	"FOREIGN",
	"FROM",
	"GROUP",
	"HAVING",
	"IN",
	"INDEX",
	"INNER",
	"INTO",
	"IS",
	"KEY",
	"LEFT",
	"LIKE",
	"LIMIT",
	"NOT",
	"ON",
	"OR",
	"ORDER",
	"OUT",
	"OUTER",
	"PRIMARY",
	"PROCEDURE",
	"REPLACE",
	"RETURNS",
	"RIGHT",
	"ROLLBACK",
	"ROWNUM",
	"SET",
	"SOME",
	"TABLE",
	"TOP",
	"UNION",
	"UNIQUE",
	"VALUES",
	"VIEW",
	"WHERE",
	"CUBE",
	"ROLLUP",
	"LITERAL",
	"WINDOW",
	"VACCUM",
	"ANALYZE",
	"ILIKE",
	"USING",
	"ASSERTION",
	"DOMAIN",
	"CLUSTER",
	"COPY",
	"PLPGSQL",
	"TRIGGER",
	"TEMPORARY",
	"UNLOGGED",
	"RECURSIVE",
	"RETURNING",
	"OFFSET",
	"OF",
	"SKIP",
	"IF",
	"ONLY",
}

var (
	// Pre-defined constants for common values
	booleanValues = []string{
		"TRUE",
		"FALSE",
	}

	nullValues = []string{
		"NULL",
	}

	procedureNames = []string{
		"PROCEDURE",
		"PROC",
	}

	ctes = []string{
		"WITH",
	}

	alias = []string{
		"AS",
	}
)

// buildCombinedTrie combines all types of SQL keywords into a single trie
// This trie is used for efficient case-insensitive keyword matching during lexing
func buildCombinedTrie() *trieNode {
	root := &trieNode{children: make(map[rune]*trieNode)}

	// Add all types of keywords
	addToTrie(root, commands, COMMAND, false)
	addToTrie(root, keywords, KEYWORD, false)
	addToTrie(root, tableIndicatorCommands, COMMAND, true)
	addToTrie(root, tableIndicatorKeywords, KEYWORD, true)
	addToTrie(root, booleanValues, BOOLEAN, false)
	addToTrie(root, nullValues, NULL, false)
	addToTrie(root, procedureNames, PROC_INDICATOR, false)
	addToTrie(root, ctes, CTE_INDICATOR, false)
	addToTrie(root, alias, ALIAS_INDICATOR, false)

	return root
}

func addToTrie(root *trieNode, words []string, tokenType TokenType, isTableIndicator bool) {
	for _, word := range words {
		node := root
		// Convert to uppercase for case-insensitive matching
		for _, ch := range strings.ToUpper(word) {
			if next, exists := node.children[ch]; exists {
				node = next
			} else {
				next = &trieNode{children: make(map[rune]*trieNode)}
				node.children[ch] = next
				node = next
			}
		}
		node.isEnd = true
		node.tokenType = tokenType
		node.isTableIndicator = isTableIndicator
	}
}

var keywordRoot = buildCombinedTrie()

// TODO: Optimize these functions to work with rune positions instead of string operations
// They are currently used by obfuscator and normalizer, which we'll optimize later
func replaceDigits(token *Token, placeholder string) string {
	var replacedToken strings.Builder
	replacedToken.Grow(len(token.Value))

	var lastWasDigit bool
	for _, r := range token.Value {
		if isDigit(r) {
			if !lastWasDigit {
				replacedToken.WriteString(placeholder)
				lastWasDigit = true
			}
		} else {
			replacedToken.WriteRune(r)
			lastWasDigit = false
		}
	}

	return replacedToken.String()
}

func trimQuotes(token *Token) string {
	var trimmedToken strings.Builder
	trimmedToken.Grow(len(token.Value))

	for _, r := range token.Value {
		if isDoubleQuote(r) || r == '[' || r == ']' || r == '`' {
			// trimmedToken.WriteString(placeholder)
		} else {
			trimmedToken.WriteRune(r)
		}
	}
	token.hasQuotes = false
	return trimmedToken.String()
}

// isDigit checks if a rune is a digit (0-9)
func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

// isLeadingDigit checks if a rune is + or -
func isLeadingSign(ch rune) bool {
	return ch == '+' || ch == '-'
}

// isExponent checks if a rune is an exponent (e or E)
func isExpontent(ch rune) bool {
	return ch == 'e' || ch == 'E'
}

// isSpace checks if a rune is a space or newline
func isSpace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

// isAsciiLetter checks if a rune is an ASCII letter (a-z or A-Z)
func isAsciiLetter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

// isLetter checks if a rune is an ASCII letter (a-z or A-Z) or unicode letter
func isLetter(ch rune) bool {
	return isAsciiLetter(ch) || ch == '_' ||
		(ch > 127 && unicode.IsLetter(ch))
}

// isAlphaNumeric checks if a rune is an ASCII letter (a-z or A-Z), digit (0-9), or unicode number
func isAlphaNumeric(ch rune) bool {
	return isLetter(ch) || isDigit(ch) ||
		(ch > 127 && unicode.IsNumber(ch))
}

// isDoubleQuote checks if a rune is a double quote (")
func isDoubleQuote(ch rune) bool {
	return ch == '"'
}

// isSingleQuote checks if a rune is a single quote (')
func isSingleQuote(ch rune) bool {
	return ch == '\''
}

// isOperator checks if a rune is an operator
func isOperator(ch rune) bool {
	return ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '=' || ch == '<' || ch == '>' ||
		ch == '!' || ch == '&' || ch == '|' || ch == '^' || ch == '%' || ch == '~' || ch == '?' ||
		ch == '@' || ch == ':' || ch == '#'
}

// isWildcard checks if a rune is a wildcard (*)
func isWildcard(ch rune) bool {
	return ch == '*'
}

// isSinglelineComment checks if two runes are a single line comment (--)
func isSingleLineComment(ch rune, nextCh rune) bool {
	return ch == '-' && nextCh == '-'
}

// isMultiLineComment checks if two runes are a multi line comment (/*)
func isMultiLineComment(ch rune, nextCh rune) bool {
	return ch == '/' && nextCh == '*'
}

// isPunctuation checks if a rune is a punctuation character
func isPunctuation(ch rune) bool {
	return ch == '(' || ch == ')' || ch == ',' || ch == ';' || ch == '.' || ch == ':' ||
		ch == '[' || ch == ']' || ch == '{' || ch == '}'
}

// isEOF checks if a rune is EOF (end of file)
func isEOF(ch rune) bool {
	return ch == 0
}

// isIdentifier checks if a rune is an identifier
func isIdentifier(ch rune) bool {
	return ch == '.' || ch == '?' || ch == '$' || ch == '#' || ch == '/' || ch == '@' || ch == '!' || isLetter(ch) || isDigit(ch)
}

// isValueToken checks if a token is a value token
// A value token is a token that is not a space, comment, or EOF
func isValueToken(token *Token) bool {
	return token.Type != EOF && token.Type != SPACE && token.Type != COMMENT && token.Type != MULTILINE_COMMENT
}
