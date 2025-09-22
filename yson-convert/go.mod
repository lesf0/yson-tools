module github.com/lesf0/yson-tools/yson-convert

go 1.24.0

toolchain go1.24.6

require go.ytsaurus.tech/yt/go v0.0.26

require golang.org/x/sys v0.36.0 // indirect

require (
	github.com/andrew-d/go-termutil v0.0.0-20150726205930-009166a695a2
	github.com/lesf0/yson-tools/yson-convert/pretty-formatter v0.0.0-00010101000000-000000000000
	go.ytsaurus.tech/library/go/core/xerrors v0.0.4 // indirect
	go.ytsaurus.tech/library/go/x/xreflect v0.0.3 // indirect
	go.ytsaurus.tech/library/go/x/xruntime v0.0.4 // indirect
	golang.org/x/term v0.35.0
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
)

replace github.com/lesf0/yson-tools/yson-convert/pretty-formatter => ./pretty-formatter
