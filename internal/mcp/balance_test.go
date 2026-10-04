package mcp

import (
	"net/http"
	"strings"
	"testing"
)

const balanceUpstreamBody = `{
	"usd_balance": "129.46956147",
	"nano_balance": "26.71801147",
	"nanoDepositAddress": "nano_1gx385nnj7rw67hsksa3pyxwnfr48zu13t35ncjmtnqb9zdebtjhh7ahks34"
}`

func TestCheckBalanceReturnsBothBalances(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, jsonUpstream(t, "/api/check-balance", balanceUpstreamBody))

	result := callTool(t, connectedSession(t, deps), "check_balance", map[string]any{})
	if result.IsError {
		t.Fatalf("check_balance failed: %s", errorText(result))
	}

	var out checkBalanceOutput
	structured(t, result, &out)

	if out.USD != 129.46956147 || out.Nano != 26.71801147 {
		t.Errorf("output = %+v, want both balances", out)
	}

	if !strings.HasPrefix(out.DepositAddress, "nano_1") {
		t.Errorf("nanoDepositAddress = %q, want the reported address", out.DepositAddress)
	}
}

func TestCheckBalanceReportsUpstreamFailures(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))

	result := callTool(t, connectedSession(t, deps), "check_balance", map[string]any{})
	if !result.IsError {
		t.Fatalf("IsError = false, want the upstream 401 to fail the call: %s", errorText(result))
	}

	if text := errorText(result); !strings.Contains(text, "401") || !strings.Contains(text, "invalid api key") {
		t.Errorf("error = %q, want the status and body", text)
	}
}
