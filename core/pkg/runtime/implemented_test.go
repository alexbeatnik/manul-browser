package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
)

// Every verb the DSL declares must actually execute.
//
// This guard exists because WAIT FOR shipped declared-but-unimplemented: it
// parsed, it appeared in the contract, and it failed at runtime with "not yet
// implemented" the first time anyone used it. Declaring a verb and forgetting
// its runtime case is a silent mistake until a user hits it, so the test walks
// every CommandType and refuses that particular error.
//
// Other errors are fine and expected here — the mock page resolves nothing, so
// most verbs fail for want of an element. Only "not yet implemented" means the
// verb was never wired up at all.
func TestEveryDeclaredVerbIsImplemented(t *testing.T) {
	// Structural verbs are consumed by the parser's block handling or by
	// Hunt.Expand() and never reach executeCommand.
	structural := map[dsl.CommandType]bool{
		dsl.CmdElIf:      true,
		dsl.CmdElse:      true,
		dsl.CmdEndIf:     true,
		dsl.CmdEndFor:    true,
		dsl.CmdEndWhile:  true,
		dsl.CmdEndRepeat: true,
		dsl.CmdCallStep:  true,
		dsl.CmdUse:       true,
		dsl.CmdUnknown:   true,
	}

	all := []dsl.CommandType{
		dsl.CmdNavigate, dsl.CmdOpenApp, dsl.CmdClick, dsl.CmdDoubleClick,
		dsl.CmdRightClick, dsl.CmdFill, dsl.CmdType, dsl.CmdSelect,
		dsl.CmdCheck, dsl.CmdUncheck, dsl.CmdVerify, dsl.CmdVerifySoft,
		dsl.CmdVerifyField, dsl.CmdExtract, dsl.CmdScroll, dsl.CmdPress,
		dsl.CmdWait, dsl.CmdWaitFor, dsl.CmdWaitForResponse, dsl.CmdWaitForSelector,
		dsl.CmdFullScan, dsl.CmdScanPage, dsl.CmdHover, dsl.CmdDrag,
		dsl.CmdSet, dsl.CmdPrint, dsl.CmdScreenshot, dsl.CmdHighlight,
		dsl.CmdRepeat, dsl.CmdForEach, dsl.CmdWhile, dsl.CmdIf,
		dsl.CmdCallGo, dsl.CmdUploadFile, dsl.CmdPause, dsl.CmdDebugVars,
		dsl.CmdMock,
	}

	for _, typ := range all {
		if structural[typ] {
			continue
		}
		t.Run(string(typ), func(t *testing.T) {
			rt := newTestRuntime(config.Default())
			// A short deadline keeps the polling verbs (WAIT FOR, WAIT FOR
			// SELECTOR) from spending their full timeout here.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err := rt.executeCommand(ctx, dsl.Command{
				Type:   typ,
				Raw:    string(typ),
				Target: "Something",
			})
			if err != nil && strings.Contains(err.Error(), "not yet implemented") {
				t.Errorf("%s is declared in the DSL but has no runtime case: %v", typ, err)
			}
		})
	}
}

// The guard above is only worth having if it can fail, so this pins the signal
// it watches for: a type with no case really does produce that error.
func TestUnhandledVerbIsDetectable(t *testing.T) {
	rt := newTestRuntime(config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.executeCommand(ctx, dsl.Command{
		Type: dsl.CommandType("DEFINITELY_NOT_A_VERB"),
		Raw:  "nonsense",
	})
	if err == nil || !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("expected the unimplemented-verb error, got %v", err)
	}
}
