package components

import (
	"strings"

	"github.com/nkhine/gohtmxelm/demo/internal/statement"
)

func gbp(minor int64) string {
	return statement.FormatGBP(minor)
}

// moneyOut / moneyIn render only the relevant leg of a transfer.
func moneyOut(t statement.Transfer) string {
	if t.Direction == "DEBIT" {
		return statement.FormatGBP(t.AmountMinor)
	}
	return ""
}

func moneyIn(t statement.Transfer) string {
	if t.Direction == "CREDIT" {
		return statement.FormatGBP(t.AmountMinor)
	}
	return ""
}

func transferTime(t statement.Transfer) string {
	return t.At.Format("2 Jan 15:04")
}

func statusClass(status string) string {
	return "status-" + strings.ToLower(status)
}
