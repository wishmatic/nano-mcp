package nanogpt

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

const (
	depositAddress = "nano_1gx385nnj7rw67hsksa3pyxwnfr48zu13t35ncjmtnqb9zdebtjhh7ahks34"

	balanceResponseBody = `{
		"usd_balance": "129.46956147",
		"nano_balance": "26.71801147",
		"nanoDepositAddress": "` + depositAddress + `"
	}`
)

func TestCheckBalanceReadsTheAmounts(t *testing.T) {
	srv, rec := recordingServer(t, balanceResponseBody, http.StatusOK)

	balance, err := newTestClient(t, srv.URL).CheckBalance(context.Background())
	if err != nil {
		t.Fatalf("CheckBalance() error: %v", err)
	}

	if rec.method != http.MethodPost {
		t.Errorf("method = %s, want POST", rec.method)
	}

	if rec.path != "/api/check-balance" {
		t.Errorf("path = %s, want /api/check-balance", rec.path)
	}

	rec.assertHeader(t, "x-api-key", testKey)
	rec.assertHeader(t, "Authorization", "")

	if balance.USD != 129.46956147 || balance.Nano != 26.71801147 {
		t.Errorf("balance = %+v, want the amounts decoded", balance)
	}

	if balance.DepositAddress != depositAddress {
		t.Errorf("DepositAddress = %q, want the reported one", balance.DepositAddress)
	}

	if len(rec.body) != 0 {
		t.Errorf("body = %v, want the endpoint sent no payload", rec.body)
	}
}

func TestCheckBalanceTreatsAnUnreportedAmountAsZero(t *testing.T) {
	srv, _ := recordingServer(t, `{"usd_balance":"0.42"}`, http.StatusOK)

	balance, err := newTestClient(t, srv.URL).CheckBalance(context.Background())
	if err != nil {
		t.Fatalf("CheckBalance() error: %v", err)
	}

	if balance.USD != 0.42 || balance.Nano != 0 {
		t.Errorf("balance = %+v, want the reported USD and no Nano", balance)
	}
}

func TestCheckBalanceRejectsAnAmountItCannotRead(t *testing.T) {
	srv, _ := recordingServer(t, `{"usd_balance":"n/a","nano_balance":"1"}`, http.StatusOK)

	_, err := newTestClient(t, srv.URL).CheckBalance(context.Background())
	if err == nil || !strings.Contains(err.Error(), "usd_balance") {
		t.Errorf("CheckBalance() error = %v, want it to name the unreadable field", err)
	}
}

func TestCheckBalanceReportsUpstreamFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusPaymentRequired} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv, _ := recordingServer(t, `{"error":"invalid api key"}`, status)

			_, err := newTestClient(t, srv.URL).CheckBalance(context.Background())
			if err == nil || !strings.Contains(err.Error(), strconv.Itoa(status)) {
				t.Errorf("CheckBalance() error = %v, want it to name the status", err)
			}

			if err != nil && !strings.Contains(err.Error(), "invalid api key") {
				t.Errorf("error = %q, want the body quoted", err)
			}
		})
	}
}
