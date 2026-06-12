package swagger

// FIXME These values are for the purposes of the code generator and should
// ideally be removed from the `swagger` package.

type ServerEndpoint struct {
	ContextTypeID string         `yaml:"context"`
	Meta          map[string]any `yaml:"meta"`
}
