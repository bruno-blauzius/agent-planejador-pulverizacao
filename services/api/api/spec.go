// Package apispec embeds the OpenAPI contract so the binary serves the exact
// document that is versioned alongside the code.
package apispec

import _ "embed"

// Spec holds the raw OpenAPI document (api/openapi.yaml) embedded at build time.
//
//go:embed openapi.yaml
var Spec []byte
