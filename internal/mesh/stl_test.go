package mesh

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// cubeASTL renders the cube fixture as a canonical ASCII STL document.
func cubeASTL() string {
	var b strings.Builder
	fmt.Fprintf(&b, "solid cube\n")
	for _, t := range cube().Triangles {
		v := cube().Vertices
		fmt.Fprintf(&b, "  facet normal 0 0 1\n    outer loop\n")
		for _, idx := range [3]int{t.A, t.B, t.C} {
			p := v[idx]
			fmt.Fprintf(&b, "      vertex %g %g %g\n", p.X, p.Y, p.Z)
		}
		b.WriteString("    endloop\n  endfacet\n")
	}
	b.WriteString("endsolid cube\n")
	return b.String()
}

func TestParseSTLASCII(t *testing.T) {
	m, err := ParseSTL([]byte(cubeASTL()))
	if err != nil {
		t.Fatalf("ParseSTL(ascii): %v", err)
	}
	if len(m.Vertices) != 36 || len(m.Triangles) != 12 {
		t.Fatalf("got %v vertices / %v triangles, want 36/12", len(m.Vertices), len(m.Triangles))
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("parsed ASCII cube fails validation: %v", err)
	}
	// It is the same solid: a slice must give the 1×1 square.
	loops, err := m.Slice(0.5)
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if len(loops) != 1 || math.Abs(loops[0].SignedArea()) != 1.0 {
		t.Errorf("slice of parsed cube wrong: %v loops", len(loops))
	}
}

func TestParseSTLASCIIErrors(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		msgPart string
	}{
		{"not a solid", "facet normal 0 0 1\n", `expected "solid"`},
		{"missing vertex", "solid s\nfacet normal 0 0 1\nouter loop\nvertex 0 0 0\nvertex 1 0 0\nendloop\nendfacet\nendsolid s\n", `loop has 2 vertices`},
		{"bad token", "solid s\nwidget\n", `expected "facet"`},
		{"garbage number", "solid s\nfacet normal 0 0 1\nouter loop\nvertex a b c\n", `not a number`},
		{"truncated", "solid s\nfacet normal 0 0 1\nouter loop\nvertex 0 0 0\n", `ended in the middle`},
		{"empty", "", `no "solid" block`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSTL([]byte(tc.doc))
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.msgPart) {
				t.Fatalf("error %q does not mention %q", err, tc.msgPart)
			}
		})
	}
}

// binarySTL builds a binary STL document: 80-byte header + uint32 count +
// 50-byte records (3×float32 normal, 3×3×float32 vertices, uint16 attr).
func binarySTL(header string, tris []Tri, verts []kernel.Point3) []byte {
	buf := new(bytes.Buffer)
	h := make([]byte, 80)
	copy(h, header)
	buf.Write(h)
	binary.Write(buf, binary.LittleEndian, uint32(len(tris)))
	for _, t := range tris {
		buf.Write(make([]byte, 12)) // normal: parsed, not trusted
		for _, idx := range [3]int{t.A, t.B, t.C} {
			p := verts[idx]
			binary.Write(buf, binary.LittleEndian, float32(p.X))
			binary.Write(buf, binary.LittleEndian, float32(p.Y))
			binary.Write(buf, binary.LittleEndian, float32(p.Z))
		}
		binary.Write(buf, binary.LittleEndian, uint16(0))
	}
	return buf.Bytes()
}

func TestParseSTLBinary(t *testing.T) {
	tv := tetra().Vertices
	tt := tetra().Triangles
	data := binarySTL("some exporter header", tt, tv)
	m, err := ParseSTL(data)
	if err != nil {
		t.Fatalf("ParseSTL(binary): %v", err)
	}
	if len(m.Vertices) != 12 || len(m.Triangles) != 4 {
		t.Fatalf("got %v vertices / %v triangles, want 12/4", len(m.Vertices), len(m.Triangles))
	}
	// The first triangle is {0,2,1}, so the first three vertices are v0, v2, v1.
	if m.Vertices[0] != kernel.P3(0, 0, 0) || m.Vertices[1] != kernel.P3(0, 1, 0) || m.Vertices[2] != kernel.P3(1, 0, 0) {
		t.Errorf("vertex values wrong: %v, %v, %v", m.Vertices[0], m.Vertices[1], m.Vertices[2])
	}
	// Same solid: slice at 0.5 must be the (1/2)²/2 triangle.
	loops, err := m.Slice(0.5)
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if len(loops) != 1 || math.Abs(loops[0].SignedArea()) != 0.125 {
		t.Errorf("binary tetra slice wrong")
	}
}

func TestParseSTLBinaryWithSolidHeader(t *testing.T) {
	// A binary file whose 80-byte header starts with "solid" must still
	// parse as binary (the documented detection rule).
	data := binarySTL("solid looks-like-ascii", tetra().Triangles, tetra().Vertices)
	if _, err := ParseSTL(data); err != nil {
		t.Fatalf("binary STL with 'solid' header: %v", err)
	}
}

func TestParseSTLBinarySizeMismatch(t *testing.T) {
	data := binarySTL("", tetra().Triangles, tetra().Vertices)
	data = data[:len(data)-1] // truncate one byte
	if _, err := ParseSTL(data); err == nil {
		t.Fatalf("truncated binary STL must be rejected")
	}
}

func TestParseSTLGarbage(t *testing.T) {
	junk := make([]byte, 200)
	for i := range junk {
		junk[i] = byte(i % 251) // not ASCII text, not a valid binary size
	}
	if _, err := ParseSTL(junk); err == nil {
		t.Fatalf("garbage input must be rejected")
	}
}
