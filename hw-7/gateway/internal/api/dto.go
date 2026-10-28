package api

import "hw-7/ledger"

type CreateTransactionRequest struct {
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Date        string  `json:"date"`
}

type TransactionResponse struct {
	ID          int     `json:"id"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Date        string  `json:"date"`
}

type CreateBudgetRequest struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

type BudgetResponse struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

type CategoryTotalResponse struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
}

type ReportSummaryResponse struct {
	From       string                  `json:"from"`
	To         string                  `json:"to"`
	Total      float64                 `json:"total"`
	Categories []CategoryTotalResponse `json:"categories"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (req CreateTransactionRequest) ToTransaction() ledger.Transaction {
	return ledger.Transaction{
		Amount:      req.Amount,
		Category:    req.Category,
		Description: req.Description,
		Date:        req.Date,
	}
}

func NewTransactionResponse(tx ledger.Transaction) TransactionResponse {
	return TransactionResponse{
		ID:          tx.ID,
		Amount:      tx.Amount,
		Category:    tx.Category,
		Description: tx.Description,
		Date:        tx.Date,
	}
}

func (req CreateBudgetRequest) ToBudget() ledger.Budget {
	return ledger.Budget{
		Category: req.Category,
		Limit:    req.Limit,
	}
}

func NewBudgetResponse(b ledger.Budget) BudgetResponse {
	return BudgetResponse{
		Category: b.Category,
		Limit:    b.Limit,
	}
}

func NewReportSummaryResponse(report ledger.ReportSummary) ReportSummaryResponse {
	categories := make([]CategoryTotalResponse, 0, len(report.Categories))
	for _, item := range report.Categories {
		categories = append(categories, CategoryTotalResponse{Category: item.Category, Total: item.Total})
	}
	return ReportSummaryResponse{
		From:       report.From,
		To:         report.To,
		Total:      report.Total,
		Categories: categories,
	}
}
