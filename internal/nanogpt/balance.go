package nanogpt

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const balancePath = "/api/check-balance"

// Balance is the credit left on the account, in both currencies nano-gpt reports it in.
type Balance struct {
	USD            float64
	Nano           float64
	DepositAddress string
}

type balanceResponse struct {
	USDBalance     string `json:"usd_balance"`
	NanoBalance    string `json:"nano_balance"`
	DepositAddress string `json:"nanoDepositAddress"`
}

// CheckBalance reads what is left to spend, which is what funds every other call. The endpoint
// itself bills nothing.
func (c *Client) CheckBalance(ctx context.Context) (*Balance, error) {
	var out balanceResponse

	cred := credential{header: "x-api-key", value: c.apiKey}
	if err := c.post(ctx, balancePath, FastTimeout, cred, nil, &out); err != nil {
		return nil, err
	}

	usd, err := parseAmount("usd_balance", out.USDBalance)
	if err != nil {
		return nil, err
	}

	nano, err := parseAmount("nano_balance", out.NanoBalance)
	if err != nil {
		return nil, err
	}

	return &Balance{USD: usd, Nano: nano, DepositAddress: out.DepositAddress}, nil
}

// parseAmount reads one of the decimal strings this endpoint answers with.
func parseAmount(field, value string) (float64, error) {
	trimmed := strings.TrimSpace(value)

	// A balance nano-gpt leaves out is one it does not report, which is no credit in it.
	if trimmed == "" {
		return 0, nil
	}

	amount, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %s %q is not a number: %w", balancePath, field, trimmed, err)
	}

	return amount, nil
}
