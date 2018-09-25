package gogekko

import (
	"fmt"
	"strconv"
	"strings"
)

// Amount value information
type Amount struct {
	val int64
}

// DecimalPlaces stores precision information
type DecimalPlaces struct {
	val int
}

// Money represents Monetary value and prescision information
type Money struct {
	amount        *Amount
	decimalPlaces *DecimalPlaces
}

// NewFromInt creates new instance of Money from int64 and decimalPlaces
func NewFromInt(amount int64, decimalPlaces int) *Money {
	return &Money{
		amount:        &Amount{val: amount},
		decimalPlaces: &DecimalPlaces{val: decimalPlaces},
	}
}

// NewFromFloat creates new instance of Money from float64
func NewFromFloat(f float64) *Money {
	s := fmt.Sprintf("%g", f)
	i := strings.Index(s, ".") + 1

	m := &Money{
		amount:        &Amount{val: 0},
		decimalPlaces: &DecimalPlaces{val: len(s) - i},
	}
	m.parseString(s)

	return m
}

func (m *Money) Amount() int64 {
	return m.amount.val
}

func (m *Money) parseString(s string) {
	if strings.Contains(s, ".") {
		s = strings.Replace(s, ".", "", -1)
	} else {
		for i := 0; i < m.decimalPlaces.val; i++ {
			s += "0"
		}
	}

	i, _ := strconv.ParseInt(s, 10, 64)
	m.amount.val = i
}

func (m *Money) parseFloat(f float64) {
	s := fmt.Sprintf("%f", f)
	m.parseString(s)
}

func (m *Money) compare(om *Money) int {
	switch {
	case m.amount.val > om.amount.val:
		return 1
	case m.amount.val < om.amount.val:
		return -1
	}

	return 0
}
