package mesh

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// ParseSTL parses an STL document — ASCII or binary (little-endian) — into
// a Mesh in the canonical WCS (mm; STL is unitless by convention and the
// file is taken as-is). Detection is "try the ASCII grammar, fall back to
// binary", so a binary file whose 80-byte header happens to start with
// "solid" still parses. Facet normals are parsed but not trusted: the
// winding defines orientation (ADR-017). Errors name the line (ASCII) or
// the triangle (binary).
func ParseSTL(data []byte) (Mesh, error) {
	m, asciiErr := parseSTLASCII(data)
	if asciiErr == nil {
		return m, nil
	}
	m, binErr := parseSTLBinary(data)
	if binErr == nil {
		return m, nil
	}
	// Both failed: report both diagnostics — the ASCII one is often the
	// precise, line-numbered explanation of what was actually typed.
	return Mesh{}, fmt.Errorf("stl: not valid ASCII or binary STL (ascii: %v; binary: %v)", asciiErr, binErr)
}

// parseSTLASCII accepts the canonical ASCII STL grammar:
//
//	solid [name]
//	facet normal nx ny nz
//	outer loop
//	vertex x y z   (×3)
//	endloop
//	endfacet
//	...
//	endsolid [name]
//
// Whitespace is liberal; unknown tokens are an error with the line number.
func parseSTLASCII(data []byte) (Mesh, error) {
	var (
		m     Mesh
		line  int
		cur   [3]kernel.Point3
		nv    int // vertices seen in the current loop
		state = stSolid
	)
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line++
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		tok := strings.ToLower(fields[0])
		switch state {
		case stSolid:
			if tok != "solid" {
				return Mesh{}, fmt.Errorf("stl: line %d: expected \"solid\", got %q", line, fields[0])
			}
			state = stFacet
		case stFacet:
			if tok == "endsolid" {
				state = stEndsolid
				continue
			}
			if tok != "facet" {
				return Mesh{}, fmt.Errorf("stl: line %d: expected \"facet\", got %q", line, fields[0])
			}
			// "facet normal nx ny nz" — the normal is parsed but not
			// trusted (winding wins); a malformed one is still an error.
			if len(fields) != 5 || strings.ToLower(fields[1]) != "normal" {
				return Mesh{}, fmt.Errorf("stl: line %d: expected \"facet normal x y z\" (got %q)", line, strings.Join(fields, " "))
			}
			for j, f := range fields[2:] {
				if _, err := strconv.ParseFloat(f, 64); err != nil {
					return Mesh{}, fmt.Errorf("stl: line %d: facet normal component %d is not a number (%q)", line, j, f)
				}
			}
			state = stLoop
		case stLoop:
			if tok != "outer" || len(fields) < 2 || strings.ToLower(fields[1]) != "loop" {
				return Mesh{}, fmt.Errorf("stl: line %d: expected \"outer loop\", got %q", line, strings.Join(fields, " "))
			}
			nv = 0
			state = stVertex
		case stVertex:
			if tok == "vertex" {
				if len(fields) != 4 {
					return Mesh{}, fmt.Errorf("stl: line %d: \"vertex\" needs \"x y z\" (got %d fields)", line, len(fields))
				}
				var p kernel.Point3
				for j, f := range fields[1:] {
					v, err := strconv.ParseFloat(f, 64)
					if err != nil {
						return Mesh{}, fmt.Errorf("stl: line %d: vertex %c is not a number (%q)", line, "xyz"[j], f)
					}
					switch j {
					case 0:
						p.X = v
					case 1:
						p.Y = v
					default:
						p.Z = v
					}
				}
				if nv >= 3 {
					return Mesh{}, fmt.Errorf("stl: line %d: more than 3 vertices in one loop", line)
				}
				cur[nv] = p
				nv++
				if nv == 3 {
					state = stEndloop
				}
				continue
			}
			if tok == "endloop" {
				return Mesh{}, fmt.Errorf("stl: line %d: loop has %d vertices (want 3)", line, nv)
			}
			return Mesh{}, fmt.Errorf("stl: line %d: expected \"vertex\" or \"endloop\", got %q", line, fields[0])
		case stEndloop:
			if tok != "endloop" {
				return Mesh{}, fmt.Errorf("stl: line %d: expected \"endloop\", got %q", line, fields[0])
			}
			m.Vertices = append(m.Vertices, cur[0], cur[1], cur[2])
			m.Triangles = append(m.Triangles, Tri{
				A: len(m.Vertices) - 3,
				B: len(m.Vertices) - 2,
				C: len(m.Vertices) - 1,
			})
			state = stEndfacet
		case stEndfacet:
			if tok != "endfacet" {
				return Mesh{}, fmt.Errorf("stl: line %d: expected \"endfacet\", got %q", line, fields[0])
			}
			state = stFacet
		case stEndsolid:
			// After endsolid only comments/blank lines may follow.
			if !isASCIIComment(fields) {
				return Mesh{}, fmt.Errorf("stl: line %d: unexpected token after \"endsolid\" (%q)", line, fields[0])
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Mesh{}, fmt.Errorf("stl: reading input: %v", err)
	}
	if state == stSolid {
		return Mesh{}, fmt.Errorf("stl: empty input (no \"solid\" block)")
	}
	if state != stFacet && state != stEndsolid {
		return Mesh{}, fmt.Errorf("stl: file ended in the middle of a facet (line %d)", line)
	}
	if len(m.Triangles) == 0 {
		return Mesh{}, fmt.Errorf("stl: no facets found")
	}
	return m, nil
}

func isASCIIComment(fields []string) bool {
	// Tolerate trailing comment lines like "// generated by ...".
	return strings.HasPrefix(fields[0], "//")
}

type stlState int

const (
	stSolid stlState = iota
	stFacet
	stLoop
	stVertex
	stEndloop
	stEndfacet
	stEndsolid
)

// parseSTLBinary reads the 84-byte header + uint32 count + 50-byte
// records (normal 3×float32, 3×vertex 3×float32, uint16 attribute).
func parseSTLBinary(data []byte) (Mesh, error) {
	const header, rec = 84, 50
	if len(data) < header {
		return Mesh{}, fmt.Errorf("binary: %d bytes, need at least %d (header + count)", len(data), header)
	}
	n := binary.LittleEndian.Uint32(data[header-4 : header])
	if len(data) != header+int(n)*rec {
		return Mesh{}, fmt.Errorf("binary: header declares %d triangles (%d bytes) but the file is %d bytes",
			n, len(data)-header, len(data))
	}
	m := Mesh{
		Vertices:  make([]kernel.Point3, 0, int(n)*3),
		Triangles: make([]Tri, 0, int(n)),
	}
	for i := 0; i < int(n); i++ {
		base := header + i*rec
		v := make([]kernel.Point3, 3)
		for k := range v {
			off := base + 12 + k*12
			v[k] = kernel.P3(
				float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off:]))),
				float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off+4:]))),
				float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off+8:]))),
			)
		}
		m.Vertices = append(m.Vertices, v...)
		m.Triangles = append(m.Triangles, Tri{
			A: len(m.Vertices) - 3,
			B: len(m.Vertices) - 2,
			C: len(m.Vertices) - 1,
		})
	}
	return m, nil
}
