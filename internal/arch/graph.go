package arch

import (
	"go/types"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// FromPackages builds the module-internal import graph of the non-test
// packages; rel maps an absolute file to its repo-relative path (load's rel);
// one edge per (From, To) at the first file in name order importing To, at
// that import spec's line. Named counts every *types.TypeName in the package
// scope (exported or not, aliases included).
func FromPackages(pkgs []*packages.Package, module string, rel func(string) (string, bool)) Graph {
	g := Graph{
		Module:     module,
		Named:      map[string]int{},
		Interfaces: map[string]int{},
		Names:      map[string]string{},
	}
	inModule := func(path string) bool {
		return path == module || strings.HasPrefix(path, module+"/")
	}
	seen := map[string]bool{}
	var kept []*packages.Package
	for _, p := range pkgs {
		if strings.HasSuffix(p.ID, ".test]") || strings.HasSuffix(p.Name, "_test") ||
			strings.HasSuffix(p.PkgPath, ".test") || !inModule(p.PkgPath) || seen[p.PkgPath] {
			continue
		}
		seen[p.PkgPath] = true
		kept = append(kept, p)
		g.Packages = append(g.Packages, p.PkgPath)
		g.Names[p.PkgPath] = p.Name
		if p.Types != nil {
			sc := p.Types.Scope()
			for _, n := range sc.Names() {
				tn, ok := sc.Lookup(n).(*types.TypeName)
				if !ok {
					continue
				}
				g.Named[p.PkgPath]++
				if _, ok := tn.Type().Underlying().(*types.Interface); ok {
					g.Interfaces[p.PkgPath]++
				}
			}
		}
	}
	sort.Strings(g.Packages)

	for _, p := range kept {
		type fileSyntax struct {
			name string
			idx  int
		}
		files := make([]fileSyntax, 0, len(p.Syntax))
		for i, f := range p.Syntax {
			abs := p.Fset.Position(f.Package).Filename
			name := abs
			if r, ok := rel(abs); ok {
				name = r
			}
			files = append(files, fileSyntax{name, i})
		}
		sort.SliceStable(files, func(i, j int) bool { return files[i].name < files[j].name })
		done := map[string]bool{}
		for _, fs := range files {
			f := p.Syntax[fs.idx]
			for _, spec := range f.Imports {
				to, err := strconv.Unquote(spec.Path.Value)
				if err != nil || to == p.PkgPath || !seen[to] || done[to] {
					continue
				}
				done[to] = true
				g.Edges = append(g.Edges, Edge{From: p.PkgPath, To: to, File: fs.name, Line: p.Fset.Position(spec.Pos()).Line})
			}
		}
	}
	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].From != g.Edges[j].From {
			return g.Edges[i].From < g.Edges[j].From
		}
		return g.Edges[i].To < g.Edges[j].To
	})
	return g
}
