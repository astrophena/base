module go.astrophena.name/base

go 1.23.0

toolchain go1.24.1

require (
	github.com/benbjohnson/hashfs v0.2.2
	github.com/google/go-cmp v0.7.0
	golang.org/x/crypto v0.36.0
)

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.23.0 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/tools v0.30.0 // indirect
	honnef.co/go/tools v0.6.0 // indirect
)

tool go.astrophena.name/base/internal/devtools/addcopyright

tool honnef.co/go/tools/cmd/staticcheck
