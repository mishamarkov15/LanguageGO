package ledger

import "math"

const (
	dateLayout = "2006-01-02"

	// maxMoney — наибольшее значение, которое помещается в NUMERIC(14,2).
	maxMoney = 999_999_999_999.99
)

type Validatable interface {
	Validate() error
}

func CheckValid(v Validatable) error {
	return v.Validate()
}

// hasCents сообщает, что у суммы не больше двух знаков после запятой,
// то есть она сохранится в NUMERIC(14,2) без округления.
func hasCents(x float64) bool {
	return math.Round(x*100)/100 == x
}

// toCents переводит сумму в копейки. Сравнение целых копеек не зависит
// от погрешностей float64: 0.1 + 0.2 не превышает лимит 0.3.
func toCents(x float64) int64 {
	return int64(math.Round(x * 100))
}

func fromCents(cents int64) float64 {
	return float64(cents) / 100
}
