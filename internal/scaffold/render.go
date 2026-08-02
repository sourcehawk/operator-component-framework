package scaffold

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// GeneratedFiles lists the files a wrapper package consists of, in write order.
var GeneratedFiles = []string{"builder.go", "builder_test.go", "mutator.go", "resource.go"}

// templatePaths maps each generated file to its embedded template.
var templatePaths = map[string]string{
	"builder.go":      "templates/builder.go.tmpl",
	"builder_test.go": "templates/builder_test.go.tmpl",
	"mutator.go":      "templates/mutator.go.tmpl",
	"resource.go":     "templates/resource.go.tmpl",
}

// Render renders every file of a wrapper package, keyed by file name. Output is
// passed through go/format, so templates do not need to be whitespace-perfect.
func Render(data TemplateData) (map[string][]byte, error) {
	if data.Spec().GenericBuilder == "" {
		return nil, fmt.Errorf("unknown variant %q", data.Variant)
	}

	rendered := make(map[string][]byte, len(GeneratedFiles))

	for _, name := range GeneratedFiles {
		path := templatePaths[name]

		tmpl, err := template.New(name).ParseFS(templateFS, path)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", path, err)
		}

		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, templateName(path), data); err != nil {
			return nil, fmt.Errorf("render %s: %w", name, err)
		}

		formatted, err := format.Source(buf.Bytes())
		if err != nil {
			return nil, fmt.Errorf("format %s: %w", name, err)
		}

		rendered[name] = formatted
	}

	return rendered, nil
}

// templateName returns the template name ParseFS assigns to an embedded path.
func templateName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
