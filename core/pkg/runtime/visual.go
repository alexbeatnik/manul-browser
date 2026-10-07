package runtime

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/explain"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
	"github.com/alexbeatnik/manul-browser/core/pkg/scorer"
)

// VERIFY VISUAL '<element>' — does the element still look the way it did.
//
// The first run has nothing to compare against, so it saves what it sees as the
// baseline and passes. Every later run compares against that file. Baselines
// are meant to be committed next to the hunt: they are the expected result.

const (
	// visualBaselineDir sits beside the .hunt file, or in the working
	// directory when the hunt has no file (stdin, a session's `run {source}`).
	visualBaselineDir = "visual_baselines"
	// visualDiffThreshold is the share of pixels allowed to differ. Rendering
	// is not bit-exact between runs — antialiasing, a blinking caret — and a
	// check that fails on one stray pixel gets deleted, not fixed.
	visualDiffThreshold = 0.01
)

func (rt *Runtime) verifyVisual(ctx context.Context, cmd dsl.Command, res *explain.ExecutionResult) error {
	target := strings.TrimSpace(rt.resolveVariables(cmd.Target))
	if target == "" {
		return fmt.Errorf("VERIFY VISUAL: missing element name in quotes")
	}
	res.TargetRequired = true
	res.TargetQuery = target

	elements, err := rt.loadSnapshot(ctx)
	if err != nil {
		return err
	}
	res.CandidatesConsidered = len(elements)

	ranked := scorer.Rank(target, cmd.TypeHint, string(dsl.ModeNone), elements, 5, nil)
	if len(ranked) == 0 || ranked[0].Explain.Score.Total < ThresholdAmbiguous ||
		!scorer.MatchesQuery(target, &ranked[0].Element) {
		res.FailureReason = explain.ReasonNotFound
		appendRankedCandidates(res, ranked, 5)
		return fmt.Errorf("target not found: %q", target)
	}
	appendRankedCandidates(res, ranked, 1)
	res.WinnerXPath = ranked[0].Element.XPath
	res.WinnerScore = ranked[0].Explain.Score.Total

	actual, err := rt.elementImage(ctx, ranked[0].Element)
	if err != nil {
		return fmt.Errorf("VERIFY VISUAL %q: %w", target, err)
	}

	step := cmd.Raw
	if step == "" {
		step = target
	}
	path := rt.visualBaselinePath(target, step)
	res.ActionValue = filepath.ToSlash(path)

	stored, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := writePNG(path, actual); err != nil {
			return fmt.Errorf("VERIFY VISUAL %q: save baseline: %w", target, err)
		}
		rt.logger.ActionDetail("📸", "VERIFY VISUAL: baseline saved → %s", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("VERIFY VISUAL %q: read baseline: %w", target, err)
	}
	decoded, err := png.Decode(bytes.NewReader(stored))
	if err != nil {
		return fmt.Errorf("VERIFY VISUAL %q: baseline %s is not a PNG: %w", target, path, err)
	}
	baseline := toNRGBA(decoded, decoded.Bounds())

	if baseline.Bounds().Size() != actual.Bounds().Size() {
		return fmt.Errorf("verification failed: '%s' is %dx%d, its baseline is %dx%d (%s)",
			target, actual.Bounds().Dx(), actual.Bounds().Dy(),
			baseline.Bounds().Dx(), baseline.Bounds().Dy(), path)
	}
	ratio := pixelDiffRatio(baseline, actual)
	if ratio > visualDiffThreshold {
		return fmt.Errorf("verification failed: '%s' differs from its baseline in %.2f%% of pixels (allowed %.2f%%; baseline %s)",
			target, ratio*100, visualDiffThreshold*100, path)
	}
	rt.logger.ActionDetail("🖼", "VERIFY VISUAL '%s': matches its baseline (%.2f%% of pixels differ)", target, ratio*100)
	return nil
}

// elementImage returns what the element looks like right now.
//
// There is no element-screenshot primitive on Page, and none is needed: the
// viewport screenshot both backends already offer is cropped to the element's
// box. The page reports that box in CSS pixels; the screenshot is in device
// pixels, so the scale between the two is read off the image itself rather
// than trusted from devicePixelRatio, which emulation can make disagree.
func (rt *Runtime) elementImage(ctx context.Context, el dom.ElementSnapshot) (*image.NRGBA, error) {
	raw, err := rt.page.EvalJS(ctx, pagejs.ElementRect(el.ID, el.XPath))
	if err != nil {
		return nil, err
	}
	var box struct {
		X, Y, Width, Height float64
		ViewportWidth       float64 `json:"viewportWidth"`
		DPR                 float64 `json:"dpr"`
	}
	if err := decodeJSONText(raw, &box); err != nil {
		return nil, fmt.Errorf("unreadable element box from the page: %w", err)
	}

	shot, err := rt.page.Screenshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	if len(shot) == 0 {
		return nil, errors.New("the browser returned an empty screenshot")
	}
	img, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		return nil, fmt.Errorf("screenshot is not a PNG: %w", err)
	}

	scale := box.DPR
	if box.ViewportWidth > 0 {
		scale = float64(img.Bounds().Dx()) / box.ViewportWidth
	}
	if scale <= 0 {
		scale = 1
	}
	px := func(v float64) int { return int(math.Round(v * scale)) }
	crop := image.Rect(px(box.X), px(box.Y), px(box.X+box.Width), px(box.Y+box.Height)).
		Add(img.Bounds().Min).Intersect(img.Bounds())
	if crop.Empty() {
		return nil, errors.New("the element has no visible area in the viewport")
	}
	return toNRGBA(img, crop), nil
}

// decodeJSONText reads JSON a page script stringified. EvalJS hands a string
// back as bare bytes, so that is normally the JSON itself; a backend that
// quoted it is unwrapped first.
func decodeJSONText(raw []byte, dst any) error {
	body := bytes.TrimSpace(raw)
	if err := json.Unmarshal(body, dst); err == nil {
		return nil
	}
	var inner string
	if err := json.Unmarshal(body, &inner); err != nil {
		return err
	}
	return json.Unmarshal([]byte(inner), dst)
}

// visualBaselinePath names the baseline for one step. The element name makes
// the file recognisable; the hash of the step text keeps two steps that name
// the same element — on different pages, say — from sharing one.
func (rt *Runtime) visualBaselinePath(target, step string) string {
	dir := "."
	if rt.sourcePath != "" {
		dir = filepath.Dir(rt.sourcePath)
	}
	sum := sha1.Sum([]byte(step))
	name := fmt.Sprintf("%s_%s.png", visualSlug(target), hex.EncodeToString(sum[:4]))
	return filepath.Join(dir, visualBaselineDir, name)
}

func visualSlug(target string) string {
	slug := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, strings.ToLower(target))
	if slug = strings.Trim(slug, "_"); slug == "" {
		return "element"
	}
	return slug
}

// toNRGBA copies region r of img into a fresh image whose origin is (0,0), so
// two crops taken at different offsets compare byte for byte.
func toNRGBA(img image.Image, r image.Rectangle) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return out
}

// pixelDiffRatio is the share of pixels that differ at all between two images
// of the same size.
func pixelDiffRatio(a, b *image.NRGBA) float64 {
	total := a.Bounds().Dx() * a.Bounds().Dy()
	if total == 0 {
		return 0
	}
	differing := 0
	for i := 0; i+3 < len(a.Pix) && i+3 < len(b.Pix); i += 4 {
		if a.Pix[i] != b.Pix[i] || a.Pix[i+1] != b.Pix[i+1] ||
			a.Pix[i+2] != b.Pix[i+2] || a.Pix[i+3] != b.Pix[i+3] {
			differing++
		}
	}
	return float64(differing) / float64(total)
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
