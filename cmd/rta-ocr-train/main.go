// Command rta-ocr-train improves the embedded captcha OCR by collecting
// labeled samples and regenerating glyph templates.
//
// Workflow:
//
//  1. capture   — download fresh captcha images into a sample directory.
//  2. propose   — name each unlabeled image with the current solver's answer
//     (e.g. a1b2c.jpg). Manually rename any wrong guesses; the
//     file name is the ground truth.
//  3. gen       — segment every labeled image and rewrite
//     rtasales/embedded_ocr_trained.go with the new templates.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Miku0139oao/rta-sales-client-go/rtasales"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "capture":
		err = cmdCapture(os.Args[2:])
	case "propose":
		err = cmdPropose(os.Args[2:])
	case "gen":
		err = cmdGen(os.Args[2:])
	case "eval":
		err = cmdEval(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: rta-ocr-train <command> [flags]

commands:
  capture -dir samples [-count 50] [-delay 500ms]
      download fresh RTA captchas as samples/unnamed-<n>.bin
  propose -dir samples
      copy unnamed images to <answer>.jpg using the current solver;
      review and fix wrong file names before running gen
  gen -dir samples -out rtasales/embedded_ocr_trained.go
      regenerate trained templates from <answer>.<ext> files
  eval -dir samples
      score the current solver against labeled files
`)
}

const captchaBase = "https://mansso.rta-os.com"

func newFlag() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(raw)), nil
}

func cmdCapture(arguments []string) (err error) {
	set := flag.NewFlagSet("capture", flag.ExitOnError)
	dir := set.String("dir", "samples", "sample directory")
	count := set.Int("count", 50, "number of captchas to download")
	delay := set.Duration("delay", 500*time.Millisecond, "delay between requests")
	if err := set.Parse(arguments); err != nil {
		return err
	}
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		return err
	}
	unlock, err := lockCaptureDir(*dir)
	if err != nil {
		return err
	}
	defer unlock()
	client := &http.Client{Timeout: 20 * time.Second}
	for index := 0; index < *count; index++ {
		body, err := fetchCaptcha(client)
		if err != nil {
			return fmt.Errorf("fetch captcha %d: %w", index+1, err)
		}
		name, err := createExclusiveUnnamed(*dir, body)
		if err != nil {
			return err
		}
		if (index+1)%25 == 0 || index+1 == *count {
			fmt.Printf("saved %d/%d latest=%s (%d bytes)\n", index+1, *count, name, len(body))
		}
		time.Sleep(*delay)
	}
	return nil
}

func fetchCaptcha(client *http.Client) ([]byte, error) {
	var last error
	for attempt := 1; attempt <= 5; attempt++ {
		flagValue, err := newFlag()
		if err != nil {
			return nil, err
		}
		request, err := http.NewRequest(http.MethodGet, captchaBase+"/getVerifyCodeImg?verifyCodeFlag="+url.QueryEscape(flagValue), nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36 Edg/118.0.1264.123")
		request.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
		request.Header.Set("Referer", "https://sso.rta-os.com/")
		request.Header.Set("Cache-Control", "no-cache")
		request.Header.Set("Pragma", "no-cache")
		response, err := client.Do(request)
		if err != nil {
			last = err
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		_ = response.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if response.StatusCode != http.StatusOK || len(body) == 0 {
			last = fmt.Errorf("status %d, %d bytes", response.StatusCode, len(body))
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		return body, nil
	}
	return nil, last
}

func lockCaptureDir(dir string) (func(), error) {
	path := filepath.Join(dir, ".capture.lock")
	for attempt := 0; attempt < 60; attempt++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("directory %s is already being captured by another process", dir)
}

func createExclusiveUnnamed(dir string, body []byte) (string, error) {
	for index := 1; index < 100000; index++ {
		name := filepath.Join(dir, fmt.Sprintf("unnamed-%04d.bin", index))
		file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", err
		}
		_, writeErr := file.Write(body)
		closeErr := file.Close()
		if writeErr != nil {
			_ = os.Remove(name)
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return name, nil
	}
	return "", fmt.Errorf("no free unnamed slot in %s", dir)
}

func cmdPropose(arguments []string) (err error) {
	set := flag.NewFlagSet("propose", flag.ExitOnError)
	dir := set.String("dir", "samples", "sample directory")
	if err := set.Parse(arguments); err != nil {
		return err
	}
	solver := rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})
	entries, err := os.ReadDir(*dir)
	if err != nil {
		return err
	}
	proposed, skipped := 0, 0
	for _, entry := range entries {
		name := entry.Name()
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		if entry.IsDir() || !strings.HasPrefix(base, "unnamed-") {
			continue
		}
		path := filepath.Join(*dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		answer, err := solver.Solve(context.Background(), data)
		if err != nil {
			fmt.Printf("SKIP %-16s %v\n", name, err)
			skipped++
			continue
		}
		target := uniqueLabeledPath(*dir, answer, ext)
		if err := os.Rename(path, target); err != nil {
			return err
		}
		fmt.Printf("OK   %-16s -> %s\n", name, filepath.Base(target))
		proposed++
	}
	fmt.Printf("\nproposed=%d skipped=%d\n", proposed, skipped)
	fmt.Println("Review every file name now: the file name IS the label.")
	return nil
}

func cmdGen(arguments []string) (err error) {
	set := flag.NewFlagSet("gen", flag.ExitOnError)
	dir := set.String("dir", "samples", "labeled sample directory")
	out := set.String("out", filepath.Join("rtasales", "embedded_ocr_trained.go"), "output Go file")
	if err := set.Parse(arguments); err != nil {
		return err
	}
	entries, err := os.ReadDir(*dir)
	if err != nil {
		return err
	}

	characters := []byte("0123456789abcdef")
	stretched := newGlyphSet(characters)
	fitted := newGlyphSet(characters)

	labeled := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		answer, ok := parseHexAnswer(entry.Name())
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(*dir, entry.Name()))
		if err != nil {
			return err
		}
		templates, err := rtasales.ExportGlyphTemplates(data, answer)
		if err != nil {
			fmt.Printf("WARN %-16s %v\n", entry.Name(), err)
			continue
		}
		labeled++
		addGlyphs(stretched, templates.Stretched)
		addGlyphs(fitted, templates.Fitted)
	}
	stretched = rtasales.FilterNovelStretchedGlyphs(stretched)
	fitted = rtasales.FilterNovelFittedGlyphs(fitted)
	if labeled == 0 {
		return fmt.Errorf("no labeled samples named <5-hex-answer>.<ext> found in %s", *dir)
	}

	var builder strings.Builder
	builder.WriteString("package rtasales\n\n")
	builder.WriteString("// Code generated by cmd/rta-ocr-train from labeled captcha samples; DO NOT EDIT.\n")
	builder.WriteString("// Regenerate with: go run ./cmd/rta-ocr-train gen -dir samples\n")
	if err := writeGlyphMap(&builder, "trainedGlyphTemplates", characters, stretched); err != nil {
		return err
	}
	builder.WriteByte('\n')
	if err := writeGlyphMap(&builder, "trainedFittedGlyphTemplates", characters, fitted); err != nil {
		return err
	}

	if err := os.WriteFile(*out, []byte(builder.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("processed %d labeled samples; wrote %d stretched + %d fitted templates to %s\n",
		labeled, countGlyphs(stretched), countGlyphs(fitted), *out)
	fmt.Println("next: go test ./rtasales/ && go build ./...")
	return nil
}

func cmdEval(arguments []string) error {
	set := flag.NewFlagSet("eval", flag.ExitOnError)
	dir := set.String("dir", "samples", "labeled sample directory")
	if err := set.Parse(arguments); err != nil {
		return err
	}
	entries, err := os.ReadDir(*dir)
	if err != nil {
		return err
	}
	solver := rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})
	correct, wrong, skipped := 0, 0, 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		want, ok := parseHexAnswer(entry.Name())
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(*dir, entry.Name()))
		if err != nil {
			return err
		}
		got, err := solver.Solve(context.Background(), data)
		if err != nil {
			fmt.Printf("SKIP %s want=%s err=%v\n", entry.Name(), want, err)
			skipped++
			continue
		}
		if got != want {
			fmt.Printf("WRONG %s got=%s\n", entry.Name(), got)
			wrong++
			continue
		}
		correct++
	}
	total := correct + wrong + skipped
	fmt.Printf("\ncorrect=%d wrong=%d skipped=%d total=%d\n", correct, wrong, skipped, total)
	if total > 0 {
		answered := correct + wrong
		accuracy := 0.0
		if answered > 0 {
			accuracy = 100 * float64(correct) / float64(answered)
		}
		fmt.Printf("solve-rate=%.1f%%  accuracy-when-answered=%.1f%%\n",
			100*float64(correct)/float64(total), accuracy)
	}
	return nil
}

func newGlyphSet(characters []byte) map[byte]map[string]bool {
	set := make(map[byte]map[string]bool, len(characters))
	for _, character := range characters {
		set[character] = map[string]bool{}
	}
	return set
}

func addGlyphs(dst map[byte]map[string]bool, src map[byte][]string) {
	for character, list := range src {
		if dst[character] == nil {
			dst[character] = map[string]bool{}
		}
		for _, encoded := range list {
			dst[character][encoded] = true
		}
	}
}

func countGlyphs(set map[byte]map[string]bool) int {
	total := 0
	for _, glyphs := range set {
		total += len(glyphs)
	}
	return total
}

func writeGlyphMap(builder *strings.Builder, name string, characters []byte, set map[byte]map[string]bool) error {
	fmt.Fprintf(builder, "var %s = map[byte][]string{\n", name)
	for _, character := range characters {
		encodedList := make([]string, 0, len(set[character]))
		for encoded := range set[character] {
			encodedList = append(encodedList, encoded)
		}
		sort.Strings(encodedList)
		if len(encodedList) == 0 {
			continue
		}
		fmt.Fprintf(builder, "\t'%c': {\n", character)
		for _, encoded := range encodedList {
			if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
				return fmt.Errorf("internal encoding error: %w", err)
			}
			fmt.Fprintf(builder, "\t\t%q,\n", encoded)
		}
		builder.WriteString("\t},\n")
	}
	builder.WriteString("}\n")
	return nil
}

func uniqueLabeledPath(dir, answer, ext string) string {
	candidate := filepath.Join(dir, answer+ext)
	if _, err := os.Stat(candidate); err != nil {
		return candidate
	}
	for index := 2; index < 1000; index++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", answer, index, ext))
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d%s", answer, time.Now().UnixNano(), ext))
}

func parseHexAnswer(name string) (string, bool) {
	base := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	if index := strings.LastIndexByte(base, '-'); index >= 0 {
		suffix := base[index+1:]
		if suffix != "" && isDigits(suffix) {
			base = base[:index]
		}
	}
	if len(base) != 5 || !isHex(base) {
		return "", false
	}
	return base, true
}

func isDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func isHex(value string) bool {
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}
