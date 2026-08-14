package rtasales

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"math/bits"
	"sort"
	"strings"
	"sync"
)

const (
	defaultOCRAlphabet = "0123456789abcdef"
	defaultOCRLength   = 5
	templateWidth      = 15
	templateHeight     = 21
)

// EmbeddedOCRConfig configures the dependency-free, CPU-only captcha solver.
// The zero value is tuned for RTA's five-character hexadecimal captcha.
type EmbeddedOCRConfig struct {
	// Length is the fixed number of glyph cells. Non-positive values use five.
	Length int
	// Alphabet limits recognition to supported ASCII characters. Empty uses
	// hexadecimal digits.
	Alphabet string
	// MaximumDistance rejects a best match above this value. Non-positive
	// values use the calibrated default; smaller values are stricter.
	MaximumDistance float64
	// MinimumScoreMargin rejects a match that is too close to the runner-up.
	// Non-positive values use the calibrated default; larger values are stricter.
	MinimumScoreMargin float64
}

// EmbeddedOCRSolver performs fixed-cell segmentation and compares cleaned
// glyphs with normalized examples compiled into the package.
// It does not require Tesseract, CGO, a model file, or a GPU.
type EmbeddedOCRSolver struct {
	length             int
	alphabet           string
	maximumDistance    float64
	minimumScoreMargin float64
	templates          map[byte][]preparedGlyph
	fittedTemplates    map[byte][]preparedGlyph
}

// NewEmbeddedOCRSolver creates the built-in RTA captcha solver.
func NewEmbeddedOCRSolver(config EmbeddedOCRConfig) *EmbeddedOCRSolver {
	length := config.Length
	if length <= 0 {
		length = defaultOCRLength
	}
	alphabet := uniqueASCII(strings.ToLower(strings.TrimSpace(config.Alphabet)))
	if alphabet == "" {
		alphabet = defaultOCRAlphabet
	}
	maximumDistance := config.MaximumDistance
	if maximumDistance <= 0 {
		maximumDistance = 0.20
	}
	minimumMargin := config.MinimumScoreMargin
	if minimumMargin <= 0 {
		minimumMargin = 0.02
	}
	templates, fittedTemplates := preparedLearnedTemplates(alphabet)
	return &EmbeddedOCRSolver{
		length:             length,
		alphabet:           alphabet,
		maximumDistance:    maximumDistance,
		minimumScoreMargin: minimumMargin,
		templates:          templates,
		fittedTemplates:    fittedTemplates,
	}
}

var (
	learnedTemplatesOnce   sync.Once
	learnedTemplates       map[byte][]preparedGlyph
	learnedFittedTemplates map[byte][]preparedGlyph
)

func preparedLearnedTemplates(alphabet string) (map[byte][]preparedGlyph, map[byte][]preparedGlyph) {
	learnedTemplatesOnce.Do(func() {
		stretched := decodedLearnedTemplates(defaultOCRAlphabet)
		supplemental := decodedGlyphTemplates(supplementalGlyphTemplates, defaultOCRAlphabet)
		for _, character := range []byte(defaultOCRAlphabet) {
			stretched[character] = append(stretched[character], supplemental[character]...)
		}
		learnedTemplates = prepareGlyphTemplates(stretched)
		learnedFittedTemplates = prepareGlyphTemplates(decodedGlyphTemplates(fittedGlyphTemplates, defaultOCRAlphabet))
	})
	selected := make(map[byte][]preparedGlyph, len(alphabet))
	selectedFitted := make(map[byte][]preparedGlyph, len(alphabet))
	for index := 0; index < len(alphabet); index++ {
		character := alphabet[index]
		selected[character] = learnedTemplates[character]
		selectedFitted[character] = learnedFittedTemplates[character]
	}
	return selected, selectedFitted
}

func (s *EmbeddedOCRSolver) Solve(ctx context.Context, encoded []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(encoded) == 0 {
		return "", errors.New("captcha image is empty")
	}
	decoded, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("decode captcha image: %w", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() < s.length*8 || bounds.Dy() < 16 {
		return "", fmt.Errorf("captcha image is too small: %dx%d", bounds.Dx(), bounds.Dy())
	}

	var answer strings.Builder
	answer.Grow(s.length)
	for index := 0; index < s.length; index++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		match, best, margin, err := s.classifyCaptchaCharacter(decoded, index)
		if err != nil {
			return "", fmt.Errorf("segment captcha character %d: %w", index+1, err)
		}
		if best > s.maximumDistance || margin < s.minimumScoreMargin {
			return "", fmt.Errorf(
				"captcha character %d is uncertain (distance %.3f, margin %.3f)",
				index+1,
				best,
				margin,
			)
		}
		answer.WriteByte(match)
	}
	return answer.String(), nil
}

type glyphMatch struct {
	character byte
	distance  float64
	margin    float64
}

func (s *EmbeddedOCRSolver) classifyCaptchaCharacter(source image.Image, index int) (byte, float64, float64, error) {
	components, err := extractCaptchaGlyphComponents(source, index, s.length, 0)
	if err != nil {
		return 0, math.Inf(1), 0, err
	}
	stretchedMatches := make([]glyphMatch, 0, len(components))
	fittedMatches := make([]glyphMatch, 0, len(components))
	for _, component := range components {
		stretched := normalizeGlyph(component, templateWidth, templateHeight)
		character, distance, margin := s.classify(stretched)
		stretchedMatches = append(stretchedMatches, glyphMatch{character: character, distance: distance, margin: margin})

		fitted := normalizeGlyphPreservingAspect(component, templateWidth, templateHeight)
		character, distance, margin = s.classifyWithTemplates(fitted, s.fittedTemplates)
		fittedMatches = append(fittedMatches, glyphMatch{character: character, distance: distance, margin: margin})
	}
	stretched := selectBestGlyphMatch(stretchedMatches)
	fitted := selectBestGlyphMatch(fittedMatches)
	if stretched.character != fitted.character {
		// The two models deliberately preserve different shape information. A
		// disagreement is safer to retry with a fresh challenge than to guess.
		return stretched.character, maxFloat(stretched.distance, fitted.distance), -1, nil
	}
	return stretched.character,
		maxFloat(stretched.distance, fitted.distance),
		minFloat(stretched.margin, fitted.margin),
		nil
}

func selectBestGlyphMatch(matches []glyphMatch) glyphMatch {
	if len(matches) == 0 {
		return glyphMatch{distance: math.Inf(1)}
	}
	sort.Slice(matches, func(left, right int) bool {
		if matches[left].distance == matches[right].distance {
			return matches[left].margin > matches[right].margin
		}
		return matches[left].distance < matches[right].distance
	})
	best := matches[0]
	// A second extraction method that prefers another character is evidence
	// against the winner. A close disagreement must be rejected even when the
	// winner's own template margin looks healthy.
	for _, alternative := range matches[1:] {
		if alternative.character == best.character {
			continue
		}
		best.margin = minFloat(best.margin, alternative.distance-best.distance)
	}
	return best
}

func (s *EmbeddedOCRSolver) classify(glyph binaryGlyph) (byte, float64, float64) {
	return s.classifyWithTemplates(glyph, s.templates)
}

func (s *EmbeddedOCRSolver) classifyWithTemplates(glyph binaryGlyph, templates map[byte][]preparedGlyph) (byte, float64, float64) {
	type candidate struct {
		character byte
		distance  float64
	}
	candidates := make([]candidate, 0, len(s.alphabet))
	prepared := prepareGlyph(glyph)
	for index := 0; index < len(s.alphabet); index++ {
		character := s.alphabet[index]
		nearest := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
		for _, template := range templates[character] {
			distance := glyphDistance(prepared, template)
			for neighbor := range nearest {
				if distance >= nearest[neighbor] {
					continue
				}
				copy(nearest[neighbor+1:], nearest[neighbor:len(nearest)-1])
				nearest[neighbor] = distance
				break
			}
		}
		best := 0.0
		count := 0
		for _, distance := range nearest {
			if !math.IsInf(distance, 1) {
				best += distance
				count++
			}
		}
		if count == 0 {
			best = math.Inf(1)
		} else {
			best /= float64(count)
		}
		candidates = append(candidates, candidate{character: character, distance: best})
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].distance == candidates[right].distance {
			return candidates[left].character < candidates[right].character
		}
		return candidates[left].distance < candidates[right].distance
	})
	if len(candidates) == 0 {
		return 0, math.Inf(1), 0
	}
	margin := math.Inf(1)
	if len(candidates) > 1 {
		margin = candidates[1].distance - candidates[0].distance
	}
	return candidates[0].character, candidates[0].distance, margin
}

type binaryGlyph struct {
	width  int
	height int
	pixels []bool
}

type preparedGlyph struct {
	binaryGlyph
	foreground []int
	rows       []uint64
	holes      int
}

func newBinaryGlyph(width, height int) binaryGlyph {
	return binaryGlyph{width: width, height: height, pixels: make([]bool, width*height)}
}

func (glyph binaryGlyph) at(x, y int) bool {
	return x >= 0 && x < glyph.width && y >= 0 && y < glyph.height && glyph.pixels[y*glyph.width+x]
}

func (glyph binaryGlyph) set(x, y int, value bool) {
	if x >= 0 && x < glyph.width && y >= 0 && y < glyph.height {
		glyph.pixels[y*glyph.width+x] = value
	}
}

func extractCaptchaGlyphVariants(source image.Image, index, length int) ([]binaryGlyph, error) {
	components, err := extractCaptchaGlyphComponents(source, index, length, 0)
	if err != nil {
		return nil, err
	}
	variants := make([]binaryGlyph, 0, len(components))
	for _, component := range components {
		variant := normalizeGlyph(component, templateWidth, templateHeight)
		duplicate := false
		for _, existing := range variants {
			if equalBinaryGlyph(existing, variant) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			variants = append(variants, variant)
		}
	}
	return variants, nil
}

func extractCaptchaGlyphComponents(source image.Image, index, length, horizontalInset int) ([]binaryGlyph, error) {
	bounds := source.Bounds()
	left := bounds.Min.X + int(math.Round(float64(index*bounds.Dx())/float64(length))) + horizontalInset
	right := bounds.Min.X + int(math.Round(float64((index+1)*bounds.Dx())/float64(length))) - horizontalInset
	top := bounds.Min.Y + 3
	bottom := bounds.Max.Y - 3
	if right-left < 4 || bottom-top < 8 {
		return nil, errors.New("invalid captcha cell bounds")
	}
	cell := image.Rect(left, top, right, bottom)
	components := make([]binaryGlyph, 0, 2)
	if component, ok := dominantColorComponent(source, cell); ok && component.foregroundCount() >= 18 {
		components = append(components, component)
	}
	if component, err := grayscaleCaptchaComponent(source, cell); err == nil {
		components = append(components, component)
	}
	if len(components) == 0 {
		return nil, errors.New("no usable glyph component")
	}
	return components, nil
}

func grayscaleCaptchaComponent(source image.Image, bounds image.Rectangle) (binaryGlyph, error) {
	width := bounds.Dx()
	height := bounds.Dy()
	gray := make([]uint8, width*height)
	histogram := [256]int{}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			red, green, blue, _ := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			value := uint8((299*(red>>8) + 587*(green>>8) + 114*(blue>>8)) / 1000)
			gray[y*width+x] = value
			histogram[value]++
		}
	}
	threshold := otsuThreshold(histogram, width*height)
	if threshold < 105 {
		threshold = 105
	}
	if threshold > 185 {
		threshold = 185
	}
	raw := newBinaryGlyph(width, height)
	for position, value := range gray {
		raw.pixels[position] = int(value) <= threshold
	}
	cleaned := removeThinLines(raw)
	component, ok := largestComponent(cleaned)
	if !ok || component.foregroundCount() < 18 {
		return binaryGlyph{}, errors.New("no usable glyph component")
	}
	return component, nil
}

func equalBinaryGlyph(left, right binaryGlyph) bool {
	if left.width != right.width || left.height != right.height || len(left.pixels) != len(right.pixels) {
		return false
	}
	for position := range left.pixels {
		if left.pixels[position] != right.pixels[position] {
			return false
		}
	}
	return true
}

type captchaColorCluster struct {
	red    float64
	green  float64
	blue   float64
	weight float64
	count  int
}

// dominantColorComponent separates the thick, consistently colored glyph
// from RTA's thin multi-colored interference lines. JPEG anti-aliasing blends
// a glyph color toward white, so clustering uses the direction from white
// instead of comparing raw RGB values.
func dominantColorComponent(source image.Image, bounds image.Rectangle) (binaryGlyph, bool) {
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return binaryGlyph{}, false
	}
	type colorKey struct {
		red   uint8
		green uint8
	}
	clusters := make(map[colorKey]*captchaColorCluster)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			red, green, blue := colorDeficit(source.At(bounds.Min.X+x, bounds.Min.Y+y))
			total := red + green + blue
			strength := maxFloat(red, maxFloat(green, blue))
			if strength < 24 || total <= 0 {
				continue
			}
			normalizedRed := red / total
			normalizedGreen := green / total
			key := colorKey{
				red:   uint8(math.Round(normalizedRed * 12)),
				green: uint8(math.Round(normalizedGreen * 12)),
			}
			cluster := clusters[key]
			if cluster == nil {
				cluster = &captchaColorCluster{}
				clusters[key] = cluster
			}
			weight := minFloat(strength, 160) / 160
			cluster.red += normalizedRed * weight
			cluster.green += normalizedGreen * weight
			cluster.blue += blue / total * weight
			cluster.weight += weight
			cluster.count++
		}
	}
	candidates := make([]captchaColorCluster, 0, len(clusters))
	for _, cluster := range clusters {
		if cluster.count < 4 || cluster.weight == 0 {
			continue
		}
		cluster.red /= cluster.weight
		cluster.green /= cluster.weight
		cluster.blue /= cluster.weight
		candidates = append(candidates, *cluster)
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].weight == candidates[right].weight {
			if candidates[left].red == candidates[right].red {
				return candidates[left].green < candidates[right].green
			}
			return candidates[left].red < candidates[right].red
		}
		return candidates[left].weight > candidates[right].weight
	})
	if len(candidates) > 10 {
		candidates = candidates[:10]
	}

	bestScore := -1.0
	var best binaryGlyph
	for _, candidate := range candidates {
		mask := newBinaryGlyph(width, height)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				red, green, blue := colorDeficit(source.At(bounds.Min.X+x, bounds.Min.Y+y))
				total := red + green + blue
				strength := maxFloat(red, maxFloat(green, blue))
				if strength < 20 || total <= 0 {
					continue
				}
				red /= total
				green /= total
				blue /= total
				difference := math.Abs(red-candidate.red) + math.Abs(green-candidate.green) + math.Abs(blue-candidate.blue)
				if difference <= 0.11 {
					mask.set(x, y, true)
				}
			}
		}
		cleaned := removeThinLines(mask)
		component, ok := largestComponent(cleaned)
		if !ok {
			continue
		}
		count := component.foregroundCount()
		if count < 12 || component.height < 10 {
			continue
		}
		// Thick, character-height components win over short intersections of
		// background lines. Very wide components are usually merged noise.
		aspectPenalty := 0.0
		if component.width > int(math.Ceil(float64(width)*0.9)) {
			aspectPenalty = float64(component.width-width/2) * 2
		}
		score := float64(count) + float64(component.height)*3 - aspectPenalty
		if score > bestScore {
			bestScore = score
			best = component
		}
	}
	return best, bestScore >= 0
}

func colorDeficit(color interface {
	RGBA() (uint32, uint32, uint32, uint32)
}) (float64, float64, float64) {
	red, green, blue, _ := color.RGBA()
	return 255 - float64(red>>8), 255 - float64(green>>8), 255 - float64(blue>>8)
}

func otsuThreshold(histogram [256]int, total int) int {
	if total <= 0 {
		return 150
	}
	sum := 0.0
	for value, count := range histogram {
		sum += float64(value * count)
	}
	backgroundWeight := 0
	backgroundSum := 0.0
	bestVariance := -1.0
	bestThreshold := 150
	for threshold := 0; threshold < 256; threshold++ {
		backgroundWeight += histogram[threshold]
		if backgroundWeight == 0 {
			continue
		}
		foregroundWeight := total - backgroundWeight
		if foregroundWeight == 0 {
			break
		}
		backgroundSum += float64(threshold * histogram[threshold])
		backgroundMean := backgroundSum / float64(backgroundWeight)
		foregroundMean := (sum - backgroundSum) / float64(foregroundWeight)
		difference := backgroundMean - foregroundMean
		variance := float64(backgroundWeight*foregroundWeight) * difference * difference
		if variance > bestVariance {
			bestVariance = variance
			bestThreshold = threshold
		}
	}
	return bestThreshold
}

func removeThinLines(source binaryGlyph) binaryGlyph {
	core := newBinaryGlyph(source.width, source.height)
	for y := 0; y < source.height; y++ {
		for x := 0; x < source.width; x++ {
			if !source.at(x, y) {
				continue
			}
			for top := y - 1; top <= y; top++ {
				for left := x - 1; left <= x; left++ {
					if source.at(left, top) && source.at(left+1, top) &&
						source.at(left, top+1) && source.at(left+1, top+1) {
						core.set(x, y, true)
					}
				}
			}
		}
	}
	result := newBinaryGlyph(source.width, source.height)
	for y := 0; y < source.height; y++ {
		for x := 0; x < source.width; x++ {
			if !core.at(x, y) {
				continue
			}
			for offsetY := -1; offsetY <= 1; offsetY++ {
				for offsetX := -1; offsetX <= 1; offsetX++ {
					if source.at(x+offsetX, y+offsetY) {
						result.set(x+offsetX, y+offsetY, true)
					}
				}
			}
		}
	}
	return result
}

func largestComponent(source binaryGlyph) (binaryGlyph, bool) {
	visited := make([]bool, len(source.pixels))
	best := make([]int, 0)
	for start, foreground := range source.pixels {
		if !foreground || visited[start] {
			continue
		}
		queue := []int{start}
		visited[start] = true
		component := make([]int, 0, 64)
		for len(queue) > 0 {
			position := queue[0]
			queue = queue[1:]
			component = append(component, position)
			x := position % source.width
			y := position / source.width
			for offsetY := -1; offsetY <= 1; offsetY++ {
				for offsetX := -1; offsetX <= 1; offsetX++ {
					if offsetX == 0 && offsetY == 0 {
						continue
					}
					nextX, nextY := x+offsetX, y+offsetY
					if nextX < 0 || nextX >= source.width || nextY < 0 || nextY >= source.height {
						continue
					}
					next := nextY*source.width + nextX
					if source.pixels[next] && !visited[next] {
						visited[next] = true
						queue = append(queue, next)
					}
				}
			}
		}
		if len(component) > len(best) {
			best = component
		}
	}
	if len(best) == 0 {
		return binaryGlyph{}, false
	}
	left, right := source.width, -1
	top, bottom := source.height, -1
	for _, position := range best {
		x := position % source.width
		y := position / source.width
		left = min(left, x)
		right = max(right, x)
		top = min(top, y)
		bottom = max(bottom, y)
	}
	result := newBinaryGlyph(right-left+1, bottom-top+1)
	for _, position := range best {
		x := position%source.width - left
		y := position/source.width - top
		result.set(x, y, true)
	}
	return result, true
}

func normalizeGlyph(source binaryGlyph, width, height int) binaryGlyph {
	result := newBinaryGlyph(width, height)
	if source.width == 0 || source.height == 0 {
		return result
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sourceLeft := float64(x*source.width) / float64(width)
			sourceRight := float64((x+1)*source.width) / float64(width)
			sourceTop := float64(y*source.height) / float64(height)
			sourceBottom := float64((y+1)*source.height) / float64(height)
			covered := 0.0
			area := (sourceRight - sourceLeft) * (sourceBottom - sourceTop)
			for sourceY := int(math.Floor(sourceTop)); sourceY < int(math.Ceil(sourceBottom)); sourceY++ {
				for sourceX := int(math.Floor(sourceLeft)); sourceX < int(math.Ceil(sourceRight)); sourceX++ {
					if !source.at(sourceX, sourceY) {
						continue
					}
					overlapX := maxFloat(0, minFloat(sourceRight, float64(sourceX+1))-maxFloat(sourceLeft, float64(sourceX)))
					overlapY := maxFloat(0, minFloat(sourceBottom, float64(sourceY+1))-maxFloat(sourceTop, float64(sourceY)))
					covered += overlapX * overlapY
				}
			}
			result.set(x, y, area > 0 && covered/area >= 0.28)
		}
	}
	return result
}

func normalizeGlyphPreservingAspect(source binaryGlyph, width, height int) binaryGlyph {
	result := newBinaryGlyph(width, height)
	if source.width <= 0 || source.height <= 0 || width <= 0 || height <= 0 {
		return result
	}
	scale := minFloat(float64(width)/float64(source.width), float64(height)/float64(source.height))
	fittedWidth := min(width, max(1, int(math.Round(float64(source.width)*scale))))
	fittedHeight := min(height, max(1, int(math.Round(float64(source.height)*scale))))
	fitted := normalizeGlyph(source, fittedWidth, fittedHeight)
	offsetX := (width - fittedWidth) / 2
	offsetY := (height - fittedHeight) / 2
	for y := 0; y < fittedHeight; y++ {
		for x := 0; x < fittedWidth; x++ {
			if fitted.at(x, y) {
				result.set(offsetX+x, offsetY+y, true)
			}
		}
	}
	return result
}

func (glyph binaryGlyph) foregroundCount() int {
	count := 0
	for _, foreground := range glyph.pixels {
		if foreground {
			count++
		}
	}
	return count
}

func prepareGlyphTemplates(source map[byte][]binaryGlyph) map[byte][]preparedGlyph {
	result := make(map[byte][]preparedGlyph, len(source))
	for character, glyphs := range source {
		prepared := make([]preparedGlyph, 0, len(glyphs))
		for _, glyph := range glyphs {
			prepared = append(prepared, prepareGlyph(glyph))
		}
		result[character] = prepared
	}
	return result
}

func prepareGlyph(glyph binaryGlyph) preparedGlyph {
	prepared := preparedGlyph{
		binaryGlyph: glyph,
		holes:       countGlyphHoles(glyph),
	}
	if glyph.width <= 64 {
		prepared.rows = make([]uint64, glyph.height)
	}
	for position, foreground := range glyph.pixels {
		if foreground {
			prepared.foreground = append(prepared.foreground, position)
			if prepared.rows != nil {
				prepared.rows[position/glyph.width] |= uint64(1) << uint(position%glyph.width)
			}
		}
	}
	return prepared
}

func glyphDistance(left, right preparedGlyph) float64 {
	if left.width != right.width || left.height != right.height {
		return math.Inf(1)
	}
	leftCount := len(left.foreground)
	rightCount := len(right.foreground)
	if leftCount == 0 || rightCount == 0 {
		return math.Inf(1)
	}
	densityPenalty := math.Abs(float64(leftCount-rightCount)) / float64(max(leftCount, rightCount))
	holePenalty := math.Abs(float64(left.holes-right.holes)) * 0.02
	overlapPenalty := alignedGlyphOverlapDistance(left, right)
	return overlapPenalty + densityPenalty*0.05 + holePenalty
}

func alignedGlyphOverlapDistance(left, right preparedGlyph) float64 {
	best := 1.0
	for offsetY := -2; offsetY <= 2; offsetY++ {
		for offsetX := -2; offsetX <= 2; offsetX++ {
			intersection := 0
			if left.rows != nil && right.rows != nil {
				for leftY, row := range left.rows {
					rightY := leftY + offsetY
					if rightY < 0 || rightY >= len(right.rows) {
						continue
					}
					if offsetX < 0 {
						row >>= uint(-offsetX)
					} else {
						row <<= uint(offsetX)
					}
					intersection += bits.OnesCount64(row & right.rows[rightY])
				}
			} else {
				for _, position := range left.foreground {
					x := position%left.width + offsetX
					y := position/left.width + offsetY
					if right.at(x, y) {
						intersection++
					}
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

func countGlyphHoles(glyph binaryGlyph) int {
	visited := make([]bool, len(glyph.pixels))
	holes := 0
	for start, foreground := range glyph.pixels {
		if foreground || visited[start] {
			continue
		}
		queue := []int{start}
		visited[start] = true
		touchesEdge := false
		for len(queue) > 0 {
			position := queue[0]
			queue = queue[1:]
			x := position % glyph.width
			y := position / glyph.width
			if x == 0 || x == glyph.width-1 || y == 0 || y == glyph.height-1 {
				touchesEdge = true
			}
			for _, step := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nextX, nextY := x+step[0], y+step[1]
				if nextX < 0 || nextX >= glyph.width || nextY < 0 || nextY >= glyph.height {
					continue
				}
				next := nextY*glyph.width + nextX
				if !glyph.pixels[next] && !visited[next] {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		if !touchesEdge {
			holes++
		}
	}
	return holes
}

func uniqueASCII(value string) string {
	seen := [256]bool{}
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character > 127 || seen[character] {
			continue
		}
		if len(learnedGlyphTemplates[character]) == 0 {
			continue
		}
		seen[character] = true
		result.WriteByte(character)
	}
	return result.String()
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}
