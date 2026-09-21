package main

import "errors"

type Transaction struct {
	ID          int
	Amount      float64
	Category    string
	Description string
	Date        string
}

var transactions = make([]Transaction, 0)

func AddTransaction(tx Transaction) error {
	if tx.Amount == 0 {
		return errors.New("сумма транзакции не должна быть равна нулю")
	}

	tx.ID = len(transactions) + 1
	transactions = append(transactions, tx)
	return nil
}

func ListTransactions() []Transaction {
	result := make([]Transaction, len(transactions))
	copy(result, transactions)
	return result
}
