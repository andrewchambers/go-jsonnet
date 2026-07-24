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
	"sort"

	"github.com/andrewchambers/go-jsonnet/ast"
)

// Value is an evaluated Jsonnet value.
//
// Values retain the interpreter that produced them so that lazy array elements
// and object fields can be evaluated on demand. Values and their accessors are
// not safe for concurrent use.
//
// The interface is sealed. A type switch can be used to distinguish its
// concrete implementations.
type Value interface {
	isValue()
}

// NullValue is the Jsonnet null value.
type NullValue struct{}

// BoolValue is a Jsonnet boolean value.
type BoolValue bool

// NumberValue is a Jsonnet number value.
type NumberValue float64

// StringValue is a Jsonnet string value.
type StringValue string

// ArrayValue is a lazily evaluated Jsonnet array.
type ArrayValue struct {
	evaluation *valueEvaluation
	array      *valueArray
}

// ObjectValue is a lazily evaluated Jsonnet object.
type ObjectValue struct {
	evaluation *valueEvaluation
	object     *valueObject
}

// FunctionValue is an opaque Jsonnet function value.
//
// Calling evaluated functions is not currently supported by this API.
type FunctionValue struct {
	evaluation *valueEvaluation
	function   *valueFunction
}

// ObjectToken is an evaluation-local identity token for an ObjectValue.
//
// Tokens are comparable and may be used as map keys. They are intended for
// memoization and cycle detection while traversing a single evaluation, not as
// persistent or semantic identities.
type ObjectToken struct {
	evaluation *valueEvaluation
	object     *valueObject
}

type valueEvaluation struct {
	interpreter *interpreter
}

func (NullValue) isValue()     {}
func (BoolValue) isValue()     {}
func (NumberValue) isValue()   {}
func (StringValue) isValue()   {}
func (ArrayValue) isValue()    {}
func (ObjectValue) isValue()   {}
func (FunctionValue) isValue() {}

// Len returns the number of elements in the array.
func (v ArrayValue) Len() int {
	return v.array.length()
}

// Index evaluates and returns the array element at index.
func (v ArrayValue) Index(index int) (Value, error) {
	restoreTrace := v.evaluation.setValueTrace(
		fmt.Sprintf("Array element %d", index),
	)
	defer restoreTrace()

	element, err := v.array.index(v.evaluation.interpreter, index)
	if err != nil {
		return nil, err
	}
	return makeEvaluatedValue(v.evaluation, element), nil
}

// Fields returns the sorted names of the object's visible fields.
//
// Object assertions are checked before the field names are returned. Hidden
// fields are omitted, matching normal Jsonnet manifestation.
func (v ObjectValue) Fields() ([]string, error) {
	restoreTrace := v.evaluation.setValueTrace("Checking object assertions")
	defer restoreTrace()

	if err := checkAssertions(v.evaluation.interpreter, v.object); err != nil {
		return nil, err
	}

	fields := objectFields(v.object, withoutHidden)
	sort.Strings(fields)
	return fields, nil
}

// Field evaluates a visible object field. The boolean result is false when the
// field does not exist or is hidden.
func (v ObjectValue) Field(name string) (Value, bool, error) {
	restoreTrace := v.evaluation.setValueTrace(fmt.Sprintf("Field %#v", name))
	defer restoreTrace()

	if err := checkAssertions(v.evaluation.interpreter, v.object); err != nil {
		return nil, false, err
	}

	visibility, ok := objectFieldsVisibility(v.object)[name]
	if !ok || visibility == ast.ObjectFieldHidden {
		return nil, false, nil
	}

	field, err := v.object.index(v.evaluation.interpreter, name)
	if err != nil {
		return nil, true, err
	}
	return makeEvaluatedValue(v.evaluation, field), true, nil
}

// Token returns an evaluation-local identity token for the object.
func (v ObjectValue) Token() ObjectToken {
	return ObjectToken{
		evaluation: v.evaluation,
		object:     v.object,
	}
}

func (e *valueEvaluation) setValueTrace(message string) func() {
	previous := e.interpreter.stack.currentTrace
	loc := ast.MakeLocationRangeMessage(message)
	e.interpreter.stack.setCurrentTrace(traceElement{loc: &loc})
	return func() {
		e.interpreter.stack.clearCurrentTrace()
		e.interpreter.stack.setCurrentTrace(previous)
	}
}

func makeEvaluatedValue(evaluation *valueEvaluation, v value) Value {
	switch v := v.(type) {
	case *valueNull:
		return NullValue{}
	case *valueBoolean:
		return BoolValue(v.value)
	case *valueNumber:
		return NumberValue(v.value)
	case valueString:
		return StringValue(v.getGoString())
	case *valueArray:
		return ArrayValue{
			evaluation: evaluation,
			array:      v,
		}
	case *valueObject:
		return ObjectValue{
			evaluation: evaluation,
			object:     v,
		}
	case *valueFunction:
		return FunctionValue{
			evaluation: evaluation,
			function:   v,
		}
	default:
		panic(fmt.Sprintf("unknown Jsonnet value type %T", v))
	}
}
