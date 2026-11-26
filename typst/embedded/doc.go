// Package embedded carries libgifttypst inside the program, so that package
// typst works without a library installed next to it. Import it for its side
// effect:
//
//	import _ "github.com/worldiety/gift/typst/embedded"
//
// # What it costs
//
// The library for the platform being built for is embedded, compressed with
// gzip: about 13 MB of the binary, 34 to 36 MB once unpacked. Only that one
// platform's library is linked in. A program that does not import this
// package pays nothing; package typst embeds nothing itself.
//
// The library is unpacked the first time package typst is used, into the
// user's cache directory — ~/Library/Caches/gift/typst on macOS,
// ~/.cache/gift/typst on Linux — in a directory named after its checksum,
// and loaded from there. That takes a fraction of a second once; later runs
// find it and load it at once. A program built with another revision of gift
// unpacks its own library next to the old one.
//
// # Platforms
//
// macOS 11 and later on arm64, and Linux on amd64 and arm64 with glibc 2.28
// or later: Debian 10, Ubuntu 20.04, RHEL 8 and later, and every Raspberry
// Pi OS of 64 bits. On any other platform this package is empty, and package
// typst finds a library only where it looks by itself. iOS is one of them,
// although Go builds a macOS file for it: an iPhone application loads no
// library of a Mac, and none from its cache directory.
//
// # Precedence
//
// A library named with typst.Open or GIFT_TYPST_LIB is preferred to the one
// embedded here, so that a library just built from typst/ffi can be tried
// without rebuilding the program.
//
// A macOS application that is signed with the hardened runtime loads only
// libraries signed by its own team, which the unpacked library is not. Such
// an application ships the library signed inside its bundle and names it
// with typst.Open instead.
//
// The libraries are built by typst/ffi/build.sh.
package embedded
