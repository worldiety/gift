package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"

	"github.com/worldiety/gift/asset"
)

// pictures are generated sample pictures for the image demos.
//
// They are written once into the cache directory and are plain gradients, so
// a screenshot does not depend on anything outside this repository.
var pictures []asset.Source

func samples(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	const n = 12
	pictures = make([]asset.Source, 0, n)
	for i := range n {
		path := filepath.Join(dir, "sample"+strconv.Itoa(i)+".png")
		if _, err := os.Stat(path); err != nil {
			if err := writeSample(path, i); err != nil {
				return err
			}
		}
		pictures = append(pictures, asset.File(path))
	}
	return nil
}

func writeSample(path string, i int) error {
	// Alternating aspect ratios, so that the gallery layouts have something
	// to arrange.
	w, h := 320, 240
	switch i % 3 {
	case 1:
		w, h = 240, 320
	case 2:
		w, h = 360, 200
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{
				R: uint8(40 + (x*150)/w + i*5),
				G: uint8(60 + (y*140)/h + i*4),
				B: uint8(220 - (x*90)/w - i*9),
				A: 255,
			})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
