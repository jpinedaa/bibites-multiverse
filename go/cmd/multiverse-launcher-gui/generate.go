//go:build generate

// THE RESOURCE OBJECT, AND WHY IT IS GENERATED RATHER THAN COMMITTED.
//
// A Win32 program needs two things out of a resource section, and walk needs the
// first of them to run at all:
//
//   - THE COMMON CONTROLS 6 DEPENDENCY. Without it the process gets the version 5
//     controls, and a list view, a splitter and a themed button are not what the
//     rest of Windows looks like. The manifest beside this file declares it.
//   - THE ICON, so the taskbar, Alt-Tab and Explorer show this program's own icon
//     rather than the Go gopher. It is the same bibites-multiverse.ico the setup,
//     the shortcuts and the Add/Remove Programs entry use — one icon, one file.
//
// The manifest also declares per-monitor DPI awareness, so the window is drawn
// sharp on a scaled display instead of being bitmap-stretched by Windows.
//
// The .syso this produces is BUILD OUTPUT, and release/tracked-binaries.txt says
// what this repository is allowed to track: documentation imagery, one icon and
// test fixtures, and explicitly "no build output". So it is generated — here by
// hand, in release/make-release.sh for a release, and in CI before the
// cross-build — and it is in .gitignore. rsrc is pinned in go.mod as a tool
// dependency, so the generation needs no network and no installed program, and
// its output is byte-for-byte the same from the same manifest and icon.
//
// Run it from the go/ directory:
//
//	go generate ./cmd/multiverse-launcher-gui
package main

//go:generate go tool rsrc -manifest app.manifest -ico ../../../release/kit/bibites-multiverse.ico -arch amd64 -o rsrc_windows_amd64.syso
