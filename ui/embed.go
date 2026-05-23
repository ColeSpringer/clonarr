package ui

import "embed"

// all: prefix is REQUIRED — without it Go's embed silently skips files
// whose name starts with `_` or `.`, which masquerades as missing-
// template runtime errors. See the _cf-row.html incident: the file
// shipped fine in source control + the docker image, but ParseFS at
// startup didn't see it, so {{template "sections/_cf-row" .}} blew up
// at first request with a 0-byte 200. Keep the `all:` prefix.
//
//go:embed all:static
var StaticFiles embed.FS
