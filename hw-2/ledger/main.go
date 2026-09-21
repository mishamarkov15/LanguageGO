package main

import (
	"fmt"
	"log"
)

func main() {
	fmt.Println("Ledger service started")

	examples := []Transaction{
		{Amount: 1250.50, Category: "Продукты", Description: "Покупка продуктов", Date: "2026-09-21"},
		{Amount: 65, Category: "Транспорт", Description: "Поездка на метро", Date: "2026-09-21"},
		{Amount: 450, Category: "Кафе", Description: "Обед", Date: "2026-09-21"},
	}

	for _, tx := range examples {
		if err := AddTransaction(tx); err != nil {
			log.Fatal(err)
		}
	}

	for _, tx := range ListTransactions() {
		fmt.Printf("ID: %d | Сумма: %.2f | Категория: %s | Описание: %s | Дата: %s\n",
			tx.ID, tx.Amount, tx.Category, tx.Description, tx.Date)
	}
}
