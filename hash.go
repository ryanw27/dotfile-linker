package dotlink

import (
	"bufio"
	"crypto/sha256"
	"io"
	"os"
)

// streamBufSize is the chunk size used when hashing files. Some things
// people keep in a dotfiles repo (shell history exports, old vimswap
// dumps, the occasional accidentally-committed binary) can be large, so
// we never want a "compare these two files" call to pull an entire file
// into a byte slice first.
const streamBufSize = 64 * 1024

// hashFile returns the SHA-256 digest of the file at path, reading it in
// fixed-size chunks so memory use stays constant regardless of file size.
func hashFile(path string) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte

	f, err := os.Open(path)
	if err != nil {
		return sum, err
	}
	defer f.Close()

	h := sha256.New()
	buf := make([]byte, streamBufSize)
	if _, err := io.CopyBuffer(h, bufio.NewReader(f), buf); err != nil {
		return sum, err
	}

	copy(sum[:], h.Sum(nil))
	return sum, nil
}
