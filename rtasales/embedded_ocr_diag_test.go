package rtasales

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnoseLabeledCaptchas(t *testing.T) {
	if os.Getenv("OCR_DIAG") == "" {
		t.Skip("set OCR_DIAG=1 to analyze local labeled samples")
	}
	solver := NewEmbeddedOCRSolver(EmbeddedOCRConfig{})
	roots := []string{"../samples", "../samples-holdout3"}
	var correct, rejectedCorrect, rejectedWrong, wrong, total int
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Logf("skip %s: %v", root, err)
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			want := parseTestHexAnswer(entry.Name())
			if want == "" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			total++
			got, err := solver.Solve(context.Background(), data)
			if err == nil {
				if got == want {
					correct++
					continue
				}
				wrong++
				t.Logf("WRONG %s/%s got=%s", root, entry.Name(), got)
				continue
			}
			guess := unconstrainedGuess(t, solver, data)
			if guess == want {
				rejectedCorrect++
				t.Logf("REJECT-OK %s/%s err=%v", root, entry.Name(), err)
				continue
			}
			rejectedWrong++
			t.Logf("REJECT-BAD %s/%s guess=%s want=%s err=%v", root, entry.Name(), guess, want, err)
		}
	}
	if total == 0 {
		t.Fatal("no labeled samples found")
	}
	t.Logf("total=%d correct=%d wrong=%d reject-but-correct=%d reject-and-wrong=%d solve=%.1f%% recoverable=%.1f%%",
		total, correct, wrong, rejectedCorrect, rejectedWrong,
		100*float64(correct)/float64(total),
		100*float64(correct+rejectedCorrect)/float64(total))
}

func unconstrainedGuess(t *testing.T, solver *EmbeddedOCRSolver, encoded []byte) string {
	t.Helper()
	decoded, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return ""
	}
	var answer strings.Builder
	for index := 0; index < solver.length; index++ {
		character, _, _, err := solver.classifyCaptchaCharacterBaseline(decoded, index)
		if err != nil || character == 0 {
			character, _, _, err = solver.classifyCaptchaCharacter(decoded, index)
			if err != nil || character == 0 {
				answer.WriteByte('?')
				continue
			}
		}
		answer.WriteByte(character)
	}
	return answer.String()
}

func parseTestHexAnswer(name string) string {
	base := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	if index := strings.LastIndexByte(base, '-'); index >= 0 {
		ok := true
		for _, character := range base[index+1:] {
			if character < '0' || character > '9' {
				ok = false
				break
			}
		}
		if ok {
			base = base[:index]
		}
	}
	if len(base) != 5 {
		return ""
	}
	for _, character := range base {
		if !strings.ContainsRune(defaultOCRAlphabet, character) {
			return ""
		}
	}
	return base
}
