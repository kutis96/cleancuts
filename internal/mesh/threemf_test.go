package mesh

import (
	"archive/zip"
	"bytes"
	"math"
	"strings"
	"testing"
)

// make3MF wraps model XML in a minimal valid 3MF (zip) archive.
func make3MF(t *testing.T, modelXML string, parts ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range parts {
		if name == "3D/3dmodel.model" {
			w, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			w.Write([]byte(modelXML))
			continue
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("unused"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const tetraModel = `<?xml version="1.0" encoding="UTF-8"?>
<model unit="millimeter" xml:lang="en-US" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">
  <resources>
    <object id="1" type="model" name="tetra">
      <mesh>
        <vertices>
          <vertex x="0" y="0" z="0"/>
          <vertex x="1" y="0" z="0"/>
          <vertex x="0" y="1" z="0"/>
          <vertex x="0" y="0" z="1"/>
        </vertices>
        <triangles>
          <triangle v1="0" v2="2" v3="1"/>
          <triangle v1="0" v2="3" v3="2"/>
          <triangle v1="0" v2="1" v3="3"/>
          <triangle v1="1" v2="2" v3="3"/>
        </triangles>
      </mesh>
    </object>
  </resources>
  <build><item objectid="1"/></build>
</model>`

func TestParse3MF(t *testing.T) {
	data := make3MF(t, tetraModel, "3D/3dmodel.model")
	m, err := Parse3MF(data)
	if err != nil {
		t.Fatalf("Parse3MF: %v", err)
	}
	if len(m.Vertices) != 4 || len(m.Triangles) != 4 {
		t.Fatalf("got %v vertices / %v triangles, want 4/4", len(m.Vertices), len(m.Triangles))
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("parsed 3MF fails validation: %v", err)
	}
	// Same solid: the slice at 0.5 is the (1/2)²/2 triangle.
	loops, err := m.Slice(0.5)
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if len(loops) != 1 || math.Abs(loops[0].SignedArea()) != 0.125 {
		t.Errorf("3MF tetra slice wrong")
	}
}

func TestParse3MFUnitInch(t *testing.T) {
	model := strings.Replace(tetraModel, `unit="millimeter"`, `unit="inch"`, 1)
	m, err := Parse3MF(make3MF(t, model, "3D/3dmodel.model"))
	if err != nil {
		t.Fatalf("Parse3MF: %v", err)
	}
	// 1 inch = 25.4 mm, converted exactly once at import.
	if got := m.Vertices[1].X; math.Abs(got-25.4) > 1e-12 {
		t.Errorf("inch conversion: X = %g, want 25.4", got)
	}
}

func TestParse3MFUnsupportedUnit(t *testing.T) {
	model := strings.Replace(tetraModel, `unit="millimeter"`, `unit="micrometer"`, 1)
	_, err := Parse3MF(make3MF(t, model, "3D/3dmodel.model"))
	if err == nil {
		t.Fatalf("unsupported units must be rejected, not guessed")
	}
	if !strings.Contains(err.Error(), "unsupported unit") {
		t.Errorf("error should name the unit: %v", err)
	}
}

func TestParse3MFNoModelPart(t *testing.T) {
	data := make3MF(t, "", "metadata.xml")
	if _, err := Parse3MF(data); err == nil {
		t.Fatalf("a 3MF without a .model part must be rejected")
	}
}

func TestParse3MFNoMeshObject(t *testing.T) {
	model := `<model xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">
  <resources>
    <object id="1" type="support"><mesh><vertices><vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="0" y="1" z="0"/></vertices></mesh></object>
  </resources>
</model>`
	_, err := Parse3MF(make3MF(t, model, "3D/3dmodel.model"))
	if err == nil {
		t.Fatalf("an object without triangles must not be taken as the part mesh")
	}
	if !strings.Contains(err.Error(), "no <object>") {
		t.Errorf("error should say no usable mesh object: %v", err)
	}
}

func TestParse3MFFirstMeshObjectWins(t *testing.T) {
	// Two mesh objects: the first one (a unit tetrahedron) must be used,
	// not the second (a 100-unit cube).
	model := `<?xml version="1.0"?>
<model xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">
  <resources>
    <object id="1" type="model">
      <mesh>
        <vertices>
          <vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/>
          <vertex x="0" y="1" z="0"/><vertex x="0" y="0" z="1"/>
        </vertices>
        <triangles>
          <triangle v1="0" v2="2" v3="1"/><triangle v1="0" v2="3" v3="2"/>
          <triangle v1="0" v2="1" v3="3"/><triangle v1="1" v2="2" v3="3"/>
        </triangles>
      </mesh>
    </object>
    <object id="2" type="model">
      <mesh>
        <vertices>
          <vertex x="0" y="0" z="0"/><vertex x="100" y="0" z="0"/>
          <vertex x="0" y="100" z="0"/><vertex x="0" y="0" z="100"/>
        </vertices>
        <triangles>
          <triangle v1="0" v2="2" v3="1"/><triangle v1="0" v2="3" v3="2"/>
          <triangle v1="0" v2="1" v3="3"/><triangle v1="1" v2="2" v3="3"/>
        </triangles>
      </mesh>
    </object>
  </resources>
</model>`
	m, err := Parse3MF(make3MF(t, model, "3D/3dmodel.model"))
	if err != nil {
		t.Fatalf("Parse3MF: %v", err)
	}
	if got := m.Bounds().Max.X; got != 1 {
		t.Errorf("first mesh object must win: Max.X = %g, want 1", got)
	}
}
