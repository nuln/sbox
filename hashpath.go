package sbox

import "path/filepath"

// HashPath generates a multi-level directory path from a hash string.
// This supports billion-scale storage by distributing files across 256^3 = 16M directories.
//
// Example: HashPath("abc123def456") → "ab/c1/23/abc123def456"
func HashPath(hash string) string {
	if len(hash) < 6 {
		return hash
	}
	return filepath.Join(hash[0:2], hash[2:4], hash[4:6], hash)
}

// HashPathWithExt generates a multi-level directory path with a file extension.
//
// Example: HashPathWithExt("abc123def456", ".json") → "ab/c1/23/abc123def456.json"
func HashPathWithExt(hash, ext string) string {
	if len(hash) < 6 {
		return hash + ext
	}
	return filepath.Join(hash[0:2], hash[2:4], hash[4:6], hash+ext)
}
