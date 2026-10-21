package ledger

import "sync"

// Gateway обрабатывает HTTP-запросы параллельно, поэтому доступ к данным
// в памяти защищён мьютексом.
var (
	mu           sync.RWMutex
	transactions = make([]Transaction, 0)
	budgets      = make(map[string]Budget)
)

// Reset очищает все транзакции и бюджеты. Нужна тестам, чтобы данные
// одного сценария не влияли на другой.
func Reset() {
	mu.Lock()
	defer mu.Unlock()

	transactions = make([]Transaction, 0)
	budgets = make(map[string]Budget)
}
