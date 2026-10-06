package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
)

func main() {
	fmt.Println("Ledger service started")

	SetBudget(Budget{Category: "Еда", Limit: 4000})
	SetBudget(Budget{Category: "Транспорт", Limit: 1000})

	file, err := os.Open("budgets.json")
	if err != nil {
		log.Fatalf("не удалось открыть budgets.json: %v", err)
	}
	if err := LoadBudgets(bufio.NewReader(file)); err != nil {
		file.Close()
		log.Fatal(err)
	}
	if err := file.Close(); err != nil {
		log.Fatalf("не удалось закрыть budgets.json: %v", err)
	}

	examples := []Transaction{
		{Amount: 3000, Category: "Еда", Description: "Продукты", Date: "2026-10-06"},
		{Amount: 120, Category: "Транспорт", Description: "Поездка на метро", Date: "2026-10-06"},
		{Amount: 2000, Category: "Еда", Description: "Покупка до лимита бюджета", Date: "2026-10-06"},
		{Amount: 500, Category: "Еда", Description: "Покупка сверх бюджета", Date: "2026-10-06"},
		{Amount: 750, Category: "Книги", Description: "Категория без бюджета", Date: "2026-10-06"},
		{Amount: 0, Category: "Еда", Description: "Нулевая сумма", Date: "2026-10-06"},
	}

	for _, tx := range examples {
		if err := AddTransaction(tx); err != nil {
			fmt.Printf("Отказ (%s): %v\n", tx.Description, err)
			continue
		}
		fmt.Printf("Добавлено: %s — %.2f\n", tx.Category, tx.Amount)
	}

	fmt.Println("\nСохранённые транзакции:")
	for _, tx := range ListTransactions() {
		fmt.Printf("ID: %d | Сумма: %.2f | Категория: %s | Описание: %s | Дата: %s\n",
			tx.ID, tx.Amount, tx.Category, tx.Description, tx.Date)
	}
}
