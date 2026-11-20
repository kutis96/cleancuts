package mesh

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// Parse3MF parses a 3MF (3D Manufacturing Format) document — a ZIP archive
// containing an XML model — into a Mesh in the canonical WCS (mm).
//
// v1 contract (ADR-017): the first <object> that carries a non-empty
// <mesh> is used (multi-object selection is a later refinement); the
// model's unit attribute is honoured exactly once, at import (millimeter
// is the 3MF default): millimeter passes through, inch ×25.4, centimeter
// ×10, anything else is rejected with a precise error rather than guessed.
//
// The XML is read with a namespace-agnostic token walk (3MF files declare
// the core namespace and sometimes extra ones; Go's schema-driven
// unmarshal would require hard-coding one of them).
func Parse3MF(data []byte) (Mesh, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Mesh{}, fmt.Errorf("3mf: not a valid 3MF (zip) file: %v", err)
	}
	// Find the model part: prefer the conventional 3D/3dmodel.model, then
	// any *.model (case-insensitive).
	var modelFile *zip.File
	for i := range zr.File {
		name := zr.File[i].Name
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".model") {
			continue
		}
		if name == "3D/3dmodel.model" {
			modelFile = zr.File[i]
			break
		}
		if modelFile == nil {
			modelFile = zr.File[i]
		}
	}
	if modelFile == nil {
		return Mesh{}, fmt.Errorf("3mf: no .model part found (expected 3D/3dmodel.model)")
	}
	rc, err := modelFile.Open()
	if err != nil {
		return Mesh{}, fmt.Errorf("3mf: opening %s: %v", modelFile.Name, err)
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return Mesh{}, fmt.Errorf("3mf: reading %s: %v", modelFile.Name, err)
	}
	return parseModelXML(raw)
}

// parseModelXML walks the model XML for the first object with a usable
// mesh (vertices + triangles).
func parseModelXML(raw []byte) (Mesh, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var (
		unit     string
		inObject bool
		verts    []kernel.Point3
		tris     []Tri
		m        Mesh // first mesh object, once complete
		needMore = true
	)
	finishObject := func() {
		inObject = false
		if needMore && len(verts) > 0 && len(tris) > 0 {
			m = Mesh{Vertices: verts, Triangles: tris}
			needMore = false
		}
		verts, tris = nil, nil
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Mesh{}, fmt.Errorf("3mf: model XML is malformed: %v", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "model":
				for _, a := range el.Attr {
					if a.Name.Local == "unit" {
						unit = a.Value
					}
				}
			case "object":
				inObject = true
			case "vertex":
				if inObject {
					p, err := parseAttrPoint(el.Attr, "vertex")
					if err != nil {
						return Mesh{}, err
					}
					verts = append(verts, p)
				}
			case "triangle":
				if inObject {
					t, err := parseAttrTri(el.Attr)
					if err != nil {
						return Mesh{}, err
					}
					tris = append(tris, t)
				}
			}
		case xml.EndElement:
			if el.Name.Local == "object" {
				finishObject()
			}
		}
	}
	if needMore {
		return Mesh{}, fmt.Errorf("3mf: no <object> with a usable <mesh> (vertices + triangles) found")
	}
	scale, err := unitScale(unit)
	if err != nil {
		return Mesh{}, err
	}
	out := Mesh{Vertices: make([]kernel.Point3, len(m.Vertices))}
	for i, v := range m.Vertices {
		out.Vertices[i] = kernel.P3(v.X*scale, v.Y*scale, v.Z*scale)
	}
	out.Triangles = m.Triangles
	return out, nil
}

func parseAttrPoint(attrs []xml.Attr, what string) (kernel.Point3, error) {
	var p kernel.Point3
	got := map[string]bool{}
	for _, a := range attrs {
		v, err := strconv.ParseFloat(a.Value, 64)
		if err != nil {
			return kernel.Point3{}, fmt.Errorf("3mf: %s attribute %s=%q is not a number", what, a.Name.Local, a.Value)
		}
		switch a.Name.Local {
		case "x":
			p.X = v
			got["x"] = true
		case "y":
			p.Y = v
			got["y"] = true
		case "z":
			p.Z = v
			got["z"] = true
		}
	}
	for _, k := range []string{"x", "y", "z"} {
		if !got[k] {
			return kernel.Point3{}, fmt.Errorf("3mf: %s is missing attribute %q", what, k)
		}
	}
	return p, nil
}

func parseAttrTri(attrs []xml.Attr) (Tri, error) {
	var t Tri
	got := map[string]bool{}
	for _, a := range attrs {
		v, err := strconv.Atoi(a.Value)
		if err != nil {
			return Tri{}, fmt.Errorf("3mf: triangle attribute %s=%q is not an index", a.Name.Local, a.Value)
		}
		switch a.Name.Local {
		case "v1":
			t.A = v
			got["v1"] = true
		case "v2":
			t.B = v
			got["v2"] = true
		case "v3":
			t.C = v
			got["v3"] = true
		}
	}
	for _, k := range []string{"v1", "v2", "v3"} {
		if !got[k] {
			return Tri{}, fmt.Errorf("3mf: triangle is missing attribute %q", k)
		}
	}
	return t, nil
}

// unitScale maps a 3MF model unit attribute to a millimetre scale factor.
// The 3MF default (absent attribute) is millimeter (ADR-017: units are
// honoured exactly once, at import).
func unitScale(unit string) (float64, error) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "millimeter", "mm":
		return 1, nil
	case "inch", "in":
		return 25.4, nil
	case "centimeter", "cm":
		return 10, nil
	}
	return 0, fmt.Errorf("3mf: unsupported unit %q (v1 supports millimeter, inch, centimeter — convert the file or extend unitScale)", unit)
}
