module github.com/lesf0/yson-tools/ysonq

go 1.24.0

toolchain go1.24.6

require go.ytsaurus.tech/yt/go v0.0.26

require (
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	golang.org/x/sys v0.38.0 // indirect
)

require (
	github.com/itchyny/gojq v0.12.19
	github.com/lesf0/yson-tools/pretty-formatter v0.0.0
	github.com/lesf0/yson-tools/ysonlib v0.0.0
	go.ytsaurus.tech/library/go/core/xerrors v0.0.4 // indirect
	go.ytsaurus.tech/library/go/x/xreflect v0.0.3 // indirect
	go.ytsaurus.tech/library/go/x/xruntime v0.0.4 // indirect
	golang.org/x/term v0.35.0
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
)

// during development the tools are built from the tree they live in; a release
// pins these to the pseudo-versions of the pushed commits instead
replace github.com/lesf0/yson-tools/pretty-formatter => ../pretty-formatter

replace github.com/lesf0/yson-tools/ysonlib => ../ysonlib
