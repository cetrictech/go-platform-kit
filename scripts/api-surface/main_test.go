package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const redisImport = "github.com/redis/go-redis/v9"

func source(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	source(t, root, "go.mod", "module example.test/sdk\n\ngo 1.26.0\n")
	for name, body := range files {
		source(t, root, name, body)
	}
	return root
}
func scan(t *testing.T, root string) map[string][]string {
	t.Helper()
	s := scanner{root: root, prefixes: []string{"github.com/redis/go-redis"}, packages: map[string]*sourcePackage{}}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	return s.inventory()
}
func TestPublicTypesAndSignatures(t *testing.T) {
	root := fixture(t, map[string]string{"api.go": `package sdk
 import r "github.com/redis/go-redis/v9"
 type Native = r.Client
 type hidden = r.PubSub
 type concealed struct{}
 func (concealed) Borrow() *r.Client { return nil }
 func ReturnedPrivate() concealed { return concealed{} }
 type Options struct { Connection *r.Client; secret *r.PubSub }
 type Contract interface { Do(*r.Client); private(*r.PubSub) }
 type Handle struct{}
 func (Handle) Borrow() *r.Client { return nil }
 func Open() *Native { return nil }
 func HiddenAlias() *hidden { return nil }
 func WithOptions(Options) {}
 func Generic[T interface { r.Client | r.PubSub }](T) {}
 var Global *r.Client
 `})
	actual := scan(t, root)
	names := []string{"Native", "Options", "Options.Connection", "Contract", "Contract.Do", "Handle.Borrow", "Open", "HiddenAlias", "ReturnedPrivate", "WithOptions", "Generic", "Global"}
	expected := map[string][]string{}
	for _, name := range names {
		expected["example.test/sdk."+name] = []string{redisImport}
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("surface mismatch\n got: %#v\nwant: %#v", actual, expected)
	}
}
func TestPrivateImplementationAndExcludedSourceAreIgnored(t *testing.T) {
	hidden := `package sdk; import r "github.com/redis/go-redis/v9"; func Leak() *r.Client { return nil }`
	root := fixture(t, map[string]string{
		"api.go": `package sdk
 import r "github.com/redis/go-redis/v9"
 type Configuration struct { private *r.Client }
 type private struct{}
 func (private) Method() *r.Client { return nil }
 func privateFunction() *r.Client { return nil }
 func Safe(Configuration) { _ = r.NewClient(nil) }
 `,
		"native_test.go": hidden, "internal/leak/native.go": hidden, "integration/leak.go": hidden, "vendor/leak.go": hidden, "testdata/leak.go": hidden,
	})
	if got := scan(t, root); len(got) != 0 {
		t.Fatalf("private implementation leaked into contract: %#v", got)
	}
}
func TestRecursiveAndCrossPackageAliases(t *testing.T) {
	root := fixture(t, map[string]string{
		"api.go":        `package sdk; import sub "example.test/sdk/sub"; func Open() sub.Alias { panic("unused") }; type Recursive struct { Next *Recursive }`,
		"sub/native.go": `package sub; import "github.com/redis/go-redis/v9"; type Alias = redis.Client`,
	})
	expected := map[string][]string{"example.test/sdk.Open": {redisImport}, "example.test/sdk/sub.Alias": {redisImport}}
	if got := scan(t, root); !reflect.DeepEqual(got, expected) {
		t.Fatalf("named type resolution mismatch: %#v", got)
	}
}
func TestPolicyAllowsReviewedNativeAndRejectsNewExposure(t *testing.T) {
	root := fixture(t, map[string]string{"api.go": `package sdk; import r "github.com/redis/go-redis/v9"; func Native() *r.Client { return nil }`})
	approved := policy{SchemaVersion: 1, DriverImportPrefixes: []string{"github.com/redis/go-redis"}, Approved: map[string][]string{"example.test/sdk.Native": {redisImport}}}
	bytes, err := json.Marshal(approved)
	if err != nil {
		t.Fatal(err)
	}
	source(t, root, "release/driver-exposure.json", string(bytes))
	if err := execute([]string{"--root", root}); err != nil {
		t.Fatal(err)
	}
	source(t, root, "new.go", `package sdk; import r "github.com/redis/go-redis/v9"; func NewLeak() *r.Client { return nil }`)
	if err := execute([]string{"--root", root}); err == nil || !strings.Contains(err.Error(), "NewLeak: unreviewed") {
		t.Fatalf("new exposure not blocked: %v", err)
	}
	path := filepath.Join(root, "review-candidate.json")
	if err := execute([]string{"--root", root, "--write-candidate", path}); err != nil {
		t.Fatal(err)
	}
	if err := execute([]string{"--root", root, "--write-candidate", path}); err == nil {
		t.Fatal("candidate output silently overwritten")
	}
	if err := execute([]string{"--root", root, "--write-candidate", "release/driver-exposure.json"}); err == nil {
		t.Fatal("approved policy overwritten")
	}
	after, err := os.ReadFile(filepath.Join(root, "release/driver-exposure.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(bytes) {
		t.Fatal("gate mutated approved baseline")
	}
}
