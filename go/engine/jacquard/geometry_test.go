package jacquard

import "testing"

// refShift is the reference for chunkShift: the largest power of two n with n*rowB <= chunkBytes, clamped.
func refShift(rowB int) uint8 {
	n, shift := 1, uint8(0)
	for n*2*rowB <= chunkBytes && n*2 <= maxChunkRows {
		n *= 2
		shift++
	}
	for n < minChunkRows {
		n *= 2
		shift++
	}
	return shift
}

// TestChunkShift checks the rows per chunk for row widths (J13): a power of two from 64 to 4096, the largest
// that fits chunkBytes, and 64 above 256 bytes per row.
func TestChunkShift(t *testing.T) {
	for rowB := 1; rowB <= 4096; rowB++ {
		shift := chunkShift(rowB)
		rows := 1 << shift
		if rows < minChunkRows || rows > maxChunkRows {
			t.Fatalf("rowB %d: %d rows per chunk is outside %d..%d", rowB, rows, minChunkRows, maxChunkRows)
		}
		if want := refShift(rowB); shift != want {
			t.Fatalf("rowB %d: shift %d, want %d", rowB, shift, want)
		}
	}
	for rowB := 20; rowB <= 256; rowB++ {
		rows := 1 << chunkShift(rowB)
		if rows*rowB > chunkBytes {
			t.Fatalf("rowB %d: %d rows need %d bytes, more than %d", rowB, rows, rows*rowB, chunkBytes)
		}
		if rows < maxChunkRows && rows*2*rowB <= chunkBytes {
			t.Fatalf("rowB %d: %d rows is not the largest power of two that fits", rowB, rows)
		}
	}
	for _, rowB := range []int{257, 300, 1000, 1 << 20} {
		if got := chunkShift(rowB); got != 6 {
			t.Fatalf("rowB %d: shift %d, want 6", rowB, got)
		}
	}
	for _, rowB := range []int{0, -1} {
		if got := chunkShift(rowB); got != 12 {
			t.Fatalf("rowB %d: shift %d, want 12", rowB, got)
		}
	}
	known := map[int]uint8{20: 9, 100: 7, 128: 7, 256: 6}
	for rowB, want := range known {
		if got := chunkShift(rowB); got != want {
			t.Fatalf("rowB %d: shift %d, want %d", rowB, got, want)
		}
	}
}
