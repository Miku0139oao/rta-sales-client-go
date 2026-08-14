package rtasales

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"math/rand"
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

func TestEmbeddedOCRSolverUsesCalibratedConsensusDefaults(t *testing.T) {
	solver := NewEmbeddedOCRSolver(EmbeddedOCRConfig{})
	if solver.maximumDistance != 0.20 || solver.minimumScoreMargin != 0.02 {
		t.Fatalf("defaults=(%.3f, %.3f), want (0.20, 0.02)", solver.maximumDistance, solver.minimumScoreMargin)
	}
	for _, character := range []byte(defaultOCRAlphabet) {
		if len(solver.templates[character]) == 0 || len(solver.fittedTemplates[character]) == 0 {
			t.Fatalf("missing consensus templates for %q", character)
		}
	}
}

func TestSelectBestGlyphMatchUsesStrongestExtraction(t *testing.T) {
	match := selectBestGlyphMatch([]glyphMatch{
		{character: '5', distance: 0.166, margin: 0.030},
		{character: '2', distance: 0.093, margin: 0.119},
	})
	if match.character != '2' || math.Abs(match.distance-0.093) > 1e-9 || math.Abs(match.margin-0.073) > 1e-9 {
		t.Fatalf("match=%+v, want character 2 with cross-extraction margin 0.073", match)
	}
}

func TestSelectBestGlyphMatchPenalizesCloseDisagreement(t *testing.T) {
	match := selectBestGlyphMatch([]glyphMatch{
		{character: 'e', distance: 0.158, margin: 0.040},
		{character: 'a', distance: 0.162, margin: 0.080},
	})
	if match.character != 'e' || match.margin < 0.0039 || match.margin > 0.0041 {
		t.Fatalf("match=%+v, want character e with cross-extraction margin about 0.004", match)
	}
}

func TestPackedGlyphDistanceMatchesReference(t *testing.T) {
	random := rand.New(rand.NewSource(20260814))
	for sample := 0; sample < 100; sample++ {
		left := newBinaryGlyph(templateWidth, templateHeight)
		right := newBinaryGlyph(templateWidth, templateHeight)
		for position := range left.pixels {
			left.pixels[position] = random.Intn(4) == 0
			right.pixels[position] = random.Intn(4) == 0
		}
		preparedLeft := prepareGlyph(left)
		preparedRight := prepareGlyph(right)
		got := alignedGlyphOverlapDistance(preparedLeft, preparedRight)
		want := alignedGlyphOverlapDistanceReference(preparedLeft, preparedRight)
		if math.Abs(got-want) > 1e-12 {
			t.Fatalf("sample %d distance=%.12f, want %.12f", sample, got, want)
		}
	}
}

func alignedGlyphOverlapDistanceReference(left, right preparedGlyph) float64 {
	best := 1.0
	for offsetY := -2; offsetY <= 2; offsetY++ {
		for offsetX := -2; offsetX <= 2; offsetX++ {
			intersection := 0
			for _, position := range left.foreground {
				x := position%left.width + offsetX
				y := position/left.width + offsetY
				if right.at(x, y) {
					intersection++
				}
			}
			distance := 1 - 2*float64(intersection)/float64(len(left.foreground)+len(right.foreground))
			if distance < best {
				best = distance
			}
		}
	}
	return best
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
