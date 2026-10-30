package aiimage

import (
	"fmt"
	"math"
)

// OpenAI's GPT Image 2/2.5 calculator, verified 2026-10-29.
func imageOutputTokens(family, quality, size string) (int64, error) {
	grids := map[string]int64{"low": 16, "medium": 24, "high": 48, "xhigh": 64, "max": 96, "auto": 96}
	if family == "gpt-image-2" {
		grids = map[string]int64{"low": 16, "medium": 48, "high": 96, "auto": 96}
	}
	grid, ok := grids[quality]
	if !ok {
		return 0, fmt.Errorf("该模型不支持此质量")
	}
	// The largest allowed pixel count is attained by a 2880-square image.
	// Square maximizes both grid dimensions; this bounds the auto-size estimate.
	if size == "auto" {
		size = "2880x2880"
	}
	width, height, err := dimensions(size)
	if err != nil {
		return 0, err
	}
	longer, shorter := width, height
	if shorter > longer {
		longer, shorter = shorter, longer
	}
	// Match the official calculator's floating-point operation order as well
	// as its half-to-even branch; rational rounding differs at a few sizes.
	ratioGrid := float64(grid) / (float64(longer) / float64(shorter))
	floor := math.Floor(ratioGrid)
	shortGrid := int64(math.Round(ratioGrid))
	if ratioGrid-floor == 0.5 {
		shortGrid = int64(floor) + int64(floor)%2
	}
	return (grid*shortGrid*(2000000+width*height) + 3999999) / 4000000, nil
}
func image25OutputTokens(quality, size string) (int64, error) {
	return imageOutputTokens("gpt-image-2.5", quality, size)
}
