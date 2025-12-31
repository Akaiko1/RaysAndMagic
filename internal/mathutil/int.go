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
