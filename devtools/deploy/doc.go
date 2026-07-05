// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

/*
Deploy sends deployment artifacts to deployd.

It supports two deployment shapes:

  - site and service deployments upload one multipart archive to /site or
    /service.
  - artifact deployments publish one or more large files through deployd's
    chunked /artifact API, with a signed manifest for client-side verification.

How this works (private links):

  - https://github.com/astrophena/infra/tree/master/services/deployd

# Usage

Deploy a site or service archive:

	$ go tool deploy -type site astrophena.name archive.tar.gz
	$ go tool deploy -type service payday archive.tar.gz

Publish an artifact bundle:

	$ go tool deploy -type artifact dungeon kernel initrd.cpio rootfs.erofs

Artifact uploads default to content-defined chunks so retries and later releases
send only chunks deployd does not already have. The signed manifest records each
file's size, SHA-256, chunking contract, and chunk SHA-256 values. Use
-artifact-upload-mode=fixed for the legacy fixed-size upload protocol.

Artifact release IDs default to the current UTC timestamp in deployd's sortable
release format, YYYYMMDDHHMMSS. Use -artifact-release-id to provide one
explicitly, for example when retrying a workflow run.

# Environment Variables

This tool requires the following environment variables to be set by the
GitHub Actions runner:

  - ACTIONS_ID_TOKEN_REQUEST_URL: The URL to request the OIDC token from.
  - ACTIONS_ID_TOKEN_REQUEST_TOKEN: The bearer token for authenticating the
    OIDC token request.

Artifact deployments also require an Ed25519 private key. By default the tool
reads DEPLOY_ARTIFACT_SIGNING_KEY; override the variable name with
-artifact-signing-key-env. The key may be one of:

  - PKCS#8 PEM, such as output from openssl genpkey -algorithm Ed25519.
  - base64 raw Ed25519 private key bytes.
  - base64 or hex Ed25519 seed bytes.
*/
package main

import (
	_ "embed"

	"go.astrophena.name/base/cli"
)

//go:embed doc.go
var doc []byte

func init() { cli.SetDocComment(doc) }
