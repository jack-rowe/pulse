package api

import (
	_ "embed"
)

// statusPageHTML is the embedded status page served at /.
//
//go:embed status_page.html
var statusPageHTML string
