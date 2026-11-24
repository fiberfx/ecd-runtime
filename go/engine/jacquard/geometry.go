package jacquard

import "math/bits"

// A chunk holds at most chunkBytes of row data. A table holds between minChunkRows and maxChunkRows rows
// per chunk, always a power of two, so that a row splits into chunk and slot by a shift and a mask.
const chunkBytes, minChunkRows, maxChunkRows = 16 << 10, 64, 4096

// chunkShift returns log2 of the number of rows per chunk for rows of rowB bytes: the largest power of two n
// with n*rowB <= chunkBytes, clamped to [minChunkRows, maxChunkRows]. The 64-row floor exceeds chunkBytes
// when rowB is above 256. A rowB of zero or less gives maxChunkRows.
func chunkShift(rowB int) uint8 {
	if rowB <= 0 {
		return uint8(bits.Len(maxChunkRows) - 1)
	}
	n := chunkBytes / rowB
	if n >= maxChunkRows {
		return uint8(bits.Len(maxChunkRows) - 1)
	}
	if n <= minChunkRows {
		return uint8(bits.Len(minChunkRows) - 1)
	}
	return uint8(bits.Len(uint(n)) - 1)
}
