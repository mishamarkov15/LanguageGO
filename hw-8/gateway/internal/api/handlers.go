package api

import (
	"errors"
	"log"
	"net/http"

	"hw-8/ledger"
)

type handlers struct {
	service ledger.Service
}

func (h handlers) createTransaction(w http.ResponseWriter, r *http.Request) {
	var req CreateTransactionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	saved, err := h.service.AddTransaction(r.Context(), req.ToTransaction())
	if err != nil {
		serviceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, NewTransactionResponse(saved))
}

func (h handlers) listTransactions(w http.ResponseWriter, r *http.Request) {
	transactions, err := h.service.ListTransactions(r.Context())
	if err != nil {
		serviceError(w, err)
		return
	}

	result := make([]TransactionResponse, 0, len(transactions))
	for _, tx := range transactions {
		result = append(result, NewTransactionResponse(tx))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h handlers) createBudget(w http.ResponseWriter, r *http.Request) {
	var req CreateBudgetRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	budget := req.ToBudget()
	if err := h.service.SetBudget(r.Context(), budget); err != nil {
		serviceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, NewBudgetResponse(budget))
}

func (h handlers) listBudgets(w http.ResponseWriter, r *http.Request) {
	budgets, err := h.service.ListBudgets(r.Context())
	if err != nil {
		serviceError(w, err)
		return
	}

	result := make([]BudgetResponse, 0, len(budgets))
	for _, budget := range budgets {
		result = append(result, NewBudgetResponse(budget))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h handlers) getReportSummary(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	report, err := h.service.GetReportSummary(r.Context(), query.Get("from"), query.Get("to"))
	if err != nil {
		serviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, NewReportSummaryResponse(report))
}

func serviceError(w http.ResponseWriter, err error) {
	var validationErr *ledger.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeError(w, http.StatusBadRequest, validationErr.Error())
	case errors.Is(err, ledger.ErrBudgetExceeded):
		writeError(w, http.StatusConflict, "budget exceeded")
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
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
