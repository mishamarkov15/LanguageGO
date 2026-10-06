package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
)

func main() {
	fmt.Println("Ledger service started")

	fmt.Println("Проверка через интерфейс Validatable:")
	values := []Validatable{
		Transaction{Amount: 120, Category: "Транспорт", Date: "2026-10-06"},
		Budget{Category: "Еда", Limit: 5000},
		Transaction{Amount: -100, Category: "Еда", Date: "2026-10-06"},
		Budget{Category: "Еда", Limit: 0},
	}
	for _, value := range values {
		if err := CheckValid(value); err != nil {
			fmt.Printf("%T: ошибка — %v\n", value, err)
			continue
		}
		fmt.Printf("%T: данные корректны\n", value)
	}

	initialBudgets := []Budget{
		{Category: "Еда", Limit: 4000},
		{Category: "Транспорт", Limit: 1000},
	}
	for _, budget := range initialBudgets {
		if err := SetBudget(budget); err != nil {
			log.Fatal(err)
		}
	}

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
		{Amount: -100, Category: "Еда", Description: "Отрицательная сумма", Date: "2026-10-06"},
		{Amount: 100, Category: "", Description: "Пустая категория", Date: "2026-10-06"},
		{Amount: 100, Category: "Еда", Description: "Некорректная дата", Date: "06.10.2026"},
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
