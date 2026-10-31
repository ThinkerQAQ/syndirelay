package extension

import "embed"

// Shared UI assets are built into the native Bridge binary. The browser
// extension and Go console load the exact same Feature HTML/JS/CSS.
//
//go:embed popup/*.html popup/*.js popup/*.css icons/*.png
var Assets embed.FS
