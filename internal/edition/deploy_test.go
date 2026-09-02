package edition

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The unit file, installer, and binary all use one product name. Nothing links the
// deployment files at compile time, so this test does.
func TestUnitFileMatchesProductName(t *testing.T) {
	path := filepath.Join("..", "..", "deploy", Name+".service")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("product name is %q but %s is missing: %v", Name, path, err)
	}
	unit := string(data)
	for _, want := range []string{
		"ExecStart=/usr/bin/env -- /usr/local/bin/" + Name + " ",
		"--config /etc/" + Name + "/config.json",
		"EnvironmentFile=/etc/" + Name + "/bot.env",
		"StateDirectory=" + Name + "\n",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("%s does not contain %q", path, want)
		}
	}
}

func TestInstallerOffersOnlyProduct(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	installer := string(data)
	if !strings.Contains(installer, "name="+Name) {
		t.Errorf("deploy/install.sh cannot install %q", Name)
	}
	if strings.Contains(installer, "--generic") || strings.Contains(installer, "gentoo-zhbot") {
		t.Error("deploy/install.sh still exposes the removed build editions")
	}
}
