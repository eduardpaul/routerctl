package asusrouter_test

// Import rules between the layers (docs/development.md, "Import rules"). This file
// is the same in the four modules of the workspace (netkit, asusrouter,
// routerkit, routerctl) apart from the package clause, and
// TestArchCopiesInSync keeps the copies identical: each copy holds every
// rule and checks the packages of its own module. Every package must match
// a rule; a rule lists the repo packages it may import. Paths are relative
// to repoBase, so they cross module boundaries. The stdlib is always
// allowed, external modules too except where a rule says stdlib only.
// Known violations are listed in allowedEdges / allowedExtra with the
// change that removes them; an entry that no longer matches anything fails
// the test, so the lists only shrink.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// repoBase is the import path every module of the workspace lives under.
const repoBase = "github.com/eduardpaul/routerctl"

// archModules are the modules of the workspace, with the minimum number
// of packages `go list` must find in each (a guard against a listing that
// silently comes back short).
var archModules = map[string]int{
	"netkit":     3,
	"asusrouter": 80,
	"routerkit":  9,
	"routerctl":  12,
}

// archPkg is one package of the module, with repo-relative imports.
type archPkg struct {
	rel         string   // path relative to repoBase ("routerkit/detect")
	dir         string   // directory on disk
	imports     []string // production imports in the repo
	testImports []string // test-only imports in the repo
	external    []string // production imports from other modules
	goFiles     []string // file names (production and test)
}

// archModule returns the module of the test (its root is the working
// directory) as a path relative to repoBase, and its directory.
func archModule(t *testing.T) (rel, dir string) {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", "{{.Module.Path}}|{{.Module.Dir}}", ".").Output()
	if err != nil {
		t.Fatalf("go list .: %v", err)
	}
	path, dir, _ := strings.Cut(strings.TrimSpace(string(out)), "|")
	rel, ok := strings.CutPrefix(path, repoBase+"/")
	if _, known := archModules[rel]; !ok || !known {
		t.Fatalf("module %s is not one of the workspace modules under %s", path, repoBase)
	}
	return rel, dir
}

func listModule(t *testing.T) []archPkg {
	t.Helper()
	modRel, modDir := archModule(t)
	cmd := exec.Command("go", "list", "-f",
		`{{.ImportPath}}|{{.Dir}}|{{join .Imports " "}}|{{join .TestImports " "}} {{join .XTestImports " "}}|{{join .GoFiles " "}} {{join .TestGoFiles " "}} {{join .XTestGoFiles " "}}`,
		"./...")
	cmd.Dir = modDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./...: %v", err)
	}
	rel := func(p string) string { return strings.TrimPrefix(p, repoBase+"/") }
	inRepo := func(p string) bool { return strings.HasPrefix(p, repoBase+"/") }
	isExternal := func(p string) bool { return strings.Contains(strings.SplitN(p, "/", 2)[0], ".") }
	var pkgs []archPkg
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, "|")
		p := archPkg{rel: rel(parts[0]), dir: parts[1], goFiles: strings.Fields(parts[4])}
		if !under(p.rel, modRel) {
			t.Fatalf("go list ./... in %s returned %s, outside the module", modDir, parts[0])
		}
		prod := map[string]bool{}
		for _, imp := range strings.Fields(parts[2]) {
			prod[imp] = true
			switch {
			case inRepo(imp):
				p.imports = append(p.imports, rel(imp))
			case isExternal(imp):
				p.external = append(p.external, imp)
			}
		}
		seen := map[string]bool{}
		for _, imp := range strings.Fields(parts[3]) {
			if prod[imp] || seen[imp] || imp == parts[0] || !inRepo(imp) {
				continue
			}
			seen[imp] = true
			p.testImports = append(p.testImports, rel(imp))
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs) < archModules[modRel] {
		t.Fatalf("go list found only %d packages in %s", len(pkgs), modRel)
	}
	return pkgs
}

// under reports whether rel is the package prefix or below it.
func under(rel, prefix string) bool {
	return rel == prefix || strings.HasPrefix(rel, prefix+"/")
}

// Allow patterns: "x" allows x and everything below it, "=x" allows x
// only, "$self" the importer's own unit (brands/<b>, drivers/<d>) and
// "$brand" any brand library (asusrouter or routerkit/brands/<b>; a
// driver may use only one, checked separately).
const (
	core       = "=routerkit"
	detect     = "routerkit/detect"
	proto      = "routerkit/proto"
	brands     = "routerkit/brands"
	drivers    = "routerkit/drivers"
	fake       = "routerkit/drivers/fake"
	drivertest = "routerkit/drivertest"
	netkit     = "netkit"
	asus       = "asusrouter"
	service    = "routerctl/internal/service"
	sinks      = "routerctl/internal/sink"
	config     = "routerctl/internal/config"
	app        = "routerctl/internal"
	cmd        = "routerctl/cmd"
)

type rule struct {
	from      string // package prefix; "=x" matches x only. The longest match wins.
	allow     []string
	testAllow []string // extra imports allowed in tests only
	stdOnly   bool     // no external modules either
}

var rules = []rule{
	{from: "=routerctl", allow: nil}, // the arch test of the routerctl module
	{from: netkit, allow: []string{netkit}, stdOnly: true},
	{from: asus, allow: []string{asus, proto, netkit}},
	{from: core, allow: nil, stdOnly: true},
	{from: detect, allow: []string{core}, testAllow: []string{drivers}}, // the cross-brand matrix
	{from: drivertest, allow: []string{core, fake}},
	{from: proto, allow: []string{proto, netkit}},
	{from: brands, allow: []string{"$self", proto, netkit}},
	{from: drivers, allow: []string{core, detect, "$self", "$brand", proto, netkit}, testAllow: []string{drivertest}},
	{from: service, allow: []string{core, service}, testAllow: []string{fake}},
	{from: sinks, allow: []string{core, service, sinks}, testAllow: []string{fake}},
	{from: config, allow: []string{core}},
	{from: app, allow: []string{core, detect, drivers, app}, testAllow: []string{fake}},
	{from: cmd, allow: []string{core, detect, drivers, app}},
}

// driverComposition lists the allowed driver-to-driver edges (a brand
// driver embedding a family driver, e.g. glinet -> openwrt), besides a
// driver's own subtree and the fake.
var driverComposition = map[string][]string{}

// configImporters may import routerctl/internal/config (docs/development.md, "Import rules").
var configImporters = []string{cmd, "routerctl/internal/app", config}

// allowedEdges are known violations ("from -> to") and the change that
// removes them.
var allowedEdges = map[string]string{}

// allowedExtra lists the files outside the brand libraries and drivers
// that read an Extra map, with the number of reads.
var allowedExtra = map[string]int{}

func match(pattern, rel string) bool {
	if strings.HasPrefix(pattern, "=") {
		return rel == pattern[1:]
	}
	return under(rel, pattern)
}

// unit returns the brand or driver unit of a package ("routerkit/brands/zte"
// for routerkit/brands/zte/ztefake), or "".
func unit(rel string) string {
	for _, base := range []string{brands, drivers} {
		if rest, ok := strings.CutPrefix(rel, base+"/"); ok {
			return base + "/" + strings.SplitN(rest, "/", 2)[0]
		}
	}
	return ""
}

// brandOf returns the brand library a package belongs to, or "".
func brandOf(rel string) string {
	if under(rel, asus) {
		return asus
	}
	if u := unit(rel); strings.HasPrefix(u, brands+"/") {
		return u
	}
	return ""
}

func ruleFor(rel string) (rule, bool) {
	best, found := rule{}, false
	for _, r := range rules {
		if !match(r.from, rel) {
			continue
		}
		if !found || len(strings.TrimPrefix(r.from, "=")) > len(strings.TrimPrefix(best.from, "=")) {
			best, found = r, true
		}
	}
	return best, found
}

func allowed(from, imp string, patterns []string) bool {
	for _, p := range patterns {
		switch p {
		case "$self":
			if u := unit(from); u != "" && under(imp, u) {
				return true
			}
		case "$brand":
			if brandOf(imp) != "" {
				return true
			}
		default:
			if match(p, imp) {
				return true
			}
		}
	}
	return false
}

func TestImportRules(t *testing.T) {
	pkgs := listModule(t)
	used := map[string]bool{}
	for _, p := range pkgs {
		r, ok := ruleFor(p.rel)
		if !ok {
			t.Errorf("%s matches no import rule: add it to rules", p.rel)
			continue
		}
		check := func(imp string, patterns []string) {
			edge := p.rel + " -> " + imp
			if _, ok := allowedEdges[edge]; ok {
				used[edge] = true
				return
			}
			if !allowed(p.rel, imp, patterns) {
				t.Errorf("forbidden import %s", edge)
				return
			}
			if under(p.rel, drivers) && under(imp, drivers) && !under(imp, unit(p.rel)) && imp != fake &&
				!contains(driverComposition[unit(p.rel)], unit(imp)) {
				t.Errorf("%s: a driver imports another driver (add a driverComposition edge)", edge)
			}
			if under(imp, config) && !p.isAny(configImporters) {
				t.Errorf("%s: only the app may import the config", edge)
			}
		}
		for _, imp := range p.imports {
			check(imp, r.allow)
		}
		for _, imp := range p.testImports {
			check(imp, append(append([]string{}, r.allow...), r.testAllow...))
		}
		if r.stdOnly {
			for _, imp := range p.external {
				t.Errorf("%s must import only the stdlib, imports %s", p.rel, imp)
			}
		}
		// A driver adapts exactly one brand library
		if under(p.rel, drivers) {
			seen := map[string]bool{}
			for _, imp := range append(append([]string{}, p.imports...), p.testImports...) {
				if b := brandOf(imp); b != "" {
					seen[b] = true
				}
			}
			if len(seen) > 1 {
				t.Errorf("%s imports more than one brand library: %v", p.rel, keys(seen))
			}
		}
	}
	for edge, why := range allowedEdges {
		if !used[edge] {
			t.Errorf("stale allowlist entry %q (%s): remove it", edge, why)
		}
	}
}

var initFunc = regexp.MustCompile(`(?m)^func init\(\)`)

// TestNoInitRegistration: libraries never self-register (docs/development.md, "Import rules"); only
// package main may use init() (the brand_<name>.go files).
func TestNoInitRegistration(t *testing.T) {
	for _, p := range listModule(t) {
		if !under(p.rel, "routerkit") && !under(p.rel, netkit) && !under(p.rel, app) {
			continue
		}
		for _, name := range p.goFiles {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(p.dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if initFunc.Match(b) {
				t.Errorf("%s/%s: init() in a library; export a Factory and register it in cmd", p.rel, name)
			}
		}
	}
}

var extraRead = regexp.MustCompile(`\.Extra\[`)

// TestNoExtraReads: services, sinks and the app never read the brand
// Extra maps; anything core logic needs becomes a typed model field.
func TestNoExtraReads(t *testing.T) {
	got := map[string]int{}
	for _, p := range listModule(t) {
		if under(p.rel, drivers) || under(p.rel, brands) || under(p.rel, asus) {
			continue
		}
		for _, name := range p.goFiles {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(p.dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if n := len(extraRead.FindAll(b, -1)); n > 0 {
				got[p.rel+"/"+name] = n
			}
		}
	}
	for file, n := range got {
		if want, ok := allowedExtra[file]; !ok || n > want {
			t.Errorf("%s reads an Extra map (%d times): use a typed field", file, n)
		}
	}
	for file, want := range allowedExtra {
		if got[file] < want {
			t.Errorf("allowlist entry %s (%d) is stale: lower or remove it", file, want)
		}
	}
}

func (p archPkg) isAny(prefixes []string) bool {
	for _, x := range prefixes {
		if under(p.rel, x) {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestArchCopiesInSync: the copies of this file in the other modules of the
// workspace are identical to this one apart from the package clause. It
// is skipped outside the workspace (a module downloaded on its own).
func TestArchCopiesInSync(t *testing.T) {
	self, dir := archModule(t)
	body := func(path string) ([]byte, error) {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		_, rest, _ := bytes.Cut(b, []byte("\n"))
		if !bytes.HasPrefix(b, []byte("package ")) {
			t.Fatalf("%s does not start with its package clause", path)
		}
		return rest, nil
	}
	mine, err := body(filepath.Join(dir, "arch_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "go.work")); err != nil {
		t.Skip("not in the workspace: no sibling modules to compare with")
	}
	for _, m := range keys(archModuleSet()) {
		if m == self {
			continue
		}
		other, err := body(filepath.Join(dir, "..", m, "arch_test.go"))
		if err != nil {
			t.Errorf("module %s has no arch test: %v", m, err)
			continue
		}
		if !bytes.Equal(mine, other) {
			t.Errorf("%s/arch_test.go differs from %s/arch_test.go: keep the copies identical", m, self)
		}
	}
}

func archModuleSet() map[string]bool {
	set := map[string]bool{}
	for m := range archModules {
		set[m] = true
	}
	return set
}
