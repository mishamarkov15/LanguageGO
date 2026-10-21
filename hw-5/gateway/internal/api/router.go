package api

import "net/http"

const apiPrefix = "/api"

func NewRouter() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /transactions", listTransactions)
	api.HandleFunc("POST /transactions", createTransaction)
	api.HandleFunc("/transactions", methodNotAllowed("GET, POST"))
	api.HandleFunc("GET /budgets", listBudgets)
	api.HandleFunc("POST /budgets", createBudget)
	api.HandleFunc("/budgets", methodNotAllowed("GET, POST"))
	api.HandleFunc("/", notFound)

	mux := http.NewServeMux()
	mux.Handle(apiPrefix+"/", http.StripPrefix(apiPrefix, api))
	mux.HandleFunc("/", notFound)

	return Logging(JSONContentType(mux))
}
