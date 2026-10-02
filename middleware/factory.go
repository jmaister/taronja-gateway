package middleware

// MiddlewareFactory creates middleware instances from configuration.
// Each existing middleware gets a factory so it can be registered, discovered,
// and composed at runtime by a Registry.
type MiddlewareFactory interface {
	// Create builds the middleware instance. cfg is factory-specific; pass
	// GetDefaultConfig() (or a compatible value) when no override is needed.
	Create(cfg interface{}) (Middleware, error)
	// GetName returns the stable identifier used in MiddlewareSpec.Name.
	GetName() string
	// GetDescription returns a short human-readable summary of what the middleware does.
	GetDescription() string
	// GetDependencies returns the names of middleware that must be enabled
	// earlier in the same chain for this middleware to function correctly.
	GetDependencies() []string
	// GetDefaultConfig returns the zero-value/default configuration for this middleware.
	GetDefaultConfig() interface{}
}

// ConcreteFactory is an embeddable base implementation shared by all factories,
// providing the name/description/dependencies bookkeeping.
type ConcreteFactory struct {
	name         string
	description  string
	dependencies []string
}

// NewConcreteFactory returns the base for a factory with the given name,
// description and optional dependencies.
func NewConcreteFactory(name, description string, dependencies ...string) ConcreteFactory {
	return ConcreteFactory{name: name, description: description, dependencies: dependencies}
}

func (f *ConcreteFactory) GetName() string           { return f.name }
func (f *ConcreteFactory) GetDescription() string    { return f.description }
func (f *ConcreteFactory) GetDependencies() []string { return f.dependencies }
