package ai

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestPrepareModelImagesCompressesLargeMultiViewRequest(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 1920, 1440))
	for y := 0; y < 1440; y++ {
		for x := 0; x < 1920; x++ {
			source.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: uint8((x + y) % 255), A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	images := make([]Image, 8)
	for i := range images {
		images[i] = Image{MIME: "image/jpeg", Data: encoded.Bytes(), Label: "视场"}
	}
	prepared, err := prepareModelImages(images)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 8 || prepared[0].MIME != "image/jpeg" || len(prepared[0].Data) >= len(images[0].Data) {
		t.Fatal("large multi-view request was not compressed")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(prepared[0].Data))
	if err != nil || config.Width != 1200 || config.Height != 900 {
		t.Fatalf("unexpected compressed dimensions: %dx%d err=%v", config.Width, config.Height, err)
	}
}

func TestPrepareModelImagesLeavesSmallRequestUntouched(t *testing.T) {
	images := []Image{{MIME: "image/png", Data: []byte("small"), Label: "原图"}}
	prepared, err := prepareModelImages(images)
	if err != nil {
		t.Fatal(err)
	}
	if prepared[0].MIME != images[0].MIME || string(prepared[0].Data) != string(images[0].Data) {
		t.Fatal("small request should remain unchanged")
	}
}
