package raw

// rcd is Ratio Corrected Demosaicing, by Luis Sanz Rodríguez (2017), as his
// description of it has it: the green channel first, interpolated along the
// direction that a high pass filter finds smoother, from neighbours
// corrected by the ratio of a low pass filter of the mosaic; then red and
// blue as differences from green, diagonally at the photosites of the
// other colour and then along rows and columns at the green ones.
//
// It works on a tile with a border, in values from 0 to 1; the border of
// tileBorder photosites around the tile is read but not made right.
type rcd struct {
	w, h int
	// colour is the colour of the photosite at (x, y) of the tile, by
	// (y&1)*2 + x&1.
	colour [4]int
	cfa    []float32
	rgb    [3][]float32
	// vh is the vertical against horizontal discrimination, pq the one
	// between the diagonals, lpf the low pass filter at red and blue, and
	// hv, hh the squared high pass filters along columns and rows, also
	// used for the diagonals.
	vh, pq, lpf, hv, hh []float32
}

const (
	rcdEps   = 1e-5
	rcdEpsSq = 1e-10
)

func (r *rcd) resize(w, h int) {
	r.w, r.h = w, h
	n := w * h
	r.cfa = grow(r.cfa, n)
	for c := range 3 {
		r.rgb[c] = grow(r.rgb[c], n)
	}
	r.vh = grow(r.vh, n)
	r.pq = grow(r.pq, n)
	r.lpf = grow(r.lpf, n)
	r.hv = grow(r.hv, n)
	r.hh = grow(r.hh, n)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func clamp01(v float32) float32 {
	return min(max(v, 0), 1)
}

func sq(v float32) float32 { return v * v }

func (r *rcd) at(x, y int) int { return r.colour[(y&1)*2+x&1] }

func (r *rcd) demosaic() {
	w, h := r.w, r.h
	w1, w2, w3, w4 := w, 2*w, 3*w, 4*w
	cfa := r.cfa
	R, G, B := r.rgb[0], r.rgb[1], r.rgb[2]

	// The photosites as they are, in the channel of their colour.
	for y := range h {
		for x := range w {
			i := y*w + x
			r.rgb[r.at(x, y)][i] = cfa[i]
		}
	}

	// Step 1: which of vertical and horizontal is smoother, by the
	// squared high pass filter along each, summed over three.
	for y := 3; y < h-3; y++ {
		for x := 3; x < w-3; x++ {
			i := y*w + x
			r.hv[i] = sq((cfa[i-w3] - cfa[i-w1] - cfa[i+w1] + cfa[i+w3]) - 3*(cfa[i-w2]+cfa[i+w2]) + 6*cfa[i])
			r.hh[i] = sq((cfa[i-3] - cfa[i-1] - cfa[i+1] + cfa[i+3]) - 3*(cfa[i-2]+cfa[i+2]) + 6*cfa[i])
		}
	}
	for y := 4; y < h-4; y++ {
		for x := 4; x < w-4; x++ {
			i := y*w + x
			v := max(rcdEpsSq, r.hv[i-w1]+r.hv[i]+r.hv[i+w1])
			hz := max(rcdEpsSq, r.hh[i-1]+r.hh[i]+r.hh[i+1])
			r.vh[i] = v / (v + hz)
		}
	}

	// Step 2: the low pass filter at red and blue.
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			if r.at(x, y) == 1 {
				continue
			}
			i := y*w + x
			r.lpf[i] = cfa[i] + 0.5*(cfa[i-w1]+cfa[i+w1]+cfa[i-1]+cfa[i+1]) +
				0.25*(cfa[i-w1-1]+cfa[i-w1+1]+cfa[i+w1-1]+cfa[i+w1+1])
		}
	}

	// Step 3: green at red and blue.
	for y := 5; y < h-5; y++ {
		for x := 5; x < w-5; x++ {
			if r.at(x, y) == 1 {
				continue
			}
			i := y*w + x
			gN := rcdEps + abs32(cfa[i-w1]-cfa[i+w1]) + abs32(cfa[i]-cfa[i-w2]) + abs32(cfa[i-w1]-cfa[i-w3]) + abs32(cfa[i-w2]-cfa[i-w4])
			gS := rcdEps + abs32(cfa[i-w1]-cfa[i+w1]) + abs32(cfa[i]-cfa[i+w2]) + abs32(cfa[i+w1]-cfa[i+w3]) + abs32(cfa[i+w2]-cfa[i+w4])
			gW := rcdEps + abs32(cfa[i-1]-cfa[i+1]) + abs32(cfa[i]-cfa[i-2]) + abs32(cfa[i-1]-cfa[i-3]) + abs32(cfa[i-2]-cfa[i-4])
			gE := rcdEps + abs32(cfa[i-1]-cfa[i+1]) + abs32(cfa[i]-cfa[i+2]) + abs32(cfa[i+1]-cfa[i+3]) + abs32(cfa[i+2]-cfa[i+4])

			l := r.lpf[i]
			eN := cfa[i-w1] * (1 + (l-r.lpf[i-w2])/(rcdEps+l+r.lpf[i-w2]))
			eS := cfa[i+w1] * (1 + (l-r.lpf[i+w2])/(rcdEps+l+r.lpf[i+w2]))
			eW := cfa[i-1] * (1 + (l-r.lpf[i-2])/(rcdEps+l+r.lpf[i-2]))
			eE := cfa[i+1] * (1 + (l-r.lpf[i+2])/(rcdEps+l+r.lpf[i+2]))

			v := (gS*eN + gN*eS) / (gN + gS)
			hz := (gW*eE + gE*eW) / (gE + gW)
			d := r.discrimination(r.vh, i)
			G[i] = clamp01(d*hz + (1-d)*v)
		}
	}

	// Step 4.0 and 4.1: which diagonal is smoother, at red and blue.
	for y := 3; y < h-3; y++ {
		for x := 3; x < w-3; x++ {
			if r.at(x, y) == 1 {
				continue
			}
			i := y*w + x
			r.hv[i] = sq((cfa[i-w3-3] - cfa[i-w1-1] - cfa[i+w1+1] + cfa[i+w3+3]) - 3*(cfa[i-w2-2]+cfa[i+w2+2]) + 6*cfa[i])
			r.hh[i] = sq((cfa[i-w3+3] - cfa[i-w1+1] - cfa[i+w1-1] + cfa[i+w3-3]) - 3*(cfa[i-w2+2]+cfa[i+w2-2]) + 6*cfa[i])
		}
	}
	for y := 4; y < h-4; y++ {
		for x := 4; x < w-4; x++ {
			if r.at(x, y) == 1 {
				continue
			}
			i := y*w + x
			p := max(rcdEpsSq, r.hv[i-w1-1]+r.hv[i]+r.hv[i+w1+1])
			q := max(rcdEpsSq, r.hh[i-w1+1]+r.hh[i]+r.hh[i+w1-1])
			r.pq[i] = p / (p + q)
		}
	}

	// Step 4.2: red at blue and blue at red, from the diagonals.
	for y := 7; y < h-7; y++ {
		for x := 7; x < w-7; x++ {
			own := r.at(x, y)
			if own == 1 {
				continue
			}
			i := y*w + x
			X := r.rgb[2-own]
			d := r.discrimination(r.pq, i)
			gNW := rcdEps + abs32(X[i-w1-1]-X[i+w1+1]) + abs32(X[i-w1-1]-X[i-w3-3]) + abs32(G[i]-G[i-w2-2])
			gNE := rcdEps + abs32(X[i-w1+1]-X[i+w1-1]) + abs32(X[i-w1+1]-X[i-w3+3]) + abs32(G[i]-G[i-w2+2])
			gSW := rcdEps + abs32(X[i+w1-1]-X[i-w1+1]) + abs32(X[i+w1-1]-X[i+w3-3]) + abs32(G[i]-G[i+w2-2])
			gSE := rcdEps + abs32(X[i+w1+1]-X[i-w1-1]) + abs32(X[i+w1+1]-X[i+w3+3]) + abs32(G[i]-G[i+w2+2])
			eNW := X[i-w1-1] - G[i-w1-1]
			eNE := X[i-w1+1] - G[i-w1+1]
			eSW := X[i+w1-1] - G[i+w1-1]
			eSE := X[i+w1+1] - G[i+w1+1]
			p := (gNW*eSE + gSE*eNW) / (gNW + gSE)
			q := (gNE*eSW + gSW*eNE) / (gNE + gSW)
			X[i] = clamp01(G[i] + (1-d)*p + d*q)
		}
	}

	// Step 4.3: red and blue at green, from rows and columns.
	for y := 10; y < h-10; y++ {
		for x := 10; x < w-10; x++ {
			if r.at(x, y) != 1 {
				continue
			}
			i := y*w + x
			d := r.discrimination(r.vh, i)
			for _, X := range [2][]float32{R, B} {
				gN := rcdEps + abs32(G[i]-G[i-w2]) + abs32(X[i-w1]-X[i+w1]) + abs32(X[i-w1]-X[i-w3])
				gS := rcdEps + abs32(G[i]-G[i+w2]) + abs32(X[i+w1]-X[i-w1]) + abs32(X[i+w1]-X[i+w3])
				gW := rcdEps + abs32(G[i]-G[i-2]) + abs32(X[i-1]-X[i+1]) + abs32(X[i-1]-X[i-3])
				gE := rcdEps + abs32(G[i]-G[i+2]) + abs32(X[i+1]-X[i-1]) + abs32(X[i+1]-X[i+3])
				eN := X[i-w1] - G[i-w1]
				eS := X[i+w1] - G[i+w1]
				eW := X[i-1] - G[i-1]
				eE := X[i+1] - G[i+1]
				v := (gN*eS + gS*eN) / (gN + gS)
				hz := (gE*eW + gW*eE) / (gE + gW)
				X[i] = clamp01(G[i] + (1-d)*v + d*hz)
			}
		}
	}
}

// discrimination is the direction weight at i: its own, or that of its
// four diagonal neighbours on average when they are surer, further from
// one half.
func (r *rcd) discrimination(dir []float32, i int) float32 {
	w := r.w
	c := dir[i]
	n := 0.25 * (dir[i-w-1] + dir[i-w+1] + dir[i+w-1] + dir[i+w+1])
	if abs32(0.5-c) < abs32(0.5-n) {
		return n
	}
	return c
}
