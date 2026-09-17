package adcampaign

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	got, diagnostics := Normalize([]string{"  Анна ", "", "Анна", "борис", " Émile ", "Émile"})
	want := []string{"Анна", "борис", "Émile"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
	if diagnostics.DuplicateCount != 2 {
		t.Fatalf("DuplicateCount = %d, want 2", diagnostics.DuplicateCount)
	}
	if !reflect.DeepEqual(diagnostics.InvalidCapitalization, []string{"борис"}) {
		t.Fatalf("InvalidCapitalization = %#v", diagnostics.InvalidCapitalization)
	}
}

func TestDefaultListsAreNormalized(t *testing.T) {
	names, surnames, nameDiagnostics, surnameDiagnostics := Defaults()
	if len(names) == 0 || len(surnames) == 0 {
		t.Fatal("embedded defaults must contain both lists")
	}
	normalizedNames, _ := Normalize(names)
	normalizedSurnames, _ := Normalize(surnames)
	if !reflect.DeepEqual(names, normalizedNames) || !reflect.DeepEqual(surnames, normalizedSurnames) {
		t.Fatal("Defaults returned non-normalized lists")
	}
	if nameDiagnostics.DuplicateCount == 0 || surnameDiagnostics.DuplicateCount == 0 {
		t.Fatal("source duplicate diagnostics unexpectedly empty")
	}
}

func TestHasNamePlaceholder(t *testing.T) {
	if !HasNamePlaceholder("Для {{name}}") || HasNamePlaceholder("Для name") {
		t.Fatal("placeholder validation failed")
	}
}
