package api

import (
	"net/http"

	"hw-8/ledger"
)

const apiPrefix = "/api"

func NewRouter(service ledger.Service) http.Handler {
	h := handlers{service: service}
	api := http.NewServeMux()
	api.HandleFunc("GET /transactions", h.listTransactions)
	api.HandleFunc("POST /transactions", h.createTransaction)
	api.HandleFunc("/transactions", methodNotAllowed("GET, POST"))
	api.HandleFunc("GET /budgets", h.listBudgets)
	api.HandleFunc("POST /budgets", h.createBudget)
	api.HandleFunc("/budgets", methodNotAllowed("GET, POST"))
	api.HandleFunc("GET /reports/summary", h.getReportSummary)
	api.HandleFunc("/reports/summary", methodNotAllowed("GET"))
	api.HandleFunc("/", notFound)

	mux := http.NewServeMux()
	mux.Handle(apiPrefix+"/", http.StripPrefix(apiPrefix, api))
	mux.HandleFunc("/", notFound)

	return Logging(JSONContentType(mux))
}
