package raw

import "fmt"

// readDNGMosaic decodes the raw data of a DNG.
func (f *File) readDNGMosaic() ([]uint16, error) {
	return nil, fmt.Errorf("%w: DNG data", ErrUnsupported)
}
