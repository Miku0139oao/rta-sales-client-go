package rtasales

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"sort"
	"strings"
)

// ExportedGlyphTemplates holds packed glyph bitmaps extracted from one labeled
// captcha. Stretched templates match the main classifier; fitted templates
// match the aspect-preserving second opinion.
type ExportedGlyphTemplates struct {
	Stretched map[byte][]string
	Fitted    map[byte][]string
}

// ExportGlyphTemplates segments a labeled captcha image and returns packed,
// base64-encoded glyph templates keyed by the expected character.
func ExportGlyphTemplates(encoded []byte, answer string) (ExportedGlyphTemplates, error) {
	answer = strings.ToLower(strings.TrimSpace(answer))
	if len(encoded) == 0 {
		return ExportedGlyphTemplates{}, fmt.Errorf("captcha image is empty")
	}
	if len(answer) != defaultOCRLength {
		return ExportedGlyphTemplates{}, fmt.Errorf("answer %q must have %d characters", answer, defaultOCRLength)
	}
	decoded, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return ExportedGlyphTemplates{}, fmt.Errorf("decode captcha image: %w", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() < defaultOCRLength*8 || bounds.Dy() < 16 {
		return ExportedGlyphTemplates{}, fmt.Errorf("captcha image is too small: %dx%d", bounds.Dx(), bounds.Dy())
	}

	result := ExportedGlyphTemplates{
		Stretched: make(map[byte][]string, len(answer)),
		Fitted:    make(map[byte][]string, len(answer)),
	}
	seenStretched := make(map[string]bool)
	seenFitted := make(map[string]bool)
	for index := 0; index < len(answer); index++ {
		character := answer[index]
		components, err := extractCaptchaGlyphEnsembleComponents(decoded, index, defaultOCRLength)
		if err != nil || len(components) == 0 {
			components, err = extractCaptchaGlyphComponents(decoded, index, defaultOCRLength, 0)
			if err != nil || len(components) == 0 {
				return ExportedGlyphTemplates{}, fmt.Errorf("segment captcha character %d: %w", index+1, err)
			}
		}
		for _, component := range components {
			stretched := encodePackedGlyph(normalizeGlyph(component, templateWidth, templateHeight))
			if !seenStretched[string(character)+":"+stretched] {
				seenStretched[string(character)+":"+stretched] = true
				result.Stretched[character] = append(result.Stretched[character], stretched)
			}
			fitted := encodePackedGlyph(normalizeGlyphPreservingAspect(component, templateWidth, templateHeight))
			if !seenFitted[string(character)+":"+fitted] {
				seenFitted[string(character)+":"+fitted] = true
				result.Fitted[character] = append(result.Fitted[character], fitted)
			}
		}
	}
	return result, nil
}

func encodePackedGlyph(glyph binaryGlyph) string {
	return base64.StdEncoding.EncodeToString([]byte(PackGlyph(glyph)))
}

// PackGlyph encodes a binary glyph row-major, one bit per pixel, matching the
// layout consumed by decodedGlyphTemplates.
func PackGlyph(glyph binaryGlyph) string {
	packed := make([]byte, (glyph.width*glyph.height+7)/8)
	for position := range glyph.pixels {
		if glyph.pixels[position] {
			packed[position/8] |= 1 << uint(position%8)
		}
	}
	return string(packed)
}

// UnpackGlyph reverses PackGlyph for a templateWidth x templateHeight glyph.
func UnpackGlyph(packed string) binaryGlyph {
	glyph := newBinaryGlyph(templateWidth, templateHeight)
	data := []byte(packed)
	for position := range glyph.pixels {
		if position/8 < len(data) {
			glyph.pixels[position] = data[position/8]&(1<<uint(position%8)) != 0
		}
	}
	return glyph
}

const (
	trainedGlyphNovelty    = 0.08
	trainedGlyphCapPerChar = 48
)

// FilterNovelStretchedGlyphs keeps only new stretched shapes that are not
// already represented in the shipped template tables.
func FilterNovelStretchedGlyphs(source map[byte]map[string]bool) map[byte]map[string]bool {
	return filterNovelGlyphSet(source, learnedGlyphTemplates, supplementalGlyphTemplates)
}

// FilterNovelFittedGlyphs keeps only new fitted shapes that are not already
// represented in the shipped fitted table.
func FilterNovelFittedGlyphs(source map[byte]map[string]bool) map[byte]map[string]bool {
	return filterNovelGlyphSet(source, fittedGlyphTemplates)
}

func filterNovelGlyphSet(source map[byte]map[string]bool, existing ...map[byte][]string) map[byte]map[string]bool {
	filtered := make(map[byte]map[string]bool, len(source))
	for character, encodedSet := range source {
		kept := make(map[string]bool)
		prepared := preparedFromEncodedTables(character, existing...)
		encodedList := make([]string, 0, len(encodedSet))
		for encoded := range encodedSet {
			encodedList = append(encodedList, encoded)
		}
		sort.Strings(encodedList)
		for _, encoded := range encodedList {
			if len(kept) >= trainedGlyphCapPerChar {
				break
			}
			packed, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				continue
			}
			candidate := prepareGlyph(UnpackGlyph(string(packed)))
			if nearestPreparedDistance(candidate, prepared) < trainedGlyphNovelty {
				continue
			}
			prepared = append(prepared, candidate)
			kept[encoded] = true
		}
		filtered[character] = kept
	}
	return filtered
}

func preparedFromEncodedTables(character byte, tables ...map[byte][]string) []preparedGlyph {
	var prepared []preparedGlyph
	for _, table := range tables {
		for _, encoded := range table[character] {
			packed, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				continue
			}
			prepared = append(prepared, prepareGlyph(UnpackGlyph(string(packed))))
		}
	}
	return prepared
}

func nearestPreparedDistance(candidate preparedGlyph, existing []preparedGlyph) float64 {
	best := 1.0
	for _, template := range existing {
		distance := glyphDistance(candidate, template)
		if distance < best {
			best = distance
		}
	}
	return best
}

func mergeGlyphTable(source map[byte][]string, alphabet string, existing ...map[byte][]string) map[byte][]string {
	known := make(map[string]bool)
	for _, table := range existing {
		for character, encodedList := range table {
			for _, encoded := range encodedList {
				known[string(character)+":"+encoded] = true
			}
		}
	}
	merged := make(map[byte][]string, len(alphabet))
	for index := 0; index < len(alphabet); index++ {
		character := alphabet[index]
		seen := make(map[string]bool)
		for _, encoded := range source[character] {
			key := string(character) + ":" + encoded
			if known[key] || seen[encoded] {
				continue
			}
			seen[encoded] = true
			merged[character] = append(merged[character], encoded)
		}
	}
	return merged
}
