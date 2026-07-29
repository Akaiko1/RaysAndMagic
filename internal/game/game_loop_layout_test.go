package game

import "testing"

func TestLogicalScreenSize(t *testing.T) {
	tests := []struct {
		name       string
		outW, outH int
		logicalW   int
		logicalH   int
	}{
		{"720p", 1280, 720, 1280, 720},
		{"768p", 1366, 768, 1366, 768},
		{"1080p", 1920, 1080, 1920, 1080},
		{"1440p", 2560, 1440, 1920, 1080},
		{"ultrawide-1440p", 3440, 1440, 2580, 1080},
		{"4k", 3840, 2160, 1920, 1080},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotW, gotH := logicalScreenSize(test.outW, test.outH)
			if gotW != test.logicalW || gotH != test.logicalH {
				t.Fatalf("logicalScreenSize(%d, %d) = %dx%d, want %dx%d",
					test.outW, test.outH, gotW, gotH, test.logicalW, test.logicalH)
			}
		})
	}
}
