package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type checkBalanceOutput struct {
	USD            float64 `json:"usdBalance" jsonschema:"what is left to spend, in USD"`
	Nano           float64 `json:"nanoBalance" jsonschema:"the same credit, as Nano"`
	DepositAddress string  `json:"nanoDepositAddress,omitempty" jsonschema:"where to send Nano to top the account up, when nano-gpt reports one"`
}

func registerCheckBalance(srv *mcp.Server, h *handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "check_balance",
		Description: "Read the nano-gpt account balance, which is what funds every other tool call. Free, " +
			"and worth a call before an expensive generation.",
	}, h.checkBalance)
}

func (h *handlers) checkBalance(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (
	*mcp.CallToolResult, checkBalanceOutput, error,
) {
	balance, err := h.nanogpt.CheckBalance(ctx)
	if err != nil {
		return nil, checkBalanceOutput{}, h.fail("check_balance", err)
	}

	return nil, checkBalanceOutput{
		USD:            balance.USD,
		Nano:           balance.Nano,
		DepositAddress: balance.DepositAddress,
	}, nil
}
