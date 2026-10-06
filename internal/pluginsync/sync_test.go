package pluginsync_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sourcehawk/operator-component-framework/internal/pluginsync"
)

const siteURL = "https://docs.example.com/"

var testCopies = []pluginsync.Copy{
	{Source: "component.md", Dest: "components/references/component.md"},
	{Source: "observability.md", Dest: "components/references/observability.md"},
	{Source: "guidelines.md", Dest: "structuring/references/guidelines.md"},
	{Source: "primitives/*.md", Dest: "primitives/references/primitives"},
}

func TestSync(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		in   string
		want string
	}{
		{
			name: "link to a doc in another skill points to its copy, anchor kept",
			doc:  "guidelines.md",
			in:   "See [status](component.md#persisting-status).\n",
			want: "See [status](../../components/references/component.md#persisting-status).\n",
		},
		{
			name: "link to a doc in the same skill stays a sibling link",
			doc:  "component.md",
			in:   "See [alerts](observability.md).\n",
			want: "See [alerts](observability.md).\n",
		},
		{
			name: "link from a doc in a subfolder resolves from that subfolder",
			doc:  "primitives/deployment.md",
			in:   "See [component](../component.md#status-model).\n",
			want: "See [component](../../../components/references/component.md#status-model).\n",
		},
		{
			name: "link to a doc in a glob points to the copy of that doc",
			doc:  "component.md",
			in:   "See [Deployment](primitives/deployment.md).\n",
			want: "See [Deployment](../../primitives/references/primitives/deployment.md).\n",
		},
		{
			name: "link to a doc that is not copied points to its page on the docs site",
			doc:  "guidelines.md",
			in:   "See [the CLI](cli.md#regenerating) and [home](index.md).\n",
			want: "See [the CLI](https://docs.example.com/cli/#regenerating) and [home](https://docs.example.com/).\n",
		},
		{
			name: "external links, anchors, and links in inline code do not change",
			doc:  "guidelines.md",
			in:   "[Go](https://go.dev), [up](#top), `[x](component.md)`, ``a ` [y](component.md)``.\n",
			want: "[Go](https://go.dev), [up](#top), `[x](component.md)`, ``a ` [y](component.md)``.\n",
		},
		{
			name: "links in a fenced code block do not change",
			doc:  "guidelines.md",
			in:   "```go\nx := cells[string](component.md)\n```\n~~~~\n[a](component.md)\n~~~~\n",
			want: "```go\nx := cells[string](component.md)\n```\n~~~~\n[a](component.md)\n~~~~\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docsDir := writeDocs(t, map[string]string{tt.doc: tt.in})
			skillsDir := t.TempDir()

			require.NoError(t, pluginsync.Sync(docsDir, skillsDir, siteURL, testCopies))

			got, err := os.ReadFile(filepath.Join(skillsDir, destOf(tt.doc)))
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestSyncRemovesStaleCopies(t *testing.T) {
	docsDir := writeDocs(t, nil)
	skillsDir := t.TempDir()
	stale := filepath.Join(skillsDir, "components/references/removed.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(stale), 0o755))
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0o644))
	skillFile := filepath.Join(skillsDir, "components/SKILL.md")
	require.NoError(t, os.WriteFile(skillFile, []byte("skill"), 0o644))

	require.NoError(t, pluginsync.Sync(docsDir, skillsDir, siteURL, testCopies))

	assert.NoFileExists(t, stale)
	assert.FileExists(t, skillFile)
}

func TestSyncFailsForLinkToMissingDoc(t *testing.T) {
	docsDir := writeDocs(t, map[string]string{"guidelines.md": "See [gone](missing.md).\n"})

	err := pluginsync.Sync(docsDir, t.TempDir(), siteURL, testCopies)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing.md")
}

func TestSyncFailsForGlobWithNoMatch(t *testing.T) {
	docsDir := writeDocs(t, nil)
	copies := slices.Concat(testCopies, []pluginsync.Copy{{Source: "none/*.md", Dest: "none/references"}})

	err := pluginsync.Sync(docsDir, t.TempDir(), siteURL, copies)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "none/*.md")
}

func TestCheckLinks(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "skills/a/references/target.md"), "# Target\n")

	t.Run("reports nothing when each relative link resolves", func(t *testing.T) {
		dir := filepath.Join(root, "valid")
		write(t, filepath.Join(dir, "doc.md"),
			"[ok](../skills/a/references/target.md#anchor) [web](https://go.dev) [top](#top)\n"+
				"`[code](missing.md)`\n```\n[fenced](missing.md)\n```\n")

		assert.NoError(t, pluginsync.CheckLinks(dir))
	})

	t.Run("names each relative link that resolves to no file", func(t *testing.T) {
		dir := filepath.Join(root, "broken")
		write(t, filepath.Join(dir, "nested/doc.md"), "[a](missing.md) [b](../gone.md#x)\n")

		err := pluginsync.CheckLinks(dir)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing.md")
		assert.Contains(t, err.Error(), "../gone.md#x")
	})
}

// writeDocs creates a docs folder with a doc for each source of testCopies, a doc that is not copied, and the docs in
// overrides.
func writeDocs(t *testing.T, overrides map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	docs := map[string]string{
		"component.md":             "# Component\n",
		"observability.md":         "# Observability\n",
		"guidelines.md":            "# Guidelines\n",
		"primitives/deployment.md": "# Deployment\n",
		"cli.md":                   "# CLI\n",
		"index.md":                 "# Home\n",
	}
	for doc, content := range overrides {
		docs[doc] = content
	}
	for doc, content := range docs {
		write(t, filepath.Join(dir, doc), content)
	}
	return dir
}

func write(t *testing.T, file, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
	require.NoError(t, os.WriteFile(file, []byte(content), 0o644))
}

// destOf returns the destination of doc in testCopies.
func destOf(doc string) string {
	if doc == "primitives/deployment.md" {
		return "primitives/references/primitives/deployment.md"
	}
	for _, c := range testCopies {
		if c.Source == doc {
			return c.Dest
		}
	}
	return ""
}
