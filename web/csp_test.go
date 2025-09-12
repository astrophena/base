// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"testing"

	"go.astrophena.name/base/testutil"
)

func TestCSP_String(t *testing.T) {
	cases := map[string]struct {
		policy CSP
		want   string
	}{
		"empty": {
			policy: CSP{},
			want:   "",
		},
		"default": {
			policy: defaultCSP,
			want:   "base-uri 'self'; block-all-mixed-content; default-src 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'self'; script-src 'self'",
		},
		"custom": {
			policy: CSP{
				DefaultSrc:              []string{CSPSelf},
				ScriptSrc:               []string{CSPSelf, "https://example.com"},
				StyleSrc:                []string{CSPNone},
				UpgradeInsecureRequests: true,
			},
			want: "default-src 'self'; script-src 'self' https://example.com; style-src 'none'; upgrade-insecure-requests",
		},
		"boolean only": {
			policy: CSP{
				BlockAllMixedContent: true,
			},
			want: "block-all-mixed-content",
		},
		"slice only": {
			policy: CSP{
				DefaultSrc: []string{CSPSelf},
			},
			want: "default-src 'self'",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := tc.policy.String()
			testutil.AssertEqual(t, got, tc.want)
		})
	}
}

func TestCSP_Finalize(t *testing.T) {
	p := CSP{
		DefaultSrc: []string{CSPSelf},
		ScriptSrc:  []string{CSPSelf, "https://example.com"},
	}

	if p.str != nil {
		t.Fatal("p.str should be nil before Finalize")
	}

	finalized := p.Finalize()

	if finalized.str == nil {
		t.Fatal("finalized.str should not be nil after Finalize")
	}

	want := "default-src 'self'; script-src 'self' https://example.com"
	gotString := finalized.String()
	testutil.AssertEqual(t, gotString, want)

	if p.str != nil {
		t.Fatal("original p.str should still be nil")
	}

	gotString = p.String()
	testutil.AssertEqual(t, gotString, want)
}

func TestCSPMux_HandlePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Handle did not panic when registering a duplicate pattern")
		}
	}()

	mux := NewCSPMux()
	mux.Handle("/", CSP{})
	mux.Handle("/", CSP{})
}
