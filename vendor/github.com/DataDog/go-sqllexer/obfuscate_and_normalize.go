package sqllexer

import "strings"

// ObfuscateAndNormalize takes an input SQL string and returns an normalized SQL string with metadata
// This function is a convenience function that combines the Obfuscator and Normalizer in one pass
func ObfuscateAndNormalize(input string, obfuscator *Obfuscator, normalizer *Normalizer, lexerOpts ...lexerOption) (normalizedSQL string, statementMetadata *StatementMetadata, err error) {
	lexer := New(input, lexerOpts...)
	var normalizedSQLBuilder strings.Builder
	normalizedSQLBuilder.Grow(len(input))

	meta := &metadataSet{
		tablesSet:     map[string]struct{}{},
		commentsSet:   map[string]struct{}{},
		commandsSet:   map[string]struct{}{},
		proceduresSet: map[string]struct{}{},
	}

	statementMetadata = &StatementMetadata{
		Tables:     []string{},
		Comments:   []string{},
		Commands:   []string{},
		Procedures: []string{},
	}

	obfuscate := func(token *Token, lastValueToken *LastValueToken) {
		obfuscator.ObfuscateTokenValue(token, lastValueToken, lexerOpts...)
	}

	// Pass obfuscation as the pre-process step
	if err = normalizer.normalizeToken(lexer, &normalizedSQLBuilder, meta, statementMetadata, obfuscate, lexerOpts...); err != nil {
		return "", nil, err
	}

	normalizedSQL = normalizedSQLBuilder.String()
	statementMetadata.Size = meta.size
	return normalizer.trimNormalizedSQL(normalizedSQL), statementMetadata, nil
}
