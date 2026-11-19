package tool

import (
	"fmt"
	"math"
)

// TipRadius returns the tool's cutting radius (mm) at the given height
// above the tool tip — the tip-envelope radius r(z) of ADR-011. This is the
// number 2D cutter compensation (ADR-013) offsets by at the working Z, and
// the seed of the 3D surface offset (Phase 5).
//
// height is measured up from the lowest point of the tool tip. Heights
// above the cutting length return 0 (the tool is out of its cutting zone).
//
//   - flat end mill: constant D/2 over the cutting length;
//   - ball-nose: the spherical-cap radius sqrt(R² − (R−h)²);
//   - bull-nose: constant D/2 − cr over the flat, then the torus corner;
//   - taper: h·tan(half-angle), from the tip point.
//
// Drill, chamfer, and face-mill tips are not 2.5D contour tools and return
// an error rather than a guessed radius — comp must refuse, not improvise.
func (t Tool) TipRadius(height float64) (float64, error) {
	if height < 0 {
		return 0, fmt.Errorf("tool %q: height must be >= 0 (got %g)", t.ID, height)
	}
	R := t.Diameter / 2
	switch t.Kind {
	case KindFlatEndMill:
		if height > t.FluteLength && t.FluteLength > 0 {
			return 0, nil
		}
		return R, nil
	case KindBallNose:
		if R <= 0 {
			return 0, nil
		}
		if height > 2*R {
			return 0, nil
		}
		// Spherical cap of a sphere radius R centred at height R.
		return math.Sqrt(math.Max(0, R*R-(R-height)*(R-height))), nil
	case KindBullNose:
		flatR := R - t.CornerRadius
		if height <= flatR {
			return flatR, nil
		}
		if height > R {
			return 0, nil
		}
		// Torus corner of radius cr centred at (flatR, R).
		dh := height - R
		return flatR + math.Sqrt(math.Max(0, t.CornerRadius*t.CornerRadius-dh*dh)), nil
	case KindTaperEndMill:
		if t.TaperAngle == 0 {
			return 0, fmt.Errorf("tool %q: taper tool has a zero taper angle; use a flat end mill", t.ID)
		}
		return height * math.Tan(t.TaperAngle*math.Pi/180), nil
	default:
		return 0, fmt.Errorf("tool %q: kind %q is not a 2.5D contour tool; tip radius is undefined", t.ID, t.Kind)
	}
}
