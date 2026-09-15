package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// CI supplies full Git history. Compare SQL itself, so changing a fixture along
// with production code cannot silently rewrite an already shipped migration.
func TestReleasedMigrationHistory(t *testing.T) {
	if os.Getenv("CHECK_MIGRATION_HISTORY") != "1" {
		t.Skip("CI history check; requires full Git history")
	}
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	refs := strings.Fields(git("tag", "--list", "v[0-9]*"))
	if base := os.Getenv("MIGRATION_BASE_REF"); base != "" && strings.Trim(base, "0") != "" {
		refs = append(refs, base)
	}
	for _, ref := range refs {
		t.Run(ref, func(t *testing.T) {
			path := "app/internal/store/migrate.go"
			if strings.TrimSpace(git("ls-tree", "--name-only", ref, "--", path)) == "" {
				return
			}
			source := git("show", ref+":"+path)
			previous := migrationSQL(t, source)
			if len(previous) > len(migrations) {
				t.Fatal("historical migrations removed")
			}
			for i, sql := range previous {
				if sql != migrations[i] {
					t.Fatalf("migration %d differs from %s; append a new step instead", i+1, ref)
				}
			}
		})
	}
}

func migrationSQL(t *testing.T, source string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "migrate.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	var result []string
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "migrations" {
			return true
		}
		if len(spec.Values) != 1 {
			t.Fatal("unsupported migration declaration")
		}
		list, ok := spec.Values[0].(*ast.CompositeLit)
		if !ok {
			t.Fatal("migrations must be a literal list")
		}
		for _, element := range list.Elts {
			literal, ok := element.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Fatal("migration must be literal SQL")
			}
			sql, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			result = append(result, sql)
		}
		return false
	})
	if len(result) == 0 {
		t.Fatal("no migration history found")
	}
	return result
}
