package gogekko

import (
	"fmt"
	"strconv"
	"strings"
)

type Amount struct {
	val int64
}

type DecimalPlaces struct {
	val int
}

type money struct {
	amount        *Amount
	decimalPlaces *DecimalPlaces
}

func New(amount int64, decimalPlaces int) *money {
	return &money{
		amount:        &Amount{val: amount},
		decimalPlaces: &DecimalPlaces{val: decimalPlaces},
	}
}

func (m *money) Amount() int64 {
	return m.amount.val
}

func (m *money) parseString(s string) {
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

func (m *money) parseFloat(f float64) {
	s := fmt.Sprintf("%f", f)
	m.parseString(s)
}

func (m *money) compare(om *money) int {
	switch {
	case m.amount.val > om.amount.val:
		return 1
	case m.amount.val < om.amount.val:
		return -1
	}

	return 0
}
