module go.astrophena.name/base

go 1.23.0

toolchain go1.24.1

require (
	github.com/benbjohnson/hashfs v0.2.2
	github.com/google/go-cmp v0.7.0
)

tool (
	go.astrophena.name/base/internal/devtools/addcopyright
	go.astrophena.name/base/internal/devtools/pre-commit
)
