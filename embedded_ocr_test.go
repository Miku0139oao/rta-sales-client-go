package rtasales

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestEmbeddedOCRSolverSolvesSyntheticCaptcha(t *testing.T) {
	encoded := syntheticCaptchaJPEG(t, "0be7f")
	solver := NewEmbeddedOCRSolver(EmbeddedOCRConfig{})
	answer, err := solver.Solve(context.Background(), encoded)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "0be7f" {
		t.Fatalf("answer=%q, want 0be7f", answer)
	}
}

func TestEmbeddedOCRSolverRejectsInvalidImages(t *testing.T) {
	solver := NewEmbeddedOCRSolver(EmbeddedOCRConfig{})
	if _, err := solver.Solve(context.Background(), []byte("not-an-image")); err == nil || !strings.Contains(err.Error(), "decode captcha image") {
		t.Fatalf("malformed image error=%v", err)
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	if _, err := solver.Solve(context.Background(), encoded.Bytes()); err == nil || !strings.Contains(err.Error(), "too small") {
		t.Fatalf("small image error=%v", err)
	}
}

func TestEmbeddedOCRSolverHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewEmbeddedOCRSolver(EmbeddedOCRConfig{}).Solve(ctx, []byte("not-an-image"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}
}

func TestEmbeddedOCRSolverRejectsUncertainAnswer(t *testing.T) {
	solver := NewEmbeddedOCRSolver(EmbeddedOCRConfig{
		MaximumDistance:    2,
		MinimumScoreMargin: 2,
	})
	_, err := solver.Solve(context.Background(), syntheticCaptchaJPEG(t, "0be7f"))
	if err == nil || !strings.Contains(err.Error(), "uncertain") {
		t.Fatalf("error=%v, want an uncertainty error", err)
	}
}

func syntheticCaptchaJPEG(t *testing.T, answer string) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, len(answer)*20, 30))
	for position := range canvas.Pix {
		canvas.Pix[position] = 0xff
	}
	palette := []color.RGBA{
		{R: 40, G: 105, B: 170, A: 255},
		{R: 150, G: 45, B: 75, A: 255},
		{R: 55, G: 130, B: 70, A: 255},
		{R: 105, G: 55, B: 155, A: 255},
		{R: 165, G: 105, B: 25, A: 255},
	}
	decoded := decodedLearnedTemplates(answer)
	for index := 0; index < len(answer); index++ {
		glyphs := decoded[answer[index]]
		if len(glyphs) == 0 {
			t.Fatalf("no embedded template for %q", answer[index])
		}
		glyph := glyphs[0]
		for y := 0; y < glyph.height; y++ {
			for x := 0; x < glyph.width; x++ {
				if glyph.at(x, y) {
					canvas.SetRGBA(index*20+3+x, 4+y, palette[index%len(palette)])
				}
			}
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, canvas, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}
