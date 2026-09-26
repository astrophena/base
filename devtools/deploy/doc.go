// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

/*
Deploy sends site, service, and artifact releases to deployd.

# Usage

Upload a site or service archive:

	$ go tool deploy -type site astrophena.name archive.tar.gz
	$ go tool deploy -type service payday archive.tar.gz

Publish signed artifacts:

	$ go tool deploy -type artifact dungeon kernel initrd.cpio rootfs.erofs

Artifact uploads use content-defined chunks by default. Deployd reuses chunks
it already has. Use -artifact-upload-mode=fixed for the older fixed-size format.

Artifact release IDs default to the current UTC time in YYYYMMDDHHMMSS format.
Use -artifact-release-id to reuse an ID when retrying a release.

# Environment

GitHub Actions must provide:

  - ACTIONS_ID_TOKEN_REQUEST_URL: OIDC token request URL.
  - ACTIONS_ID_TOKEN_REQUEST_TOKEN: token for that request.

Artifact deployments also need an Ed25519 private key in
DEPLOY_ARTIFACT_SIGNING_KEY. Use -artifact-signing-key-env to read another
variable. Accepted formats are:

  - PKCS#8 PEM.
  - base64 private key bytes.
  - base64 or hex seed bytes.
*/
package main

import (
	_ "embed"

	"go.astrophena.name/base/cli"
)

//go:embed doc.go
var doc []byte

func init() { cli.SetDocComment(doc) }
