package pipeline

import (
	"reflect"
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/sensitive"
)

var sites = []sensitive.Site{
	{API: "a.A", From: 10, To: 10},
	{API: "b.B", From: 12, To: 15},
	{API: "a.A", From: 20, To: 20},
}

func TestSensitive_AddedLineOnCall(t *testing.T) {
	got := touchedAPIs(sites, []int{10}, "")
	if !reflect.DeepEqual(got, []string{"a.A"}) {
		t.Errorf("got %v", got)
	}
}

func TestSensitive_AddedLineElsewhere(t *testing.T) {
	if got := touchedAPIs(sites, []int{11, 16, 30}, ""); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestSensitive_AddedArgLineOfCall(t *testing.T) {
	got := touchedAPIs(sites, []int{14}, "")
	if !reflect.DeepEqual(got, []string{"b.B"}) {
		t.Errorf("got %v", got)
	}
}

func TestSensitive_NewFunction(t *testing.T) {
	got := touchedAPIs(sites, []int{9, 10, 11, 12, 13, 14, 15, 16, 20}, "")
	if !reflect.DeepEqual(got, []string{"a.A", "b.B"}) {
		t.Errorf("got %v", got)
	}
}

func TestSensitive_CriticalWholeFunction(t *testing.T) {
	got := touchedAPIs(sites, []int{30}, "internal/pay/...")
	if !reflect.DeepEqual(got, []string{"a.A", "b.B"}) {
		t.Errorf("got %v", got)
	}
}
