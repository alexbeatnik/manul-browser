//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mcpClient drives `manul mcp` over its stdio, as Claude Desktop does.
type mcpClient struct {
	t     *testing.T
	cmd   *exec.Cmd
	in    interface{ Write([]byte) (int, error) }
	lines chan string
	next  int
}

func startMCP(t *testing.T, browser string) *mcpClient {
	t.Helper()
	cmd := exec.Command(binary, "mcp", "--browser", browser, "--headless")
	cmd.Dir = t.TempDir()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &mcpClient{t: t, cmd: cmd, in: stdin, lines: make(chan string, 64)}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 4<<20)
		for sc.Scan() {
			c.lines <- sc.Text()
		}
		close(c.lines)
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = cmd.Process.Kill()
			t.Error("manul mcp did not exit at end of input")
		}
	})
	return c
}

func (c *mcpClient) rpc(method string, params any) json.RawMessage {
	c.t.Helper()
	c.next++
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.next, "method": method, "params": params})
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		c.t.Fatal(err)
	}
	for {
		select {
		case line, ok := <-c.lines:
			if !ok {
				c.t.Fatal("manul mcp closed its output")
			}
			var r struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			// Anything on stdout that is not a JSON-RPC message breaks every
			// client; stdout carries the protocol and nothing else.
			if err := json.Unmarshal([]byte(line), &r); err != nil {
				c.t.Fatalf("not JSON-RPC on stdout: %q", line)
			}
			if r.ID != c.next {
				continue
			}
			if len(r.Error) > 0 {
				c.t.Fatalf("%s: %s", method, r.Error)
			}
			return r.Result
		case <-time.After(2 * time.Minute):
			c.t.Fatalf("%s: no reply", method)
		}
	}
}

func (c *mcpClient) call(name string, args map[string]any) (string, bool) {
	c.t.Helper()
	var res struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(c.rpc("tools/call", map[string]any{"name": name, "arguments": args}), &res); err != nil {
		c.t.Fatal(err)
	}
	var parts []string
	for _, p := range res.Content {
		parts = append(parts, p.Text)
	}
	return strings.Join(parts, "\n"), res.IsError
}

// TestMCP drives `manul mcp` the way an MCP client does, on both engines: the
// handshake, then the tools on the e2e site — a payment form in a cross-site
// frame, a new tab, values and text, a failed step, a whole script.
func TestMCP(t *testing.T) {
	o := &origins{}
	o.site = serve(t, o)
	o.cross = serve(t, o)
	o.xsite = strings.Replace(o.cross, "127.0.0.1", "localhost", 1)

	names, required := browsers()
	for _, browser := range names {
		t.Run(browser, func(t *testing.T) {
			c := startMCP(t, browser)
			var init struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(c.rpc("initialize", map[string]any{
				"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
				"clientInfo": map[string]any{"name": "e2e", "version": "1"},
			}), &init)
			if init.ProtocolVersion != "2025-06-18" {
				t.Errorf("protocol %q", init.ProtocolVersion)
			}
			_, _ = c.in.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))

			step := func(s string) string {
				t.Helper()
				out, isErr := c.call("step", map[string]any{"step": s})
				if isErr {
					if !required && (strings.Contains(out, "chrome not found") || strings.Contains(out, "firefox not found")) {
						t.Skipf("%s is not installed", browser)
					}
					t.Fatalf("%s:\n%s", s, out)
				}
				return out
			}

			// A payment form in a cross-site frame, seen and used.
			step("NAVIGATE to " + o.site + "/pay.html")
			if m, _ := c.call("map", map[string]any{}); !strings.Contains(m, "'Card number' [textbox — FILL]") || !strings.Contains(m, "'Pay now' [button — CLICK]") {
				t.Errorf("map does not show the frame's form:\n%s", m)
			}
			step("FILL 'Card number' field with '4242 4242'")
			step("CLICK the 'Pay now' button")
			step("VERIFY that 'paid with 4242 4242' is present")
			if v, isErr := c.call("read", map[string]any{"target": "Card number"}); isErr || v != "4242 4242" {
				t.Errorf("read 'Card number' = %q (error %v)", v, isErr)
			}

			// A failed step is the tool's answer, with labels to use instead.
			if out, isErr := c.call("step", map[string]any{"step": "CLICK the 'Pay later' button"}); !isErr || !strings.HasPrefix(out, "FAILED:") ||
				!strings.Contains(out, "The page open now is "+o.site+"/pay.html") || strings.Contains(out, "made up") {
				t.Errorf("a missing target: isError=%v\n%s", isErr, out)
			}

			// A tab a step opens is where the next step runs.
			step("NAVIGATE to " + o.site + "/tabs.html")
			step("CLICK the 'Open the product' link")
			step("VERIFY that 'Manul Plush' is present")
			step("SWITCH TO TAB 1")
			step("VERIFY that 'Shop front' is present")

			// A single-page app's search: the map after Enter showed the
			// suggestions and no results on YouTube, because the step returned
			// on the new address before the results had come back.
			step("NAVIGATE to " + o.site + "/spa.html")
			step("FILL 'Search' field with 'manul'")
			step("PRESS ENTER")
			if m, _ := c.call("map", map[string]any{}); !strings.Contains(m, "'Manul in the snow' [link — CLICK]") {
				t.Errorf("map after Enter does not show the results:\n%s", m)
			}

			// A field named only by a placeholder the page animates, as
			// Comfy's search box is: the label the map gave was gone by the
			// step, and FILL found nothing that matched it.
			step("NAVIGATE to " + o.site + "/hint.html")
			m, _ := c.call("map", map[string]any{})
			hint := ""
			for _, line := range strings.Split(m, "\n") {
				if i := strings.Index(line, "' [textbox — FILL]"); i > 0 {
					if j := strings.Index(line, "'"); j >= 0 && j < i {
						hint = line[j+1 : i]
					}
				}
			}
			if hint == "" {
				t.Fatalf("map shows no field:\n%s", m)
			}
			time.Sleep(2 * time.Second) // what a model takes to answer; the hint moves on
			step("FILL '" + hint + "' field with 'kettle'")
			step("CLICK the 'Find' button")
			step("VERIFY that 'Searching for kettle' is present")

			// Text drawn at font-size 0 is not a label: Allo's colour swatches
			// were listed as 'hex', and Comfy's rating link as '4.9 4.9 218'.
			step("NAVIGATE to " + o.site + "/swatches.html")
			m, _ = c.call("map", map[string]any{})
			if strings.Contains(m, "'hex'") || strings.Contains(m, "'gradient'") || strings.Contains(m, "4.9 4.9") ||
				!strings.Contains(m, "'4.9 218' [link — CLICK]") || !strings.Contains(m, "'One' [link — CLICK]") {
				t.Errorf("map shows text nobody sees, or hides text people do:\n%s", m)
			}
			step("CLICK the '4.9 218' link")
			step("VERIFY that 'Reviews of Manul Plush' is present")
			step("CLICK the 'One' link")
			step("VERIFY that 'Page one' is present")

			// Values and prose, on a page pages/ has a name for: the step and
			// the map say it, and NAVIGATE goes there by it.
			if err := os.MkdirAll(filepath.Join(c.cmd.Dir, "pages"), 0o755); err != nil {
				t.Fatal(err)
			}
			fragment := `{"site": "` + o.site + `/", "/extract.html": "Order details"}`
			if err := os.WriteFile(filepath.Join(c.cmd.Dir, "pages", "fixtures.json"), []byte(fragment), 0o644); err != nil {
				t.Fatal(err)
			}
			if out := step("NAVIGATE to 'Order details'"); !strings.Contains(out, "the page is now Order details ("+o.site+"/extract.html)") {
				t.Errorf("NAVIGATE by name:\n%s", out)
			}
			if m, _ := c.call("map", map[string]any{"max_per_group": 1}); !strings.Contains(m, "Order details") {
				t.Errorf("map does not name the page:\n%s", m)
			}
			step("VERIFY that the page is 'Order details'")
			if v, _ := c.call("read", map[string]any{"target": "Price"}); v != "$24.50" {
				t.Errorf("read 'Price' = %q", v)
			}
			if txt, _ := c.call("read_text", map[string]any{}); !strings.Contains(txt, "Order summary") || !strings.Contains(txt, "Shipped") {
				t.Errorf("read_text:\n%s", txt)
			}

			// A whole script.
			step("NAVIGATE to " + o.site + "/checkboxes.html")
			out, isErr := c.call("run_hunt", map[string]any{"script": "UNCHECK the checkbox for 'checkbox 2'\nVERIFY that 'checkbox 2' is NOT checked\nFILL 'Number' field with '7'"})
			if isErr || !strings.HasPrefix(out, "SUCCESS: ran 3 step(s)") {
				t.Errorf("run_hunt: isError=%v\n%s", isErr, out)
			}
		})
	}
}
