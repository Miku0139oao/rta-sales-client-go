package rtasales

import (
	"bytes"
	"context"
	"encoding/base64"
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

func TestEmbeddedOCRSolverReviewsConfidentColorNoiseMistake(t *testing.T) {
	encoded, err := base64.StdEncoding.DecodeString(colorNoiseCaptchaBase64)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	solver := NewEmbeddedOCRSolver(EmbeddedOCRConfig{})
	baseline, distance, margin, err := solver.classifyCaptchaCharacterBaseline(decoded, 2)
	if err != nil {
		t.Fatal(err)
	}
	if baseline != '0' || distance > solver.maximumDistance || margin < solver.minimumScoreMargin {
		t.Fatalf("baseline=%q distance=%.3f margin=%.3f, want a confident false-positive 0", baseline, distance, margin)
	}
	answer, err := solver.Solve(context.Background(), encoded)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "e2c63" {
		t.Fatalf("answer=%q, want e2c63", answer)
	}
}

// This is an expired, anonymous challenge fixture. It contains no account,
// cookie, flag, or session data.
const colorNoiseCaptchaBase64 = "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"

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
