package gogekko

import "testing"

func TestNew(t *testing.T) {
	m := New(1, 2)

	if m.amount.val != 1 {
		t.Errorf("Expected %d got %d", 1, m.amount.val)
	}

	if m.decimalPlaces.val != 2 {
		t.Errorf("Expected %d got %d", 2, m.amount.val)
	}

	m = New(-100, 2)

	if m.amount.val != -100 {
		t.Errorf("Expected %d got %d", -100, m.amount.val)
	}
}
