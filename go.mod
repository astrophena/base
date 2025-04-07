module go.astrophena.name/base

go 1.23.0

toolchain go1.24.1

require (
	github.com/benbjohnson/hashfs v0.2.2
	github.com/google/go-cmp v0.7.0
	github.com/tailscale/tscert v0.0.0-20240608151842-d3f834017e53
	golang.org/x/crypto v0.37.0
)

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	golang.org/x/tools v0.31.0 // indirect
	honnef.co/go/tools v0.6.0 // indirect
)

tool go.astrophena.name/base/internal/devtools/addcopyright

tool honnef.co/go/tools/cmd/staticcheck
