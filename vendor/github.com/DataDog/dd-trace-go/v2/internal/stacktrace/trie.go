// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package stacktrace

// segmentPrefixTrie is a path segment-based trie optimized for "/" delimited paths.
// It stores path segments (e.g., "github.com", "DataDog") as nodes instead of individual characters,
// providing better memory efficiency and potentially faster lookups for module paths.
//
// Concurrency: This trie follows a write-once-read-many (WORM) pattern where all writes
// occur during package initialization (init function) before any concurrent access begins.
// After initialization, the trie is effectively immutable and can be safely read by multiple
// goroutines without synchronization.
type segmentPrefixTrie struct {
	root *segmentTrieNode
	// No mutex needed - structure is immutable after init()
}

// segmentTrieNode represents a single path segment node in the trie
type segmentTrieNode struct {
	children map[string]*segmentTrieNode
	isEnd    bool // true if this node represents the end of a prefix
}

// newSegmentPrefixTrie creates a new empty segmentPrefixTrie
func newSegmentPrefixTrie() *segmentPrefixTrie {
	return &segmentPrefixTrie{
		root: &segmentTrieNode{
			children: make(map[string]*segmentTrieNode),
		},
	}
}

// Insert adds a prefix to the segment trie.
// This method should only be called during initialization before any concurrent access.
func (t *segmentPrefixTrie) Insert(prefix string) {
	if prefix == "" {
		return
	}

	node := t.root
	start := 0

	for i := 0; i <= len(prefix); i++ {
		// Check for segment boundary (slash or end of string)
		if i == len(prefix) || prefix[i] == '/' {
			if i > start {
				segment := prefix[start:i]

				if node.children[segment] == nil {
					node.children[segment] = &segmentTrieNode{
						children: make(map[string]*segmentTrieNode),
					}
				}
				node = node.children[segment]
			}
			start = i + 1
		}
	}
	node.isEnd = true
}

// HasPrefix checks if the given string has any of the prefixes stored in the segment trie.
// Safe for concurrent use after initialization.
func (t *segmentPrefixTrie) HasPrefix(s string) (found bool) {
	if s == "" {
		return false
	}

	node := t.root
	start := 0

	for i := 0; i <= len(s); i++ {
		// Check for segment boundary (slash or end of string)
		if i == len(s) || s[i] == '/' {
			if i > start {
				segment := s[start:i]

				if node.isEnd {
					return true
				}

				node = node.children[segment]
				if node == nil {
					return false
				}
			}
			start = i + 1
		}
	}

	return node.isEnd
}

// InsertAll adds multiple prefixes to the segment trie in a single operation.
// This method should only be called during initialization before any concurrent access.
func (t *segmentPrefixTrie) InsertAll(prefixes []string) {
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}

		node := t.root
		start := 0

		for i := 0; i <= len(prefix); i++ {
			// Check for segment boundary (slash or end of string)
			if i == len(prefix) || prefix[i] == '/' {
				if i > start {
					segment := prefix[start:i]

					if node.children[segment] == nil {
						node.children[segment] = &segmentTrieNode{
							children: make(map[string]*segmentTrieNode),
						}
					}
					node = node.children[segment]
				}
				start = i + 1
			}
		}
		node.isEnd = true
	}
}

// Size returns the number of prefixes stored in the segment trie.
// Safe for concurrent use after initialization.
func (t *segmentPrefixTrie) Size() int {
	return t.countSegmentPrefixes(t.root)
}

// countSegmentPrefixes recursively counts the number of complete prefixes in the segment trie
func (t *segmentPrefixTrie) countSegmentPrefixes(node *segmentTrieNode) int {
	if node == nil {
		return 0
	}

	count := 0
	if node.isEnd {
		count = 1
	}

	for _, child := range node.children {
		count += t.countSegmentPrefixes(child)
	}

	return count
}

// Clear removes all prefixes from the segment trie.
// This method should only be called during initialization before any concurrent access.
func (t *segmentPrefixTrie) Clear() {
	t.root = &segmentTrieNode{
		children: make(map[string]*segmentTrieNode),
	}
}
