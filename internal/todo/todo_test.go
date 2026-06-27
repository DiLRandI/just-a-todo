package todo

import (
	"reflect"
	"testing"
)

func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{" Home,work ", "home", ""})
	want := []string{"home", "work"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
}

func TestNormalizePriorityAndRepeatRejectInvalidValues(t *testing.T) {
	if _, err := NormalizePriority("urgent"); err == nil {
		t.Fatal("expected invalid priority error")
	}
	if _, err := NormalizeRepeat("yearly"); err == nil {
		t.Fatal("expected invalid repeat error")
	}
}
