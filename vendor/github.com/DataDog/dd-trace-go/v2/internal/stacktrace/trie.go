// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package stacktrace

import (
	"sync"
)

// prefixTrie is a thread-safe trie data structure optimized for prefix matching.
// It's designed for high-performance concurrent read operations with occasional writes.
//
// Memory vs Performance Trade-offs:
// - Slightly higher initial memory overhead for data structure vs slice
// - Zero allocations during lookups (same as slice)
// - O(m) lookup time where m=string length (vs O(n) where n=prefix count)
// - Performance varies: slower for early matches, much faster for no-match scenarios
type prefixTrie struct {
	root *trieNode
	mu   sync.RWMutex
}

// trieNode represents a single node in the trie
type trieNode struct {
	children map[rune]*trieNode
	isEnd    bool // true if this node represents the end of a prefix
}

// newPrefixTrie creates a new empty PrefixTrie
func newPrefixTrie() *prefixTrie {
	return &prefixTrie{
		root: &trieNode{
			children: make(map[rune]*trieNode),
		},
	}
}

// Insert adds a prefix to the trie
func (t *prefixTrie) Insert(prefix string) {
	if prefix == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	node := t.root
	for _, ch := range prefix {
		if node.children[ch] == nil {
			node.children[ch] = &trieNode{
				children: make(map[rune]*trieNode),
			}
		}
		node = node.children[ch]
	}
	node.isEnd = true
}

// HasPrefix checks if the given string has any of the prefixes stored in the trie.
// Returns true if any prefix in the trie is a prefix of the input string.
func (t *prefixTrie) HasPrefix(s string) (found bool) {
	if s == "" {
		return false
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	node := t.root
	for _, ch := range s {
		if node.isEnd {
			return true
		}

		node = node.children[ch]
		if node == nil {
			return false
		}
	}

	return node.isEnd
}

// InsertAll adds multiple prefixes to the trie in a single operation
func (t *prefixTrie) InsertAll(prefixes []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}

		node := t.root
		for _, ch := range prefix {
			if node.children[ch] == nil {
				node.children[ch] = &trieNode{
					children: make(map[rune]*trieNode),
				}
			}
			node = node.children[ch]
		}
		node.isEnd = true
	}
}

// Size returns the number of prefixes stored in the trie
func (t *prefixTrie) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.countPrefixes(t.root)
}

// countPrefixes recursively counts the number of complete prefixes in the trie
func (t *prefixTrie) countPrefixes(node *trieNode) int {
	if node == nil {
		return 0
	}

	count := 0
	if node.isEnd {
		count = 1
	}

	for _, child := range node.children {
		count += t.countPrefixes(child)
	}

	return count
}

// Clear removes all prefixes from the trie
func (t *prefixTrie) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.root = &trieNode{
		children: make(map[rune]*trieNode),
	}
}

// segmentPrefixTrie is a path segment-based trie optimized for "/" delimited paths.
// It stores path segments (e.g., "github.com", "DataDog") as nodes instead of individual characters,
// providing better memory efficiency and potentially faster lookups for module paths.
type segmentPrefixTrie struct {
	root *segmentTrieNode
	mu   sync.RWMutex
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

// Insert adds a prefix to the segment trie
func (t *segmentPrefixTrie) Insert(prefix string) {
	if prefix == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

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
func (t *segmentPrefixTrie) HasPrefix(s string) (found bool) {
	if s == "" {
		return false
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

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

// InsertAll adds multiple prefixes to the segment trie in a single operation
func (t *segmentPrefixTrie) InsertAll(prefixes []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

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

// Size returns the number of prefixes stored in the segment trie
func (t *segmentPrefixTrie) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

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

// Clear removes all prefixes from the segment trie
func (t *segmentPrefixTrie) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.root = &segmentTrieNode{
		children: make(map[string]*segmentTrieNode),
	}
}
