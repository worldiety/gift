package cms

import "math"

// The black of the perceptual intent of ICC v4: a reference medium whose
// darkest colour is L* 3.1373 rather than 0, ICC specification 6.2.4.
var perceptualBlack = XYZ{0.00336, 0.0034731, 0.00287}

type blackKey struct {
	intent      Intent
	destination bool
}

// cachedBlack returns the black point under key k, computing it with f
// the first time. The lock is not held while f runs: the destination black
// point of one intent can ask for the black point of another.
func (p *Profile) cachedBlack(k blackKey, f func() XYZ) XYZ {
	p.mu.Lock()
	b, ok := p.black[k]
	p.mu.Unlock()
	if ok {
		return b
	}
	b = f()
	p.mu.Lock()
	if p.black == nil {
		p.black = map[blackKey]XYZ{}
	}
	p.black[k] = b
	p.mu.Unlock()
	return b
}

// blackPoint is the darkest colour the profile produces as a source, for
// black point compensation; LittleCMS, cmssamp.c, cmsDetectBlackPoint.
func (p *Profile) blackPoint(intent Intent) XYZ {
	return p.cachedBlack(blackKey{intent, false}, func() XYZ {
		if intent == AbsoluteColorimetric {
			return XYZ{}
		}
		if p.version >= 0x04000000 && (intent == Perceptual || intent == Saturation) {
			// A v4 matrix profile shares its black between the intents;
			// a v4 table has the perceptual reference black.
			if p.isMatrixShaper() {
				return p.darkestColorant(RelativeColorimetric)
			}
			return perceptualBlack
		}
		if intent == RelativeColorimetric && p.class == ClassOutput && p.space == SpaceCMYK {
			// The ink limit of a printer shows in its perceptual table.
			return p.perceptualBlackPoint()
		}
		return p.darkestColorant(intent)
	})
}

// darkestColorant is the colour of the device's black, all inks or no
// light, as the profile maps it, with its chroma removed and L* at most
// 50 (BlackPointAsDarkerColorant).
func (p *Profile) darkestColorant(intent Intent) XYZ {
	if !p.supports(intent, true) {
		return XYZ{}
	}
	pl, err := p.toPCS(intent)
	if err != nil {
		return XYZ{}
	}
	var black vec
	if p.space == SpaceCMYK {
		black = vec{1, 1, 1, 1}
	}
	v := pl.apply(black)
	l := v[0]
	if p.pcs == SpaceXYZ {
		l, _, _ = xyzToLab(D50, v[0], v[1], v[2])
	}
	l = min(l, 50)
	x, y, z := labToXYZ(D50, l, 0, 0)
	return XYZ{x, y, z}
}

// perceptualBlackPoint is black sent through the profile's perceptual
// table and back through its colorimetric one (BlackPointUsingPerceptualBlack).
func (p *Profile) perceptualBlackPoint() XYZ {
	if !p.supports(Perceptual, true) {
		return XYZ{}
	}
	rt, err := p.roundTrip(Perceptual)
	if err != nil {
		return XYZ{}
	}
	v := rt.apply(vec{})
	x, y, z := labToXYZ(D50, min(v[0], 50), 0, 0)
	return XYZ{x, y, z}
}

// roundTrip is the pipeline from Lab to the device with the intent and
// back to Lab with relative colorimetry (CreateRoundtripXForm).
func (p *Profile) roundTrip(intent Intent) (pipeline, error) {
	out, err := p.fromPCS(intent)
	if err != nil {
		return nil, err
	}
	in, err := p.toPCS(RelativeColorimetric)
	if err != nil {
		return nil, err
	}
	// The link from the Lab profile into this one gets the black point
	// compensation LittleCMS forces on v4 profiles; the Lab profile itself
	// is abstract and has no black point.
	a := affine{m: identity3}
	if p.version >= 0x04000000 && (intent == Perceptual || intent == Saturation) {
		if bo := p.destinationBlackPoint(intent); bo != (XYZ{}) {
			a = bpc(XYZ{}, bo)
		}
	}
	pl := connect(SpaceLab, p.pcs, a)
	pl = append(pl, out...)
	pl = append(pl, in...)
	return append(pl, connect(p.pcs, SpaceLab, affine{m: identity3})...), nil
}

// destinationBlackPoint is the darkest colour the profile reaches as a
// destination (cmsDetectDestinationBlackPoint). For a table profile of v2
// it follows Adobe's algorithm: send a ramp of L* through the profile and
// back, and find where the curve of the result leaves its flat foot.
func (p *Profile) destinationBlackPoint(intent Intent) XYZ {
	return p.cachedBlack(blackKey{intent, true}, func() XYZ {
		if intent == AbsoluteColorimetric {
			return XYZ{}
		}
		if p.version >= 0x04000000 && (intent == Perceptual || intent == Saturation) {
			if p.isMatrixShaper() {
				return p.darkestColorant(RelativeColorimetric)
			}
			return perceptualBlack
		}
		if !p.hasLut(intent) {
			return p.blackPoint(intent)
		}
		var il, ia, ib float64
		if intent == RelativeColorimetric {
			b := p.blackPoint(intent)
			il, ia, ib = xyzToLab(D50, b.X, b.Y, b.Z)
		}
		rt, err := p.roundTrip(intent)
		if err != nil {
			return XYZ{}
		}
		var inRamp, outRamp [256]float64
		a, b := min(max(ia, -50), 50), min(max(ib, -50), 50)
		for l := range 256 {
			inRamp[l] = float64(l) * 100 / 255
			outRamp[l] = rt.apply(vec{inRamp[l], a, b})[0]
		}
		for l := 254; l > 0; l-- {
			outRamp[l] = min(outRamp[l], outRamp[l+1])
		}
		if !(outRamp[0] < outRamp[255]) {
			return XYZ{}
		}
		lo, hi := outRamp[0], outRamp[255]
		if intent == RelativeColorimetric {
			straight := true
			for l := range 256 {
				if !(inRamp[l] <= lo+0.2*(hi-lo) || math.Abs(inRamp[l]-outRamp[l]) < 4) {
					straight = false
				}
			}
			if straight {
				x, y, z := labToXYZ(D50, il, ia, ib)
				return XYZ{x, y, z}
			}
		}
		from, to := 0.03, 0.25
		if intent == RelativeColorimetric {
			from, to = 0.1, 0.5
		}
		var xs, ys []float64
		for l := range 256 {
			if f := (outRamp[l] - lo) / (hi - lo); f >= from && f < to {
				xs = append(xs, inRamp[l])
				ys = append(ys, f)
			}
		}
		if len(xs) < 3 {
			return XYZ{}
		}
		x, y, z := labToXYZ(D50, max(quadraticRoot(xs, ys), 0), ia, ib)
		return XYZ{x, y, z}
	})
}

// quadraticRoot fits y = a x² + b x + c by least squares and returns the
// vertex of the fit, between 0 and 50 (RootOfLeastSquaresFitQuadraticCurve).
func quadraticRoot(xs, ys []float64) float64 {
	if len(xs) < 4 {
		return 0
	}
	var n, sx, sx2, sx3, sx4, sy, syx, syx2 float64
	for i, x := range xs {
		y := ys[i]
		n++
		sx += x
		sx2 += x * x
		sx3 += x * x * x
		sx4 += x * x * x * x
		sy += y
		syx += y * x
		syx2 += y * x * x
	}
	m := mat3{{n, sx, sx2}, {sx, sx2, sx3}, {sx2, sx3, sx4}}
	inv, ok := m.inverse()
	if !ok {
		return 0
	}
	c, b, a := inv.apply(sy, syx, syx2)
	if math.Abs(a) < 1e-10 {
		if math.Abs(b) < 1e-10 {
			return 0
		}
		// Always 0, as in LittleCMS, whose bounds are the wrong way round
		// here; a transform that agrees with it matters more.
		return min(0, max(50, -c/b))
	}
	d := b*b - 4*a*c
	if d <= 0 {
		return 0
	}
	return max(0, min(50, (-b+math.Sqrt(d))/(2*a)))
}

// bpc is the black point compensation from black point in to out: a
// scaling per channel that keeps D50 where it is and moves in to out
// (cmscnvrt.c, ComputeBlackPointCompensation).
func bpc(in, out XYZ) affine {
	tx, ty, tz := in.X-D50.X, in.Y-D50.Y, in.Z-D50.Z
	return affine{
		m: diag((out.X-D50.X)/tx, (out.Y-D50.Y)/ty, (out.Z-D50.Z)/tz),
		off: [3]float64{
			-D50.X * (out.X - in.X) / tx,
			-D50.Y * (out.Y - in.Y) / ty,
			-D50.Z * (out.Z - in.Z) / tz,
		},
	}
}
