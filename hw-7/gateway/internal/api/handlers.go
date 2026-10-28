package api

import (
	"errors"
	"log"
	"net/http"

	"hw-7/ledger"
)

func createTransaction(w http.ResponseWriter, r *http.Request) {
	var req CreateTransactionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tx := req.ToTransaction()
	if err := tx.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	saved, err := ledger.AddTransaction(r.Context(), tx)
	switch {
	case errors.Is(err, ledger.ErrBudgetExceeded):
		writeError(w, http.StatusConflict, "budget exceeded")
		return
	case err != nil:
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, NewTransactionResponse(saved))
}

func listTransactions(w http.ResponseWriter, r *http.Request) {
	transactions, err := ledger.ListTransactions(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}

	result := make([]TransactionResponse, 0, len(transactions))
	for _, tx := range transactions {
		result = append(result, NewTransactionResponse(tx))
	}
	writeJSON(w, http.StatusOK, result)
}

func createBudget(w http.ResponseWriter, r *http.Request) {
	var req CreateBudgetRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	budget := req.ToBudget()
	if err := budget.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := ledger.SetBudget(r.Context(), budget); err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, NewBudgetResponse(budget))
}

func listBudgets(w http.ResponseWriter, r *http.Request) {
	budgets, err := ledger.ListBudgets(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}

	result := make([]BudgetResponse, 0, len(budgets))
	for _, budget := range budgets {
		result = append(result, NewBudgetResponse(budget))
	}
	writeJSON(w, http.StatusOK, result)
}

func getReportSummary(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	period := ledger.Period{From: query.Get("from"), To: query.Get("to")}
	if err := period.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	report, err := ledger.GetReportSummary(r.Context(), period.From, period.To)
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, NewReportSummaryResponse(report))
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not found")
}

func methodNotAllowed(allow string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// internalError не раскрывает клиенту детали ошибки, а только пишет их в лог.
func internalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}
