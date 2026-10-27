package mathutil

// IntMin returns the smaller of two ints (search: int-math).
func IntMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// IntMax returns the larger of two ints (search: int-math).
func IntMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// IntAbs returns the absolute value of an int (search: int-math).
func IntAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// IntSign returns -1, 0, or 1 based on sign (search: int-math).
func IntSign(x int) int {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	default:
		return 0
	}
}

// ScaleCount is a count at another scale: n x k to the nearest integer, never
// rounding a positive count to zero (a frame count at another tick rate).
func ScaleCount(n int, k float64) int {
	if n <= 0 {
		return n
	}
	return max(1, int(float64(n)*k+0.5))
}
