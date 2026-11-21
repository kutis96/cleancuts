package fixture

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// --- Validate ------------------------------------------------------------

func TestFixtureValidate(t *testing.T) {
	goodBox := Volume{Type: KindBox, Box: &Box{Min: kernel.P3(0, 0, 0), Max: kernel.P3(1, 2, 3)}}
	cases := []struct {
		name    string
		f       Fixture
		wantErr bool
		msgPart string
	}{
		{"valid empty fixture", Fixture{ID: "f"}, false, ""},
		{"valid with box", Fixture{ID: "f", KeepOut: []Volume{goodBox}}, false, ""},
		{"missing id", Fixture{KeepOut: []Volume{goodBox}}, true, "id is required"},
		{"bad box", Fixture{ID: "f", KeepOut: []Volume{{
			Type: KindBox, Box: &Box{Min: kernel.P3(1, 0, 0), Max: kernel.P3(1, 2, 3)},
		}}}, true, "min must be < max"},
		{"box type missing box", Fixture{ID: "f", KeepOut: []Volume{{Type: KindBox}}}, true, "needs a box"},
		{"unknown type", Fixture{ID: "f", KeepOut: []Volume{{Type: VolumeKind("torus"), Mesh: &MeshVolume{File: "x.stl"}}}}, true, "unknown type"},
		{"mesh type missing file", Fixture{ID: "f", KeepOut: []Volume{{Type: KindMesh}}}, true, "needs a mesh file"},
		{"workholding validated too", Fixture{ID: "f", Workholding: []Volume{{Type: KindBox}}}, true, "workholding[0]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.f.Validate()
			if c.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.wantErr && !strings.Contains(err.Error(), c.msgPart) {
				t.Fatalf("error %q does not mention %q", err, c.msgPart)
			}
		})
	}
}

// --- KeepOuts mapping ------------------------------------------------------

func TestKeepOutsBox(t *testing.T) {
	f := Fixture{
		ID: "vise",
		KeepOut: []Volume{{
			Type: KindBox,
			Box:  &Box{Min: kernel.P3(-20, -20, 0), Max: kernel.P3(120, -5, 20)},
			Note: "vise jaw (front)",
		}},
	}
	out, err := f.KeepOuts()
	if err != nil {
		t.Fatalf("KeepOuts: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d keep-outs, want 1", len(out))
	}
	ko := out[0]
	if ko.Name != "vise jaw (front)" {
		t.Fatalf("name = %q, want the note", ko.Name)
	}
	if ko.Box == nil || ko.Cyl != nil {
		t.Fatal("want a box keep-out")
	}
	if ko.Box.Min.X != -20 || ko.Box.Max.Y != -5 || ko.Box.Max.Z != 20 {
		t.Fatalf("box = %+v, want the volume's box", ko.Box)
	}
}

func TestKeepOutsCylinder(t *testing.T) {
	cases := []struct {
		name   string
		axis   kernel.Point3
		lo, hi float64
	}{
		{"plus Z", kernel.P3(0, 0, 1), 5, 15},
		{"minus Z", kernel.P3(0, 0, -1), -5, 5},
		{"non-unit plus Z", kernel.P3(0, 0, 3), 5, 15},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := Fixture{
				ID: "clamp",
				KeepOut: []Volume{{
					Type: KindCylinder,
					Cyl: &Cylinder{
						Center: kernel.P3(30, 30, 5),
						Axis:   c.axis,
						Radius: 10,
						Length: 10,
					},
				}},
			}
			out, err := f.KeepOuts()
			if err != nil {
				t.Fatalf("KeepOuts: %v", err)
			}
			ko := out[0]
			if ko.Cyl == nil {
				t.Fatal("want a cylinder keep-out")
			}
			if ko.Cyl.ZMin != c.lo || ko.Cyl.ZMax != c.hi {
				t.Fatalf("z range = [%g, %g], want [%g, %g]", ko.Cyl.ZMin, ko.Cyl.ZMax, c.lo, c.hi)
			}
			if ko.Cyl.Center.X != 30 || ko.Cyl.Center.Y != 30 || ko.Cyl.Radius != 10 {
				t.Fatalf("cyl = %+v", ko.Cyl)
			}
		})
	}
}

func TestKeepOutsRefusals(t *testing.T) {
	t.Run("non-vertical axis", func(t *testing.T) {
		f := Fixture{
			ID: "tilt",
			KeepOut: []Volume{{
				Type: KindCylinder,
				Cyl: &Cylinder{
					Center: kernel.P3(0, 0, 0),
					Axis:   kernel.P3(0, 1, 1), // leaning over
					Radius: 5,
					Length: 10,
				},
			}},
		}
		_, err := f.KeepOuts()
		if err == nil || !strings.Contains(err.Error(), "non-vertical") {
			t.Fatalf("want a non-vertical refusal, got %v", err)
		}
	})

	t.Run("mesh volume", func(t *testing.T) {
		f := Fixture{
			ID: "blob",
			KeepOut: []Volume{{
				Type: KindMesh,
				Mesh: &MeshVolume{File: "claw.stl"},
			}},
		}
		_, err := f.KeepOuts()
		if err == nil || !strings.Contains(err.Error(), "claw.stl") {
			t.Fatalf("want a mesh refusal naming the file, got %v", err)
		}
	})

	t.Run("empty fixture maps to no keep-outs", func(t *testing.T) {
		out, err := Fixture{ID: "bare"}.KeepOuts()
		if err != nil {
			t.Fatalf("KeepOuts: %v", err)
		}
		if len(out) != 0 {
			t.Fatalf("got %d keep-outs, want 0", len(out))
		}
	})
}

// --- the pinned example -----------------------------------------------------

func TestExampleFixtureRoundTrip(t *testing.T) {
	data, err := os.ReadFile("../../examples/fixture/clamp_setup.json")
	if err != nil {
		t.Fatalf("read example fixture: %v", err)
	}
	var f Fixture
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal example fixture: %v", err)
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("example fixture fails Validate: %v", err)
	}
	if f.ID != "clamp-setup-100x60" {
		t.Fatalf("id = %q", f.ID)
	}
	if len(f.KeepOut) != 3 {
		t.Fatalf("keep_out = %d, want 3", len(f.KeepOut))
	}
	out, err := f.KeepOuts()
	if err != nil {
		t.Fatalf("KeepOuts: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("mapped keep-outs = %d, want 3", len(out))
	}
	// The notes become the zone names the errors will use.
	if out[0].Name != "vise jaw (front) — never enter" {
		t.Fatalf("keep-out 0 name = %q", out[0].Name)
	}
	// Round-trip: re-marshalling keeps the same contract.
	back, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	var again Fixture
	if err := json.Unmarshal(back, &again); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if len(again.KeepOut) != 3 || again.ID != f.ID {
		t.Fatal("round-trip lost data")
	}
}
