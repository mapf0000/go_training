package gogekko

import "testing"

func TestNewFromInt(t *testing.T) {
	m := NewFromInt(1, 2)

	if m.amount.val != 1 {
		t.Errorf("Expected %d got %d", 1, m.amount.val)
	}

	if m.decimalPlaces.val != 2 {
		t.Errorf("Expected %d got %d", 2, m.amount.val)
	}

	m = NewFromInt(-100, 2)

	if m.amount.val != -100 {
		t.Errorf("Expected %d got %d", -100, m.amount.val)
	}
}

func TestNewFromFloat(t *testing.T) {
	m := NewFromFloat(0.1)

	if m.amount.val != 1 {
		t.Errorf("Expected %d got %d", 1, m.amount.val)
	}

	if m.decimalPlaces.val != 1 {
		t.Errorf("Expected %d got %d", 1, m.amount.val)
	}

	m = NewFromFloat(0.1575)

	if m.amount.val != 1575 {
		t.Errorf("Expected %d got %d", 1575, m.amount.val)
	}

	if m.decimalPlaces.val != 4 {
		t.Errorf("Expected %d got %d", 4, m.amount.val)
	}

	m = NewFromFloat(-0.1)

	if m.amount.val != -1 {
		t.Errorf("Expected %d got %d", -1, m.amount.val)
	}

	if m.decimalPlaces.val != 1 {
		t.Errorf("Expected %d got %d", 1, m.amount.val)
	}

	m = NewFromFloat(0.100)

	if m.amount.val != 1 {
		t.Errorf("Expected %d got %d", 1, m.amount.val)
	}

	if m.decimalPlaces.val != 1 {
		t.Errorf("Expected %d got %d", 1, m.amount.val)
	}
}
