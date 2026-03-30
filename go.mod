module go.astrophena.name/base

go 1.26.0

require (
	github.com/a-h/templ v0.3.1001
	github.com/go4org/hashtriemap v0.0.0-20251130024219-545ba229f689
	github.com/lmittmann/tint v1.1.3
	golang.org/x/sync v0.20.0
	golang.org/x/term v0.41.0
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/a-h/parse v0.0.0-20250122154542-74294addb73e // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cli/browser v1.3.0 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/natefinch/atomic v1.0.1 // indirect
	golang.org/x/exp/typeparams v0.0.0-20251219203646-944ab1f22d93 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	golang.org/x/tools/go/expect v0.1.1-deprecated // indirect
	honnef.co/go/tools v0.6.1 // indirect
)

tool (
	go.astrophena.name/base/devtools/addcopyright
	go.astrophena.name/base/devtools/pre-commit
)

tool honnef.co/go/tools/cmd/staticcheck

tool github.com/a-h/templ/cmd/templ
