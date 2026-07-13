package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevMode(t *testing.T) {
	if err := Init(true); err != nil {
		t.Fatalf("Init(dev): %v", err)
	}
	if !Dev {
		t.Fatal("Dev should be true after Init(true)")
	}
	if Client() != devServer+"/@vite/client" {
		t.Errorf("Client() = %q", Client())
	}
	if got := Entry("src/app.js"); got != devServer+"/src/app.js" {
		t.Errorf("dev Entry = %q", got)
	}
	if Styles("src/app.js") != nil {
		t.Error("dev Styles should be nil (Vite injects CSS via JS)")
	}
}

func TestProdMode(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	manifestDir := filepath.Join(dir, "public", "build", ".vite")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"src/app.js":{"file":"assets/app-abc123.js","css":["assets/app-abc123.css"]}}`
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Init(false); err != nil {
		t.Fatalf("Init(prod): %v", err)
	}
	if Dev {
		t.Fatal("Dev should be false after Init(false)")
	}
	if got := Entry("src/app.js"); got != "/build/assets/app-abc123.js" {
		t.Errorf("prod Entry = %q", got)
	}
	styles := Styles("src/app.js")
	if len(styles) != 1 || styles[0] != "/build/assets/app-abc123.css" {
		t.Errorf("prod Styles = %v", styles)
	}
}

func TestProdMode_MissingManifest(t *testing.T) {
	t.Chdir(t.TempDir()) // no public/build/.vite/manifest.json here
	if err := Init(false); err == nil {
		t.Fatal("Init(false) should error when the manifest is missing")
	}
}
