package tool

import (
	"encoding/json"
	"testing"
)

func TestToolValidate(t *testing.T) {
	valid := Tool{ID: "t1", Name: "10mm 4F", Kind: KindFlatEndMill, Diameter: 10, Flutes: 4, MaxRPM: 24000}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid tool failed: %v", err)
	}
	if valid.Radius() != 5 {
		t.Errorf("radius = %v want 5", valid.Radius())
	}

	if err := (Tool{ID: "t", Diameter: 0, Flutes: 4}).Validate(); err == nil {
		t.Error("expected error for zero diameter")
	}
	if err := (Tool{ID: "t", Diameter: 10, Flutes: 4, CornerRadius: 6}).Validate(); err == nil {
		t.Error("expected error for corner radius > radius")
	}
	if err := (Tool{Diameter: 10, Flutes: 4}).Validate(); err == nil {
		t.Error("expected error for missing ID")
	}
	if err := (Tool{ID: "t", Diameter: 10, Flutes: 4, TaperAngle: 90}).Validate(); err == nil {
		t.Error("expected error for taper angle == 90")
	}
}

func TestLibraryGet(t *testing.T) {
	l := Library{ID: "lib", Name: "Test", Tools: []Tool{
		{ID: "a", Diameter: 6, Flutes: 2},
		{ID: "b", Diameter: 10, Flutes: 4},
	}}
	if err := l.Validate(); err != nil {
		t.Fatalf("library validate: %v", err)
	}
	if got, ok := l.Get("b"); !ok || got.Diameter != 10 {
		t.Errorf("get b failed: %+v ok=%v", got, ok)
	}
	if _, ok := l.Get("z"); ok {
		t.Error("get z should miss")
	}

	// Duplicate IDs must be rejected.
	dup := Library{ID: "lib", Tools: []Tool{
		{ID: "x", Diameter: 6, Flutes: 2},
		{ID: "x", Diameter: 8, Flutes: 2},
	}}
	if err := dup.Validate(); err == nil {
		t.Error("expected duplicate-ID error")
	}
}

func TestToolJSONRoundTrip(t *testing.T) {
	tl := Tool{
		ID: "t1", Name: "10mm", Kind: KindFlatEndMill,
		Diameter: 10, Flutes: 4, MaxRPM: 24000,
		Feeds: []FeedPerTooth{{Material: "aluminum-6061", FZ: 0.15, VC: 200}},
	}
	data, err := json.Marshal(tl)
	if err != nil {
		t.Fatal(err)
	}
	var back Tool
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Diameter != 10 || back.Flutes != 4 || back.Kind != KindFlatEndMill {
		t.Errorf("round trip mismatch: %+v", back)
	}
	if len(back.Feeds) != 1 || back.Feeds[0].Material != "aluminum-6061" || back.Feeds[0].FZ != 0.15 {
		t.Errorf("feeds not preserved: %+v", back.Feeds)
	}
	if err := back.Validate(); err != nil {
		t.Errorf("round-tripped tool should validate: %v", err)
	}
}
