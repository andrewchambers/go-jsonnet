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

package jsonnet

import (
	"fmt"
	"runtime/debug"

	"github.com/andrewchambers/go-jsonnet/ast"
	"github.com/andrewchambers/go-jsonnet/internal/program"
)

// EvaluateValue evaluates a Jsonnet AST without manifesting it to JSON.
//
// Errors are returned without applying ErrorFormatter.
func (vm *VM) EvaluateValue(node ast.Node) (result Value, err error) {
	defer recoverValueEvaluation(&err)

	interpreter, err := vm.buildConfiguredInterpreter()
	if err != nil {
		return nil, err
	}
	return evaluateValue(interpreter, node, vm.tla)
}

// EvaluateAnonymousSnippetValue evaluates a Jsonnet snippet without manifesting
// it to JSON.
//
// filename is used only in diagnostics. Errors are returned without applying
// ErrorFormatter.
func (vm *VM) EvaluateAnonymousSnippetValue(
	filename string,
	snippet string,
) (result Value, err error) {
	defer recoverValueEvaluation(&err)

	node, err := program.SnippetToAST(
		ast.DiagnosticFileName(filename),
		"",
		snippet,
	)
	if err != nil {
		return nil, err
	}
	return vm.EvaluateValue(node)
}

// EvaluateFileValue evaluates a Jsonnet file without manifesting it to JSON.
//
// The configured importer is used to load filename. Errors are returned without
// applying ErrorFormatter.
func (vm *VM) EvaluateFileValue(filename string) (result Value, err error) {
	defer recoverValueEvaluation(&err)

	node, _, err := vm.ImportAST("", filename)
	if err != nil {
		return nil, err
	}
	return vm.EvaluateValue(node)
}

func evaluateValue(
	interpreter *interpreter,
	node ast.Node,
	tla vmExtMap,
) (Value, error) {
	result, err := evaluateAux(interpreter, node, tla)
	if err != nil {
		return nil, err
	}
	evaluation := &valueEvaluation{interpreter: interpreter}
	return makeEvaluatedValue(evaluation, result), nil
}

func recoverValueEvaluation(err *error) {
	if recovered := recover(); recovered != nil {
		*err = fmt.Errorf("(CRASH) %v\n%s", recovered, debug.Stack())
	}
}
