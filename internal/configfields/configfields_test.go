package configfields

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Kevinbarrero/mr-impact/internal/load"
)

func TestUnused(t *testing.T) {
	p, err := load.Load(context.Background(), "testdata/cfg", nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{"Port": true, "OldKey": true, "Debug": true, "Written": true, "Plain": true, "JSON": true}
	var got []string
	for _, f := range Unused(p.Packages, p.Rel, func(_, n string) bool { return names[n] }) {
		got = append(got, fmt.Sprintf("%s %s %s:%d test=%v", f.Display, f.Tag, f.File, f.Line, f.TestOnly))
	}
	want := strings.Join([]string{
		`conf.Config.OldKey envconfig:"OLD_KEY" conf/conf.go:6 test=false`,
		`conf.Config.Debug yaml:"debug" conf/conf.go:7 test=true`,
	}, "\n")
	if strings.Join(got, "\n") != want {
		t.Errorf("Unused =\n%s\nwant\n%s", strings.Join(got, "\n"), want)
	}
	if len(Unused(p.Packages, p.Rel, func(_, n string) bool { return n == "Port" })) != 0 {
		t.Error("a name not in names must not be reported, a read field never")
	}
}
