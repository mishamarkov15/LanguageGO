package api

import (
	"net/http"
	"strconv"

	"hw-11/ledger"
)

type importErrorResponse struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

type importResponse struct {
	Accepted int64                 `json:"accepted"`
	Rejected int64                 `json:"rejected"`
	Errors   []importErrorResponse `json:"errors"`
}

func (h handlers) importTransactions(w http.ResponseWriter, r *http.Request) {
	workers := 4
	if values, ok := r.URL.Query()["workers"]; ok {
		n, err := strconv.Atoi(values[0])
		if err != nil || len(values) != 1 {
			writeError(w, http.StatusBadRequest, "ожидается одно целое значение workers")
			return
		}
		workers = n
	}
	var req []CreateTransactionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req == nil {
		writeError(w, http.StatusBadRequest, "ожидается JSON-массив транзакций")
		return
	}
	items := make([]ledger.Transaction, len(req))
	for i, entry := range req {
		items[i] = entry.ToTransaction()
	}
	summary, err := h.service.ImportTransactions(r.Context(), items, workers)
	if err != nil {
		serviceError(w, err)
		return
	}
	body := importResponse{Accepted: summary.Accepted, Rejected: summary.Rejected, Errors: make([]importErrorResponse, len(summary.Errors))}
	for i, entry := range summary.Errors {
		_, message := describeError(entry.Err)
		body.Errors[i] = importErrorResponse{Index: entry.Index, Error: message}
	}
	writeJSON(w, http.StatusOK, body)
}
