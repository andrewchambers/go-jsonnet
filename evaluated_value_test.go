/*
Copyright 2026 The go-jsonnet Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package jsonnet_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	jsonnet "github.com/andrewchambers/go-jsonnet"
)

func TestEvaluateValueTypes(t *testing.T) {
	vm := jsonnet.MakeVM()
	value, err := vm.EvaluateAnonymousSnippetValue("types.jsonnet", `{
		"null": null,
		"bool": true,
		"number": 1.5,
		"string": "hello",
		"array": [1, 2],
		"object": { answer: 42 },
		"function": function(x) x,
		hidden:: "secret",
	}`)
	if err != nil {
		t.Fatal(err)
	}

	object := requireObject(t, value)
	fields, err := object.Fields()
	if err != nil {
		t.Fatal(err)
	}
	wantFields := []string{
		"array",
		"bool",
		"function",
		"null",
		"number",
		"object",
		"string",
	}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("Fields() = %#v, want %#v", fields, wantFields)
	}

	tests := []struct {
		name string
		want any
	}{
		{name: "null", want: jsonnet.NullValue{}},
		{name: "bool", want: jsonnet.BoolValue(true)},
		{name: "number", want: jsonnet.NumberValue(1.5)},
		{name: "string", want: jsonnet.StringValue("hello")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := requireField(t, object, test.name)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("%s = %#v, want %#v", test.name, got, test.want)
			}
		})
	}

	array, ok := requireField(t, object, "array").(jsonnet.ArrayValue)
	if !ok {
		t.Fatal("array field is not an ArrayValue")
	}
	if array.Len() != 2 {
		t.Fatalf("array.Len() = %d, want 2", array.Len())
	}
	element, err := array.Index(1)
	if err != nil {
		t.Fatal(err)
	}
	if element != jsonnet.NumberValue(2) {
		t.Fatalf("array[1] = %#v, want NumberValue(2)", element)
	}

	if _, ok := requireField(t, object, "object").(jsonnet.ObjectValue); !ok {
		t.Fatal("object field is not an ObjectValue")
	}
	if _, ok := requireField(t, object, "function").(jsonnet.FunctionValue); !ok {
		t.Fatal("function field is not a FunctionValue")
	}

	if hidden, found, err := object.Field("hidden"); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatalf("hidden field unexpectedly returned %#v", hidden)
	}
}

func TestObjectTokensPreserveEvaluationIdentity(t *testing.T) {
	vm := jsonnet.MakeVM()
	value, err := vm.EvaluateAnonymousSnippetValue("identity.jsonnet", `
		local dependency = { name: "dependency" };
		{
			first: dependency,
			second: dependency,
			equivalent: { name: "dependency" },
		}
	`)
	if err != nil {
		t.Fatal(err)
	}

	root := requireObject(t, value)
	first := requireObject(t, requireField(t, root, "first"))
	second := requireObject(t, requireField(t, root, "second"))
	equivalent := requireObject(t, requireField(t, root, "equivalent"))

	if first.Token() != second.Token() {
		t.Fatal("references to the same local produced different object tokens")
	}
	if first.Token() == equivalent.Token() {
		t.Fatal("separately constructed objects produced the same object token")
	}

	visited := map[jsonnet.ObjectToken]bool{first.Token(): true}
	if !visited[second.Token()] {
		t.Fatal("ObjectToken is not usable as a comparable map key")
	}
}

func TestObjectValueSupportsInheritanceAndSelf(t *testing.T) {
	vm := jsonnet.MakeVM()
	value, err := vm.EvaluateAnonymousSnippetValue("inheritance.jsonnet", `
		local base = {
			name: "base",
			greeting: "hello " + self.name,
		};
		base + { name: "child" }
	`)
	if err != nil {
		t.Fatal(err)
	}

	object := requireObject(t, value)
	greeting := requireField(t, object, "greeting")
	if greeting != jsonnet.StringValue("hello child") {
		t.Fatalf("greeting = %#v, want %q", greeting, "hello child")
	}
}

func TestValueAccessRemainsLazy(t *testing.T) {
	vm := jsonnet.MakeVM()
	value, err := vm.EvaluateAnonymousSnippetValue("lazy.jsonnet", `{
		good: 1,
		bad: error "not evaluated yet",
	}`)
	if err != nil {
		t.Fatal(err)
	}

	object := requireObject(t, value)
	if _, err := object.Fields(); err != nil {
		t.Fatalf("Fields() evaluated a field value: %v", err)
	}
	if got := requireField(t, object, "good"); got != jsonnet.NumberValue(1) {
		t.Fatalf("good = %#v, want NumberValue(1)", got)
	}

	_, found, err := object.Field("bad")
	if !found {
		t.Fatal("bad field was not found")
	}
	var runtimeErr jsonnet.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("bad field error = %T, want RuntimeError", err)
	}
}

func TestValueAccessChecksAssertions(t *testing.T) {
	vm := jsonnet.MakeVM()
	value, err := vm.EvaluateAnonymousSnippetValue(
		"assertion.jsonnet",
		`{ assert false : "assertion failed", value: 1 }`,
	)
	if err != nil {
		t.Fatal(err)
	}

	object := requireObject(t, value)
	if _, err := object.Fields(); err == nil {
		t.Fatal("Fields() succeeded despite a failing assertion")
	}
}

func TestArrayValueBoundsError(t *testing.T) {
	vm := jsonnet.MakeVM()
	value, err := vm.EvaluateAnonymousSnippetValue("array.jsonnet", `[]`)
	if err != nil {
		t.Fatal(err)
	}

	array, ok := value.(jsonnet.ArrayValue)
	if !ok {
		t.Fatalf("value = %T, want ArrayValue", value)
	}
	_, err = array.Index(0)
	var runtimeErr jsonnet.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("Index(0) error = %T, want RuntimeError", err)
	}
}

func TestEvaluateFileValueUsesConfiguredImporter(t *testing.T) {
	dir := t.TempDir()
	dependencyPath := filepath.Join(dir, "dependency.jsonnet")
	if err := os.WriteFile(
		dependencyPath,
		[]byte(`{ name: "dependency" }`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	rootPath := filepath.Join(dir, "root.jsonnet")
	if err := os.WriteFile(
		rootPath,
		[]byte(`{ dependency: import "./dependency.jsonnet" }`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	vm := jsonnet.MakeVM()
	value, err := vm.EvaluateFileValue(rootPath)
	if err != nil {
		t.Fatal(err)
	}

	root := requireObject(t, value)
	dependency := requireObject(t, requireField(t, root, "dependency"))
	if name := requireField(t, dependency, "name"); name != jsonnet.StringValue("dependency") {
		t.Fatalf("dependency.name = %#v, want %q", name, "dependency")
	}
}

func requireObject(t *testing.T, value jsonnet.Value) jsonnet.ObjectValue {
	t.Helper()
	object, ok := value.(jsonnet.ObjectValue)
	if !ok {
		t.Fatalf("value = %T, want ObjectValue", value)
	}
	return object
}

func requireField(
	t *testing.T,
	object jsonnet.ObjectValue,
	name string,
) jsonnet.Value {
	t.Helper()
	value, found, err := object.Field(name)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("field %q was not found", name)
	}
	return value
}
