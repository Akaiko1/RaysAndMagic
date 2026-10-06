package graphics

import (
	"bufio"
	"image"
	"os"
)

// DecodeImageFile bounds compressed-input buffering independently of image
// size. PNG decompression makes many small reads; a larger read buffer avoids
// repeated file syscalls without retaining another complete asset in memory.
func DecodeImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(bufio.NewReaderSize(f, 64<<10))
	return img, err
}
