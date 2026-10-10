package ai

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	multiImageWireThreshold = 4 << 20
	modelImageMaxDimension  = 1200
	modelImageJPEGQuality   = 80
)

// prepareModelImages keeps the stored originals untouched while reducing only
// the copies sent to the model when a multi-view request would otherwise be
// unusually large. Aspect ratio is preserved, so normalized observation boxes
// still map back to the original first image.
func prepareModelImages(images []Image) ([]Image, error) {
	total := 0
	for _, item := range images {
		total += len(item.Data)
	}
	if len(images) <= 6 && total <= multiImageWireThreshold {
		return images, nil
	}
	prepared := make([]Image, 0, len(images))
	for _, item := range images {
		source, _, err := image.Decode(bytes.NewReader(item.Data))
		if err != nil {
			return nil, fmt.Errorf("模型图片预处理失败: %w", err)
		}
		bounds := source.Bounds()
		width, height := bounds.Dx(), bounds.Dy()
		targetWidth, targetHeight := width, height
		if width > modelImageMaxDimension || height > modelImageMaxDimension {
			scale := float64(modelImageMaxDimension) / float64(max(width, height))
			targetWidth = max(1, int(float64(width)*scale))
			targetHeight = max(1, int(float64(height)*scale))
		}
		resized := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
		xdraw.CatmullRom.Scale(resized, resized.Bounds(), source, bounds, xdraw.Over, nil)
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, resized, &jpeg.Options{Quality: modelImageJPEGQuality}); err != nil {
			return nil, fmt.Errorf("模型图片压缩失败: %w", err)
		}
		prepared = append(prepared, Image{MIME: "image/jpeg", Data: encoded.Bytes(), Label: item.Label})
	}
	return prepared, nil
}
