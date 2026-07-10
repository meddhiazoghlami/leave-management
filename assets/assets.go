// Package assets bridges Go/templ to the Vite-built frontend assets.
//
// Two modes:
//   - Dev  (VITE_DEV=true): the templ layout points at the Vite dev server
//     (http://localhost:5173) so we get Hot Module Replacement.
//   - Prod (default): we read Vite's manifest.json once at startup and emit the
//     hashed, self-hosted files that `vite build` produced into public/build/.
package assets

import (
	"encoding/json"
	"os"
)

const (
	devServer = "http://localhost:5173"
	// Vite (v5+) writes the manifest here, relative to the outDir in vite.config.js.
	manifestPath = "public/build/.vite/manifest.json"
)

// Dev toggles dev-server mode (HMR) vs reading the built manifest.
var Dev bool

// manifestEntry is one record in Vite's manifest.json. We only need file + css.
type manifestEntry struct {
	File string   `json:"file"`
	CSS  []string `json:"css"`
}

var manifest map[string]manifestEntry

// Init selects the mode. In prod it loads the Vite manifest so we can map an
// entry point (e.g. "src/app.js") to its hashed output file(s).
func Init(dev bool) error {
	Dev = dev
	if dev {
		return nil
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &manifest)
}

// Client is the Vite HMR client script. Dev only — omit it in prod.
func Client() string { return devServer + "/@vite/client" }

// Entry returns the <script> src for an entry point.
//
//	dev:  the dev server serves the source module directly (with HMR)
//	prod: the hashed, bundled file recorded in the manifest
func Entry(entry string) string {
	if Dev {
		return devServer + "/" + entry
	}
	return "/build/" + manifest[entry].File
}

// Styles returns the stylesheet hrefs for an entry point.
//
//	dev:  none — Vite injects CSS via JS so it can hot-reload
//	prod: the hashed CSS file(s) the entry imported
func Styles(entry string) []string {
	if Dev {
		return nil
	}
	hrefs := make([]string, 0, len(manifest[entry].CSS))
	for _, css := range manifest[entry].CSS {
		hrefs = append(hrefs, "/build/"+css)
	}
	return hrefs
}
