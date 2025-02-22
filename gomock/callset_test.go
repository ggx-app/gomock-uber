// Copyright 2011 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gomock

import (
	"reflect"
	"testing"
)

type receiverType struct{}

func (receiverType) Func() {}

func TestCallSetAdd(t *testing.T) {
	method := "TestMethod"
	var receiver any = "TestReceiver"
	cs := newCallSet()

	numCalls := 10
	for i := 0; i < numCalls; i++ {
		cs.Add(newCall(t, receiver, method, reflect.TypeOf(receiverType{}.Func), nil))
	}

	call, err := cs.FindMatch(receiver, method, []any{})
	if err != nil {
		t.Fatalf("FindMatch: %v", err)
	}
	if call == nil {
		t.Fatalf("FindMatch: Got nil, want non-nil *Call")
	}
}

func TestCallSetAdd_WhenOverridable_ClearsPreviousExpectedAndExhausted(t *testing.T) {
	method := "TestMethod"
	var receiver any = "TestReceiver"
	cs := newOverridableCallSet()

	cs.Add(newCall(t, receiver, method, reflect.TypeOf(receiverType{}.Func), nil))
	numExpectedCalls := len(cs.expected[callSetKey{receiver, method}])
	if numExpectedCalls != 1 {
		t.Fatalf("Expected 1 expected call in callset, got %d", numExpectedCalls)
	}

	cs.Add(newCall(t, receiver, method, reflect.TypeOf(receiverType{}.Func), nil))
	newNumExpectedCalls := len(cs.expected[callSetKey{receiver, method}])
	if newNumExpectedCalls != 1 {
		t.Fatalf("Expected 1 expected call in callset, got %d", newNumExpectedCalls)
	}
}

func TestCallSetRemove(t *testing.T) {
	method := "TestMethod"
	var receiver any = "TestReceiver"

	cs := newCallSet()
	ourCalls := []*Call{}

	numCalls := 10
	for i := 0; i < numCalls; i++ {
		// NOTE: abuse the `numCalls` value to convey initial ordering of mocked calls
		generatedCall := &Call{receiver: receiver, method: method, numCalls: i}
		cs.Add(generatedCall)
		ourCalls = append(ourCalls, generatedCall)
	}

	// validateOrder validates that the calls in the array are ordered as they were added
	validateOrder := func(calls []*Call) {
		// lastNum tracks the last `numCalls` (call order) value seen
		lastNum := -1
		for _, c := range calls {
			if lastNum >= c.numCalls {
				t.Errorf("found call %d after call %d", c.numCalls, lastNum)
			}
			lastNum = c.numCalls
		}
	}

	for _, c := range ourCalls {
		validateOrder(cs.expected[callSetKey{receiver, method}])
		cs.Remove(c)
	}
}

func TestCallSetFindMatch(t *testing.T) {
	t.Run("call is exhausted", func(t *testing.T) {
		cs := newCallSet()
		var receiver any = "TestReceiver"
		method := "TestMethod"
		args := []any{}

		c1 := newCall(t, receiver, method, reflect.TypeOf(receiverType{}.Func), nil)
		cs.exhausted = map[callSetKey][]*Call{
			{receiver: receiver, fname: method}: {c1},
		}

		_, err := cs.FindMatch(receiver, method, args)
		if err == nil {
			t.Fatal("expected error, but was nil")
		}

		if err.Error() == "" {
			t.Fatal("expected error to have message, but was empty")
		}
	})
}
