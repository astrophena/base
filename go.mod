module go.astrophena.name/base

go 1.25

require (
	github.com/lmittmann/tint v1.1.2
	golang.org/x/term v0.36.0
)

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/tools v0.31.0 // indirect
	honnef.co/go/tools v0.6.0 // indirect
)

tool (
	go.astrophena.name/base/devtools/addcopyright
	go.astrophena.name/base/devtools/pre-commit
)

tool honnef.co/go/tools/cmd/staticcheck
