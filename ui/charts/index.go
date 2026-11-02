package charts

import "sort"

// leaf is the number of points a leaf of the index summarises. The ends of
// a range are scanned point by point, at most two leaves; everything between
// them is answered by the tree.
const leaf = 64

// index answers, for any range of a series, which point has the smallest
// and which the largest value, and where its missing values are: everything
// [M4] needs of a pixel column, in O(log n) instead of a pass over the
// column.
//
// It is a segment tree over leaves of [leaf] points. Node k has the
// children 2k and 2k+1, the leaves are the nodes from size on, and every
// node holds the index of the minimum and of the maximum of its points, or
// -1 when all of them are missing. The missing values are kept apart, as
// the ascending list of their indices, since a range asks for the first and
// the last of them.
type index struct {
	size     int
	min, max []int32
	nans     []int32
}

// build indexes ys from scratch.
func (ix *index) build(ys []float64) {
	nl := (len(ys) + leaf - 1) / leaf
	size := 1
	for size < nl {
		size *= 2
	}
	ix.size = size
	ix.min = fill(ix.min, 2*size)
	ix.max = fill(ix.max, 2*size)
	ix.nans = ix.nans[:0]
	for i, y := range ys {
		if Missing(y) {
			ix.nans = append(ix.nans, int32(i))
			continue
		}
		k := size + i/leaf
		ix.min[k] = lower(ys, ix.min[k], int32(i))
		ix.max[k] = upper(ys, ix.max[k], int32(i))
	}
	for k := size - 1; k >= 1; k-- {
		ix.min[k] = lower(ys, ix.min[2*k], ix.min[2*k+1])
		ix.max[k] = upper(ys, ix.max[2*k], ix.max[2*k+1])
	}
}

// fill returns a slice of n entries of -1, reusing s.
func fill(s []int32, n int) []int32 {
	if cap(s) < n {
		s = make([]int32, n)
	}
	s = s[:n]
	for i := range s {
		s[i] = -1
	}
	return s
}

// push indexes ys[i], the point just appended.
func (ix *index) push(ys []float64, i int) {
	if Missing(ys[i]) {
		ix.nans = append(ix.nans, int32(i))
		return
	}
	b := i / leaf
	if b >= ix.size {
		ix.build(ys)
		return
	}
	k := ix.size + b
	ix.min[k] = lower(ys, ix.min[k], int32(i))
	ix.max[k] = upper(ys, ix.max[k], int32(i))
	for k /= 2; k >= 1; k /= 2 {
		ix.min[k] = lower(ys, ix.min[2*k], ix.min[2*k+1])
		ix.max[k] = upper(ys, ix.max[2*k], ix.max[2*k+1])
	}
}

// lower returns the one of a and b with the smaller value, the earlier one
// of two equal values, and the other one when one of them is -1.
func lower(ys []float64, a, b int32) int32 {
	switch {
	case a < 0:
		return b
	case b < 0:
		return a
	case ys[b] < ys[a] || ys[b] == ys[a] && b < a:
		return b
	}
	return a
}

// upper is [lower] for the larger value.
func upper(ys []float64, a, b int32) int32 {
	switch {
	case a < 0:
		return b
	case b < 0:
		return a
	case ys[b] > ys[a] || ys[b] == ys[a] && b < a:
		return b
	}
	return a
}

// extremes returns the indices of the smallest and the largest value in
// ys[i0:i1], leaving out missing values, and -1 for both when there is no
// value.
func (ix *index) extremes(ys []float64, i0, i1 int) (lo, hi int) {
	l, h := int32(-1), int32(-1)
	scan := func(from, to int) {
		for i := from; i < to; i++ {
			if !Missing(ys[i]) {
				l, h = lower(ys, l, int32(i)), upper(ys, h, int32(i))
			}
		}
	}
	b0, b1 := (i0+leaf-1)/leaf, i1/leaf
	if b0 >= b1 {
		scan(i0, i1)
		return int(l), int(h)
	}
	scan(i0, b0*leaf)
	scan(b1*leaf, i1)
	for a, b := b0+ix.size, b1+ix.size; a < b; a, b = a/2, b/2 {
		if a&1 == 1 {
			l, h = lower(ys, l, ix.min[a]), upper(ys, h, ix.max[a])
			a++
		}
		if b&1 == 1 {
			b--
			l, h = lower(ys, l, ix.min[b]), upper(ys, h, ix.max[b])
		}
	}
	return int(l), int(h)
}

// missing returns the first and the last missing value in [i0, i1), and
// false when there is none.
func (ix *index) missing(i0, i1 int) (first, last int, ok bool) {
	a := sort.Search(len(ix.nans), func(k int) bool { return int(ix.nans[k]) >= i0 })
	if a == len(ix.nans) || int(ix.nans[a]) >= i1 {
		return 0, 0, false
	}
	b := sort.Search(len(ix.nans), func(k int) bool { return int(ix.nans[k]) >= i1 })
	return int(ix.nans[a]), int(ix.nans[b-1]), true
}
