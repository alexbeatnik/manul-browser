package runtime

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/explain"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// Things the CLI and the contracts described before the engine did them:
// `--screenshot`, `--retries`, VERIFY VISUAL, and VERIFY SOFTLY on a state.

// shotPage is a MockPage whose viewport screenshot is a real PNG the test
// controls, and which knows where its one interesting element sits in it.
type shotPage struct {
	*MockPage
	shot  []byte
	shots int
}

func (p *shotPage) Screenshot(context.Context) ([]byte, error) {
	p.shots++
	return p.shot, nil
}

// viewport renders a 200x100 white page with a 40x20 "logo" at (10,10) filled
// with fill, and a pixel of noise far from it.
func viewport(t *testing.T, fill color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	for y := 10; y < 30; y++ {
		for x := 10; x < 50; x++ {
			img.SetNRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newShotPage(t *testing.T, fill color.NRGBA) *shotPage {
	logo := regEl(1, "img", "")
	logo.AriaLabel = "Logo"
	page := &shotPage{
		MockPage: &MockPage{Elements: []dom.ElementSnapshot{logo, regEl(2, "p", "hello")}},
		shot:     viewport(t, fill),
	}
	page.EvalResult = func(expr string) ([]byte, bool) {
		if strings.Contains(expr, "viewportWidth") {
			return []byte(`{"x":10,"y":10,"width":40,"height":20,"viewportWidth":200,"dpr":1}`), true
		}
		return nil, false
	}
	return page
}

// ── VERIFY VISUAL ────────────────────────────────────────────────────────────

func TestVerifyVisual_SavesThenComparesItsBaseline(t *testing.T) {
	dir := t.TempDir()
	hunt, err := dsl.Parse(strings.NewReader("VERIFY VISUAL 'Logo'\n"))
	if err != nil {
		t.Fatal(err)
	}
	hunt.SourcePath = filepath.Join(dir, "brand.hunt")

	blue := color.NRGBA{B: 0xff, A: 0xff}
	run := func(page *shotPage) (*explain.HuntResult, error) {
		rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))
		return rt.RunHunt(context.Background(), hunt)
	}

	// First run: nothing to compare with, so what it sees becomes the baseline.
	res, err := run(newShotPage(t, blue))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	baselines, _ := filepath.Glob(filepath.Join(dir, "visual_baselines", "logo_*.png"))
	if len(baselines) != 1 {
		t.Fatalf("want one baseline beside the hunt, found %v", baselines)
	}
	if got := res.Results[0].ActionValue; !strings.HasSuffix(got, filepath.Base(baselines[0])) {
		t.Errorf("result does not name the baseline: %q", got)
	}
	// It is the element, not the page.
	f, _ := os.Open(baselines[0])
	cfg, _ := png.DecodeConfig(f)
	f.Close()
	if cfg.Width != 40 || cfg.Height != 20 {
		t.Errorf("baseline is %dx%d, want the element's 40x20", cfg.Width, cfg.Height)
	}

	// Same again: matches.
	if _, err := run(newShotPage(t, blue)); err != nil {
		t.Errorf("unchanged element: %v", err)
	}

	// The element changed colour: every pixel of it differs.
	_, err = run(newShotPage(t, color.NRGBA{R: 0xff, A: 0xff}))
	if err == nil || !strings.Contains(err.Error(), "differs from its baseline") {
		t.Errorf("changed element: want a mismatch, got %v", err)
	}

	// A change under the threshold is rendering noise, not a regression.
	almost := newShotPage(t, blue)
	img, _ := png.Decode(bytes.NewReader(almost.shot))
	nrgba := toNRGBA(img, img.Bounds())
	nrgba.SetNRGBA(12, 12, color.NRGBA{R: 1, B: 0xff, A: 0xff}) // 1 of 800 pixels
	var buf bytes.Buffer
	_ = png.Encode(&buf, nrgba)
	almost.shot = buf.Bytes()
	if _, err := run(almost); err != nil {
		t.Errorf("one stray pixel should pass: %v", err)
	}
}

func TestVerifyVisual_SizeChangeAndMissingElementFail(t *testing.T) {
	dir := t.TempDir()
	hunt, _ := dsl.Parse(strings.NewReader("VERIFY VISUAL 'Logo'\n"))
	hunt.SourcePath = filepath.Join(dir, "brand.hunt")
	blue := color.NRGBA{B: 0xff, A: 0xff}

	rt := New(regConfig(), newShotPage(t, blue), utils.NewLoggerTo(nopWriter{}, nil))
	if _, err := rt.RunHunt(context.Background(), hunt); err != nil {
		t.Fatal(err)
	}

	wider := newShotPage(t, blue)
	wider.EvalResult = func(expr string) ([]byte, bool) {
		if strings.Contains(expr, "viewportWidth") {
			return []byte(`{"x":10,"y":10,"width":60,"height":20,"viewportWidth":200,"dpr":1}`), true
		}
		return nil, false
	}
	rt = New(regConfig(), wider, utils.NewLoggerTo(nopWriter{}, nil))
	_, err := rt.RunHunt(context.Background(), hunt)
	if err == nil || !strings.Contains(err.Error(), "its baseline is 40x20") {
		t.Errorf("resized element: %v", err)
	}

	gone, _ := dsl.Parse(strings.NewReader("VERIFY VISUAL 'Zzyzx'\n"))
	gone.SourcePath = hunt.SourcePath
	rt = New(regConfig(), newShotPage(t, blue), utils.NewLoggerTo(nopWriter{}, nil))
	res, err := rt.RunHunt(context.Background(), gone)
	if err == nil || res.Results[0].FailureReason != explain.ReasonNotFound {
		t.Errorf("missing element: err=%v reason=%q", err, res.Results[0].FailureReason)
	}
	// A miss must not leave a baseline of whatever happened to rank first.
	if stray, _ := filepath.Glob(filepath.Join(dir, "visual_baselines", "zzyzx_*.png")); len(stray) != 0 {
		t.Errorf("a baseline was saved for an element that is not there: %v", stray)
	}
}

// A device-pixel-ratio of 2 doubles the screenshot, not the page: the crop has
// to follow the image.
func TestElementImage_ScalesWithTheScreenshot(t *testing.T) {
	page := newShotPage(t, color.NRGBA{B: 0xff, A: 0xff})
	page.EvalResult = func(expr string) ([]byte, bool) {
		// The page is 100 CSS pixels wide; the 200-pixel screenshot is 2x.
		return []byte(`{"x":5,"y":5,"width":20,"height":10,"viewportWidth":100,"dpr":2}`), true
	}
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))
	img, err := rt.elementImage(context.Background(), page.Elements[0])
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 40 || img.Bounds().Dy() != 20 {
		t.Fatalf("crop is %v, want 40x20", img.Bounds())
	}
	if c := img.NRGBAAt(0, 0); c.B != 0xff || c.R != 0 {
		t.Errorf("crop starts off the element: %v", c)
	}
}

// ── --screenshot ─────────────────────────────────────────────────────────────

func TestStepScreenshots_FollowTheConfiguredMode(t *testing.T) {
	const src = "PRINT 'fine'\nCLICK the 'Zzyzx' button\n"

	for mode, want := range map[string][]bool{
		"none":    {false, false},
		"":        {false, false},
		"on-fail": {false, true},
		"always":  {true, true},
	} {
		t.Run("mode="+mode, func(t *testing.T) {
			t.Chdir(t.TempDir())
			cfg := regConfig()
			cfg.Screenshot = mode
			page := newShotPage(t, color.NRGBA{A: 0xff})
			rt := New(cfg, page, utils.NewLoggerTo(nopWriter{}, nil))
			hunt, _ := dsl.Parse(strings.NewReader(src))

			res, err := rt.RunHunt(context.Background(), hunt)
			if err == nil {
				t.Fatal("the second step should fail")
			}
			for i, r := range res.Results {
				if got := r.ScreenshotPath != ""; got != want[i] {
					t.Errorf("step %d: screenshot=%v, want %v", i, got, want[i])
				}
				if r.ScreenshotPath == "" {
					continue
				}
				if strings.Contains(r.ScreenshotPath, `\`) {
					t.Errorf("path is not portable: %q", r.ScreenshotPath)
				}
				if data, rerr := os.ReadFile(r.ScreenshotPath); rerr != nil || !bytes.Equal(data, page.shot) {
					t.Errorf("screenshot file %q: %v", r.ScreenshotPath, rerr)
				}
			}
		})
	}
}

// A block fails because a step inside it did. One picture, not two.
func TestStepScreenshots_OnePerFailureInsideABlock(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := regConfig()
	cfg.Screenshot = "on-fail"
	page := newShotPage(t, color.NRGBA{A: 0xff})
	rt := New(cfg, page, utils.NewLoggerTo(nopWriter{}, nil))
	hunt, _ := dsl.Parse(strings.NewReader("REPEAT 2 TIMES:\n    CLICK the 'Zzyzx' button\n"))

	if _, err := rt.RunHunt(context.Background(), hunt); err == nil {
		t.Fatal("expected a failure")
	}
	files, _ := filepath.Glob(filepath.Join("screenshots", "*.png"))
	if len(files) != 1 || page.shots != 1 {
		t.Errorf("want 1 screenshot, found %d file(s) from %d capture(s)", len(files), page.shots)
	}
}

// ── --retries ────────────────────────────────────────────────────────────────

func TestRunWithRetries(t *testing.T) {
	fail := &explain.HuntResult{Success: false}
	boom := errors.New("step failed")

	t.Run("passes first time", func(t *testing.T) {
		res, err := RunWithRetries(context.Background(), 3, func(int) (*explain.HuntResult, error) {
			return &explain.HuntResult{Success: true}, nil
		})
		if err != nil || res.Attempts != 1 || res.Flaky {
			t.Errorf("attempts=%d flaky=%v err=%v", res.Attempts, res.Flaky, err)
		}
	})

	t.Run("fails then passes is flaky", func(t *testing.T) {
		res, err := RunWithRetries(context.Background(), 2, func(attempt int) (*explain.HuntResult, error) {
			if attempt == 1 {
				return &explain.HuntResult{}, boom
			}
			return &explain.HuntResult{Success: true}, nil
		})
		if err != nil || !res.Success || !res.Flaky || res.Attempts != 2 {
			t.Errorf("success=%v flaky=%v attempts=%d err=%v", res.Success, res.Flaky, res.Attempts, err)
		}
	})

	t.Run("gives up after the last retry", func(t *testing.T) {
		calls := 0
		res, err := RunWithRetries(context.Background(), 2, func(int) (*explain.HuntResult, error) {
			calls++
			return &explain.HuntResult{}, boom
		})
		if !errors.Is(err, boom) || calls != 3 || res.Attempts != 3 || res.Flaky {
			t.Errorf("calls=%d attempts=%d flaky=%v err=%v", calls, res.Attempts, res.Flaky, err)
		}
	})

	t.Run("no retries means one attempt", func(t *testing.T) {
		calls := 0
		_, _ = RunWithRetries(context.Background(), 0, func(int) (*explain.HuntResult, error) {
			calls++
			return fail, nil
		})
		if calls != 1 {
			t.Errorf("ran %d times", calls)
		}
	})

	t.Run("a cancelled run and a debugger stop are not retried", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		_, _ = RunWithRetries(ctx, 5, func(int) (*explain.HuntResult, error) {
			calls++
			return fail, ctx.Err()
		})
		if calls != 1 {
			t.Errorf("cancelled: ran %d times", calls)
		}

		calls = 0
		_, _ = RunWithRetries(context.Background(), 5, func(int) (*explain.HuntResult, error) {
			calls++
			return fail, ErrDebugStop
		})
		if calls != 1 {
			t.Errorf("debug stop: ran %d times", calls)
		}
	})
}

// ── VERIFY SOFTLY on a state ─────────────────────────────────────────────────

// It used to be answered as "is the word on the page", which is true of a
// button whether it is enabled or not.
func TestVerifySoftly_ChecksTheStateItNames(t *testing.T) {
	button := regEl(1, "button", "Submit")
	page := &MockPage{Elements: []dom.ElementSnapshot{button, regEl(2, "p", "hello")}}

	res, err := runSource(t, page, "VERIFY SOFTLY that 'Submit' is disabled\n")
	if err != nil {
		t.Fatalf("soft checks never fail the hunt: %v", err)
	}
	if len(res.SoftErrors) != 1 || !strings.Contains(res.SoftErrors[0], "disabled") {
		t.Errorf("an enabled button passed `is disabled`: %v", res.SoftErrors)
	}

	res, _ = runSource(t, page, "VERIFY SOFTLY that 'Submit' is enabled\nVERIFY SOFTLY that 'hello' is present\n")
	if len(res.SoftErrors) != 0 {
		t.Errorf("true statements were recorded as failures: %v", res.SoftErrors)
	}
}
