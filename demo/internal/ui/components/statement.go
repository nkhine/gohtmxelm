package components

import (
	"strings"

	"github.com/nkhine/gohtmxelm/demo/internal/statement"
)

// statementRow pairs a transfer with its running balance so the table view can
// render a per-row balance without recomputing in the template.
type statementRow struct {
	T            statement.Transfer
	BalanceMinor int64
}

// statementRows attaches the running balance (from the opening balance) to each
// transfer in a newest-first slice.
func statementRows(transfers []statement.Transfer, opening int64) []statementRow {
	bal := statement.RunningBalance(opening, transfers)
	rows := make([]statementRow, len(transfers))
	for i, t := range transfers {
		rows[i] = statementRow{T: t, BalanceMinor: bal[t.ID]}
	}
	return rows
}

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

func balanceClass(minor int64) string {
	if minor < 0 {
		return "balance negative"
	}
	return "balance"
}
