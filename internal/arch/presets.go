package arch

import "slices"

// l is a compact layer literal for the preset tables.
func l(name string, dirs []string, may ...string) Layer {
	return Layer{Name: name, Dirs: dirs, MayImport: may}
}

func ss(s ...string) []string { return s }

// presetBuilders build a fresh Model per call so callers may mutate it.
var presetBuilders = map[string]func() []Layer{
	"clean": func() []Layer {
		return []Layer{
			l("entities", ss("entity", "entities", "model", "models", "domain")),
			l("usecases", ss("usecase", "usecases", "interactor", "interactors", "application", "app"), "entities"),
			l("adapters", ss("adapter", "adapters", "controller", "controllers", "presenter", "presenters", "gateway", "gateways", "repository", "repositories", "handler", "handlers"), "usecases", "entities"),
			l("frameworks", ss("framework", "frameworks", "infrastructure", "infra", "driver", "drivers", "db", "web", "http", "grpc"), "adapters", "usecases", "entities"),
		}
	},
	"onion": func() []Layer {
		return []Layer{
			l("domain", ss("domain", "model", "models", "entity", "entities")),
			l("domainservices", ss("domainservice", "domainservices"), "domain"),
			l("application", ss("application", "app", "usecase", "usecases"), "domainservices", "domain"),
			l("infrastructure", ss("infrastructure", "infra", "persistence", "repository", "repositories"), "application", "domainservices", "domain"),
			l("ui", ss("ui", "api", "web", "http", "grpc", "handler", "handlers", "controller", "controllers"), "application", "domainservices", "domain"),
		}
	},
	"hexagonal": func() []Layer {
		ad := l("adapters", ss("adapter", "adapters", "infrastructure", "infra", "repository", "repositories", "storage", "store", "transport", "http", "grpc", "handler", "handlers", "controller", "controllers"), "application", "ports", "domain")
		ad.Isolated = true
		return []Layer{
			l("domain", ss("domain", "core", "model", "models", "entity", "entities")),
			l("ports", ss("port", "ports"), "domain"),
			l("application", ss("application", "app", "usecase", "usecases", "service", "services"), "ports", "domain"),
			ad,
		}
	},
	"ddd": func() []Layer {
		return []Layer{
			l("domain", ss("domain")),
			l("application", ss("application", "app", "usecase", "usecases"), "domain"),
			l("infrastructure", ss("infrastructure", "infra", "persistence"), "application", "domain"),
			l("interfaces", ss("interfaces", "interface", "api", "http", "grpc", "web", "handler", "handlers", "controller", "controllers"), "application", "domain"),
		}
	},
	"layered": func() []Layer {
		return []Layer{
			l("presentation", ss("presentation", "ui", "web", "api", "http", "handler", "handlers", "controller", "controllers"), "service", "repository", "model"),
			l("service", ss("service", "services", "business", "logic"), "repository", "model"),
			l("repository", ss("repository", "repositories", "dao", "store", "storage", "persistence"), "model"),
			l("model", ss("model", "models", "entity", "entities", "domain")),
		}
	},
	"mvc": func() []Layer {
		return []Layer{
			l("controller", ss("controller", "controllers", "handler", "handlers"), "view", "model"),
			l("view", ss("view", "views", "template", "templates"), "model"),
			l("model", ss("model", "models")),
		}
	},
}

// presetOrder is the spec order of all eight names.
var presetOrder = []string{"clean", "onion", "hexagonal", "ddd", "layered", "mvc", "pipeline", "modular"}

// PresetNames lists every preset name in spec order.
func PresetNames() []string { return slices.Clone(presetOrder) }

// Preset returns a fresh copy of a table-driven preset. Pipeline and modular
// are built by NewPipeline and NewModular; they and unknown names report false.
func Preset(name string) (Model, bool) {
	b, ok := presetBuilders[name]
	if !ok {
		return Model{}, false
	}
	return Model{Name: name, Kind: Layers, Layers: b()}, true
}

// NewPipeline builds the pipeline model: stage i may import stages before it
// and every stage is isolated. Judging uses the plain Layers rules.
func NewPipeline(stages []string) Model {
	m := Model{Name: "pipeline", Kind: Pipeline}
	for i, s := range stages {
		m.Layers = append(m.Layers, Layer{
			Name:      s,
			Dirs:      []string{s},
			Isolated:  true,
			MayImport: slices.Clone(stages[:i]),
		})
	}
	return m
}

// NewModular builds the modular model; root "" means "internal".
func NewModular(root string) Model {
	if root == "" {
		root = "internal"
	}
	return Model{Name: "modular", Kind: Modular, ModulesRoot: root}
}

// Inferred is the zero-config layer map. It is advisory only.
func Inferred() Model {
	return Model{Name: "inferred", Inferred: true, Kind: Layers, Layers: []Layer{
		l("core", ss("domain", "core", "entity", "entities", "model", "models")),
		l("ports", ss("port", "ports"), "core"),
		l("application", ss("usecase", "usecases", "application", "app", "service", "services"), "ports", "core"),
		l("adapters", ss("adapter", "adapters", "infrastructure", "infra", "repository", "repositories", "storage", "store", "persistence", "transport", "http", "grpc", "handler", "handlers", "controller", "controllers", "api", "web", "db"), "application", "ports", "core"),
	}}
}
