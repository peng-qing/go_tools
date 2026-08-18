package mongo_driver

import (
	"reflect"
	"testing"
)

func TestOperatorBuilderAllOperators(t *testing.T) {
	empty := NewOperatorBuilder().
		Set().
		Unset().
		Increment().
		Rename().
		SetOnUpdate().
		Push().
		Pull().
		Pop().
		PullAll().
		AddToSet().
		CurrentDate().
		Bit().
		Build()
	if len(empty) != 0 {
		t.Fatalf("empty operations = %#v", empty)
	}

	update := E{Key: "field", Value: 1}
	builder := NewOperatorBuilder().
		Set(update).
		Unset(update).
		Increment(update).
		Rename(update).
		SetOnUpdate(update).
		Push(update).
		Pull(update).
		Pop(update).
		PullAll(update).
		AddToSet(update).
		CurrentDate(update).
		Bit(update)
	builder.ReplaceWith(D{{Key: "replacement", Value: true}})

	got := builder.Build()
	wantKeys := []string{
		"$set", "$unset", "$inc", "$rename", "$setOnInsert", "$push",
		"$pull", "$pop", "$pullAll", "$addToSet", "$currentDate", "$bit",
		"$replaceWith",
	}
	actualKeys := make([]string, 0, len(got))
	for _, item := range got {
		actualKeys = append(actualKeys, item.Key)
	}
	t.Logf("actual: %v", actualKeys)
	t.Logf("expected: %v", wantKeys)
	if len(got) != len(wantKeys) {
		t.Fatalf("Build() length = %d, want %d: %#v", len(got), len(wantKeys), got)
	}
	for i, key := range wantKeys {
		if got[i].Key != key {
			t.Fatalf("operator %d key = %q, want %q", i, got[i].Key, key)
		}
	}
	if !reflect.DeepEqual(got[0].Value, []E{update}) {
		t.Fatalf("$set value = %#v", got[0].Value)
	}
}

func TestProjectionBuilderOperations(t *testing.T) {
	pb := &ProjectionBuilder{
		fields:      make(map[string]struct{}),
		projections: Projection{},
	}
	if got := pb.Fields().Excludes().Build(); len(got) != 0 {
		t.Fatalf("empty projections = %#v", got)
	}

	pb.Fields("name", "age").
		Excludes("secret").
		Only("status").
		FirstMatchSliceElem("items").
		ElementMatch("matched", NewFilterBuilder().EQ("matched.kind", "x").Build()).
		Slice("window", 2, 3)

	got := pb.Build()
	wantKeys := []string{"name", "age", "secret", "status", "_id", "items.$", "_id", "matched", "window"}
	actualKeys := make([]string, 0, len(got))
	for _, item := range got {
		actualKeys = append(actualKeys, item.Key)
	}
	t.Logf("actual: %v", actualKeys)
	t.Logf("expected: %v", wantKeys)
	if len(got) != len(wantKeys) {
		t.Fatalf("Build() length = %d, want %d: %#v", len(got), len(wantKeys), got)
	}
	for i, key := range wantKeys {
		if got[i].Key != key {
			t.Fatalf("projection %d key = %q, want %q", i, got[i].Key, key)
		}
	}
	before := len(got)
	pb.ElementMatch("matched", Filter{})
	if len(pb.Build()) != before {
		t.Fatal("duplicate ElementMatch was appended")
	}
}
