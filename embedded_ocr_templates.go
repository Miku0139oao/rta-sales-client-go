package rtasales

import "encoding/base64"

func decodedLearnedTemplates(alphabet string) map[byte][]binaryGlyph {
	return decodedGlyphTemplates(learnedGlyphTemplates, alphabet)
}

func decodedGlyphTemplates(source map[byte][]string, alphabet string) map[byte][]binaryGlyph {
	result := make(map[byte][]binaryGlyph, len(alphabet))
	for index := 0; index < len(alphabet); index++ {
		character := alphabet[index]
		for _, encoded := range source[character] {
			packed, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				continue
			}
			glyph := newBinaryGlyph(templateWidth, templateHeight)
			for position := range glyph.pixels {
				if position/8 < len(packed) {
					glyph.pixels[position] = packed[position/8]&(1<<uint(position%8)) != 0
				}
			}
			result[character] = append(result[character], glyph)
		}
	}
	return result
}
