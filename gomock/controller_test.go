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

package gomock_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type ErrorReporter struct {
	t          *testing.T
	log        []string
	failed     bool
	fatalToken struct{}
}

func NewErrorReporter(t *testing.T) *ErrorReporter {
	return &ErrorReporter{t: t}
}

func (e *ErrorReporter) reportLog() {
	for _, entry := range e.log {
		e.t.Log(entry)
	}
}

func (e *ErrorReporter) assertPass(msg string) {
	if e.failed {
		e.t.Errorf("Expected pass, but got failure(s): %s", msg)
		e.reportLog()
	}
}

func (e *ErrorReporter) assertFail(msg string) {
	if !e.failed {
		e.t.Errorf("Expected failure, but got pass: %s", msg)
	}
}

// Use to check that code triggers a fatal test failure.
func (e *ErrorReporter) assertFatal(fn func(), expectedErrMsgs ...string) {
	defer func() {
		err := recover()
		if err == nil {
			var actual string
			if e.failed {
				actual = "non-fatal failure"
			} else {
				actual = "pass"
			}
			e.t.Error("Expected fatal failure, but got a", actual)
		} else if token, ok := err.(*struct{}); ok && token == &e.fatalToken {
			// This is okay - the panic is from Fatalf().
			if expectedErrMsgs != nil {
				// assert that the actual error message
				// contains expectedErrMsgs

				// check the last actualErrMsg, because the previous messages come from previous errors
				actualErrMsg := e.log[len(e.log)-1]
				for _, expectedErrMsg := range expectedErrMsgs {
					i := strings.Index(actualErrMsg, expectedErrMsg)
					if i == -1 {
						e.t.Errorf("Error message:\ngot: %q\nwant to contain: %q\n", actualErrMsg, expectedErrMsg)
					} else {
						actualErrMsg = actualErrMsg[i+len(expectedErrMsg):]
					}
				}
			}
			return
		} else {
			// Some other panic.
			panic(err)
		}
	}()

	fn()
}

// recoverUnexpectedFatal can be used as a deferred call in test cases to
// recover from and display a call to ErrorReporter.Fatalf().
func (e *ErrorReporter) recoverUnexpectedFatal() {
	err := recover()
	if err == nil {
		// No panic.
	} else if token, ok := err.(*struct{}); ok && token == &e.fatalToken {
		// Unexpected fatal error happened.
		e.t.Error("Got unexpected fatal error(s). All errors up to this point:")
		e.reportLog()
		return
	} else {
		// Some other panic.
		panic(err)
	}
}

func (e *ErrorReporter) Log(args ...any) {
	e.log = append(e.log, fmt.Sprint(args...))
}

func (e *ErrorReporter) Logf(format string, args ...any) {
	e.log = append(e.log, fmt.Sprintf(format, args...))
}

func (e *ErrorReporter) Errorf(format string, args ...any) {
	e.Logf(format, args...)
	e.failed = true
}

func (e *ErrorReporter) Fatalf(format string, args ...any) {
	e.Logf(format, args...)
	e.failed = true
	panic(&e.fatalToken)
}

type HelperReporter struct {
	gomock.TestReporter
	helper int
}

func (h *HelperReporter) Helper() {
	h.helper++
}

// A type purely for use as a receiver in testing the Controller.
type Subject struct{}

func (s *Subject) FooMethod(arg string) int {
	return 0
}

func (s *Subject) BarMethod(arg string) int {
	return 0
}

func (s *Subject) VariadicMethod(arg int, vararg ...string) {}

// A type purely for ActOnTestStructMethod
type TestStruct struct {
	Number        int
	Message       string
	secretMessage string
}

func (s *Subject) ActOnTestStructMethod(arg TestStruct, arg1 int) int {
	return 0
}

func (s *Subject) SetArgMethod(sliceArg []byte, ptrArg *int, mapArg map[any]any) {}
func (s *Subject) SetArgMethodInterface(sliceArg, ptrArg, mapArg any)            {}

func assertEqual(t *testing.T, expected any, actual any) {
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Expected %+v, but got %+v", expected, actual)
	}
}

func createFixtures(t *testing.T) (reporter *ErrorReporter, ctrl *gomock.Controller) {
	// reporter acts as a testing.T-like object that we pass to the
	// Controller. We use it to test that the mock considered tests
	// successful or failed.
	reporter = NewErrorReporter(t)
	ctrl = gomock.NewController(
		reporter, gomock.WithCmpOpts(cmpopts.IgnoreUnexported(TestStruct{})),
	)
	return
}

func TestNoCalls(t *testing.T) {
	reporter, _ := createFixtures(t)
	reporter.assertPass("No calls expected or made.")
}

func TestNoRecordedCallsForAReceiver(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	subject := new(Subject)

	reporter.assertFatal(func() {
		ctrl.Call(subject, "NotRecordedMethod", "argument")
	}, "Unexpected call to", "there are no expected calls of the method \"NotRecordedMethod\" for that receiver")
}

func TestNoRecordedMatchingMethodNameForAReceiver(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	subject := new(Subject)

	ctrl.RecordCall(subject, "FooMethod", "argument")
	reporter.assertFatal(func() {
		ctrl.Call(subject, "NotRecordedMethod", "argument")
	}, "Unexpected call to", "there are no expected calls of the method \"NotRecordedMethod\" for that receiver")
	reporter.assertFatal(func() {
		// The expected call wasn't made.
		ctrl.Finish()
	})
}

func TestNoStringerDeadlockOnError(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	subject := new(Subject)
	mockFoo := NewMockFoo(ctrl)
	var _ fmt.Stringer = mockFoo

	ctrl.RecordCall(subject, "FooMethod", mockFoo)
	reporter.assertFatal(func() {
		ctrl.Call(subject, "NotRecordedMethod", mockFoo)
	}, "Unexpected call to", "there are no expected calls of the method \"NotRecordedMethod\" for that receiver")
	reporter.assertFatal(func() {
		// The expected call wasn't made.
		ctrl.Finish()
	})
}

// This tests that a call with an arguments of some primitive type matches a recorded call.
func TestExpectedMethodCall(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	subject := new(Subject)

	ctrl.RecordCall(subject, "FooMethod", "argument")
	ctrl.Call(subject, "FooMethod", "argument")

	reporter.assertPass("Expected method call made.")
}

func TestUnexpectedMethodCall(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	subject := new(Subject)

	reporter.assertFatal(func() {
		ctrl.Call(subject, "FooMethod", "argument")
	})
}

func TestRepeatedCall(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	subject := new(Subject)

	ctrl.RecordCall(subject, "FooMethod", "argument").Times(3)
	ctrl.Call(subject, "FooMethod", "argument")
	ctrl.Call(subject, "FooMethod", "argument")
	ctrl.Call(subject, "FooMethod", "argument")
	reporter.assertPass("After expected repeated method calls.")
	reporter.assertFatal(func() {
		ctrl.Call(subject, "FooMethod", "argument")
	})
	reporter.assertFail("After calling one too many times.")
}

func TestUnexpectedArgCount(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	defer reporter.recoverUnexpectedFatal()
	subject := new(Subject)

	ctrl.RecordCall(subject, "FooMethod", "argument")
	reporter.assertFatal(func() {
		// This call is made with the wrong number of arguments...
		ctrl.Call(subject, "FooMethod", "argument", "extra_argument")
	}, "Unexpected call to", "wrong number of arguments", "Got: 2, want: 1")
	reporter.assertFatal(func() {
		// ... so is this.
		ctrl.Call(subject, "FooMethod")
	}, "Unexpected call to", "wrong number of arguments", "Got: 0, want: 1")
	reporter.assertFatal(func() {
		// The expected call wasn't made.
		ctrl.Finish()
	})
}

// This tests that a call with complex arguments (a struct and some primitive type) matches a recorded call.
func TestExpectedMethodCall_CustomStruct(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	subject := new(Subject)

	expectedArg0 := TestStruct{Number: 123, Message: "hello"}
	ctrl.RecordCall(subject, "ActOnTestStructMethod", expectedArg0, 15)
	ctrl.Call(subject, "ActOnTestStructMethod", expectedArg0, 15)

	reporter.assertPass("Expected method call made.")
}

func TestUnexpectedArgValue_FirstArg(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	defer reporter.recoverUnexpectedFatal()
	subject := new(Subject)

	expectedArg0 := TestStruct{Number: 123, Message: "hello %s"}
	ctrl.RecordCall(subject, "ActOnTestStructMethod", expectedArg0, 15)

	reporter.assertFatal(func() {
		// the method argument (of TestStruct type) has 1 unexpected value (for the Message field)
		ctrl.Call(subject, "ActOnTestStructMethod", TestStruct{Number: 123, Message: "no message"}, 15)
	}, "Unexpected call to", "doesn't match the argument at index 0",
		"Diff (-want +got):", "gomock_test.TestStruct{", "Number:  123", "-", "Message: \"hello %s\",", "+", "Message: \"no message\",", "}")

	reporter.assertFatal(func() {
		// the method argument (of TestStruct type) has 2 unexpected values (for both fields)
		ctrl.Call(subject, "ActOnTestStructMethod", TestStruct{Number: 11, Message: "no message"}, 15)
	}, "Unexpected call to", "doesn't match the argument at index 0",
		"Diff (-want +got):", "gomock_test.TestStruct{", "-", "Number:  123,", "+", "Number:  11,", "-", "Message: \"hello %s\",", "+", "Message: \"no message\",", "}")

	reporter.assertFatal(func() {
		// The expected call wasn't made.
		ctrl.Finish()
	})
}

func TestUnexpectedArgValue_SecondArg(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	defer reporter.recoverUnexpectedFatal()
	subject := new(Subject)

	expectedArg0 := TestStruct{Number: 123, Message: "hello"}
	ctrl.RecordCall(subject, "ActOnTestStructMethod", expectedArg0, 15)

	reporter.assertFatal(func() {
		ctrl.Call(subject, "ActOnTestStructMethod", TestStruct{Number: 123, Message: "hello"}, 3)
	}, "Unexpected call to", "doesn't match the argument at index 1",
		"Diff (-want +got):", "int(", "-", "15,", "+", "3,", ")")

	reporter.assertFatal(func() {
		// The expected call wasn't made.
		ctrl.Finish()
	})
}

func TestUnexpectedArgValue_WantFormatter(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	defer reporter.recoverUnexpectedFatal()
	subject := new(Subject)

	expectedArg0 := TestStruct{Number: 123, Message: "hello"}
	ctrl.RecordCall(
		subject,
		"ActOnTestStructMethod",
		expectedArg0,
		gomock.WantFormatter(
			gomock.StringerFunc(func() string { return "is equal to fifteen" }),
			gomock.Eq(15),
		),
	)

	reporter.assertFatal(func() {
		ctrl.Call(subject, "ActOnTestStructMethod", TestStruct{Number: 123, Message: "hello"}, 3)
	}, "Unexpected call to", "doesn't match the argument at index 1",
		"Got: 3 (int)\nWant: is equal to fifteen")

	reporter.assertFatal(func() {
		// The expected call wasn't made.
		ctrl.Finish()
	})
}

func TestUnexpectedArgValue_GotFormatter(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	defer reporter.recoverUnexpectedFatal()
	subject := new(Subject)

	expectedArg0 := TestStruct{Number: 123, Message: "hello"}
	ctrl.RecordCall(
		subject,
		"ActOnTestStructMethod",
		expectedArg0,
		gomock.GotFormatterAdapter(
			gomock.GotFormatterFunc(func(i any) string {
				// Leading 0s
				return fmt.Sprintf("%02d", i)
			}),
			gomock.Eq(15),
		),
	)

	reporter.assertFatal(func() {
		ctrl.Call(subject, "ActOnTestStructMethod", TestStruct{Number: 123, Message: "hello"}, 3)
	}, "Unexpected call to", "doesn't match the argument at index 1",
		"Got: 03\nWant: is equal to 15")

	reporter.assertFatal(func() {
		// The expected call wasn't made.
		ctrl.Finish()
	})
}

func TestAnyTimes(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	subject := new(Subject)

	ctrl.RecordCall(subject, "FooMethod", "argument").AnyTimes()
	for i := 0; i < 100; i++ {
		ctrl.Call(subject, "FooMethod", "argument")
	}
	reporter.assertPass("After 100 method calls.")
}

func TestMinTimes1(t *testing.T) {
	// It fails if there are no calls
	reporter, ctrl := createFixtures(t)
	subject := new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MinTimes(1)
	reporter.assertFatal(func() {
		ctrl.Finish()
	})

	// It succeeds if there is one call
	_, ctrl = createFixtures(t)
	subject = new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MinTimes(1)
	ctrl.Call(subject, "FooMethod", "argument")
	ctrl.Finish()

	// It succeeds if there are many calls
	_, ctrl = createFixtures(t)
	subject = new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MinTimes(1)
	for i := 0; i < 100; i++ {
		ctrl.Call(subject, "FooMethod", "argument")
	}
	ctrl.Finish()
}

func TestMaxTimes1(t *testing.T) {
	// It succeeds if there are no calls
	_, ctrl := createFixtures(t)
	subject := new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MaxTimes(1)
	ctrl.Finish()

	// It succeeds if there is one call
	_, ctrl = createFixtures(t)
	subject = new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MaxTimes(1)
	ctrl.Call(subject, "FooMethod", "argument")
	ctrl.Finish()

	// It fails if there are more
	reporter, ctrl := createFixtures(t)
	subject = new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MaxTimes(1)
	ctrl.Call(subject, "FooMethod", "argument")
	reporter.assertFatal(func() {
		ctrl.Call(subject, "FooMethod", "argument")
	})
	ctrl.Finish()
}

func TestMinMaxTimes(t *testing.T) {
	// It fails if there are less calls than specified
	reporter, ctrl := createFixtures(t)
	subject := new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MinTimes(2).MaxTimes(2)
	ctrl.Call(subject, "FooMethod", "argument")
	reporter.assertFatal(func() {
		ctrl.Finish()
	})

	// It fails if there are more calls than specified
	reporter, ctrl = createFixtures(t)
	subject = new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MinTimes(2).MaxTimes(2)
	ctrl.Call(subject, "FooMethod", "argument")
	ctrl.Call(subject, "FooMethod", "argument")
	reporter.assertFatal(func() {
		ctrl.Call(subject, "FooMethod", "argument")
	})

	// It succeeds if there is just the right number of calls
	_, ctrl = createFixtures(t)
	subject = new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MaxTimes(2).MinTimes(2)
	ctrl.Call(subject, "FooMethod", "argument")
	ctrl.Call(subject, "FooMethod", "argument")
	ctrl.Finish()

	// If MaxTimes is called after MinTimes is called with 1, MaxTimes takes precedence.
	reporter, ctrl = createFixtures(t)
	subject = new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MinTimes(1).MaxTimes(2)
	ctrl.Call(subject, "FooMethod", "argument")
	ctrl.Call(subject, "FooMethod", "argument")
	reporter.assertFatal(func() {
		ctrl.Call(subject, "FooMethod", "argument")
	})

	// If MinTimes is called after MaxTimes is called with 1, MinTimes takes precedence.
	_, ctrl = createFixtures(t)
	subject = new(Subject)
	ctrl.RecordCall(subject, "FooMethod", "argument").MaxTimes(1).MinTimes(2)
	for i := 0; i < 100; i++ {
		ctrl.Call(subject, "FooMethod", "argument")
	}
	ctrl.Finish()
}

func TestDo(t *testing.T) {
	_, ctrl := createFixtures(t)
	subject := new(Subject)

	doCalled := false
	var argument string
	wantArg := "argument"
	ctrl.RecordCall(subject, "FooMethod", wantArg).Do(
		func(arg string) {
			doCalled = true
			argument = arg
		})
	if doCalled {
		t.Error("Do() callback called too early.")
	}

	ctrl.Call(subject, "FooMethod", wantArg)

	if !doCalled {
		t.Error("Do() callback not called.")
	}
	if wantArg != argument {
		t.Error("Do callback received wrong argument.")
	}
}

func TestDoAndReturn(t *testing.T) {
	_, ctrl := createFixtures(t)
	subject := new(Subject)

	doCalled := false
	var argument string
	wantArg := "argument"
	ctrl.RecordCall(subject, "FooMethod", wantArg).DoAndReturn(
		func(arg string) int {
			doCalled = true
			argument = arg
			return 5
		})
	if doCalled {
		t.Error("Do() callback called too early.")
	}

	rets := ctrl.Call(subject, "FooMethod", wantArg)

	if !doCalled {
		t.Error("Do() callback not called.")
	}
	if wantArg != argument {
		t.Error("Do callback received wrong argument.")
	}
	if len(rets) != 1 {
		t.Fatalf("Return values from Call: got %d, want 1", len(rets))
	}
	if ret, ok := rets[0].(int); !ok {
		t.Fatalf("Return value is not an int")
	} else if ret != 5 {
		t.Errorf("DoAndReturn return value: got %d, want 5", ret)
	}
}

func TestSetArgSlice(t *testing.T) {
	_, ctrl := createFixtures(t)
	subject := new(Subject)

	var in = []byte{4, 5, 6}
	var set = []byte{1, 2, 3}
	ctrl.RecordCall(subject, "SetArgMethod", in, nil, nil).SetArg(0, set)
	ctrl.Call(subject, "SetArgMethod", in, nil, nil)

	if !reflect.DeepEqual(in, set) {
		t.Error("Expected SetArg() to modify input slice argument")
	}

	ctrl.RecordCall(subject, "SetArgMethodInterface", in, nil, nil).SetArg(0, set)
	ctrl.Call(subject, "SetArgMethodInterface", in, nil, nil)

	if !reflect.DeepEqual(in, set) {
		t.Error("Expected SetArg() to modify input slice argument as any")
	}
}

func TestSetArgMap(t *testing.T) {
	_, ctrl := createFixtures(t)
	subject := new(Subject)

	var in = map[any]any{"int": 1, "string": "random string", 1: "1", 0: 0}
	var set = map[any]any{"int": 2, 1: "2", 2: 100}
	ctrl.RecordCall(subject, "SetArgMethod", nil, nil, in).SetArg(2, set)
	ctrl.Call(subject, "SetArgMethod", nil, nil, in)

	if !reflect.DeepEqual(in, set) {
		t.Error("Expected SetArg() to modify input map argument")
	}

	ctrl.RecordCall(subject, "SetArgMethodInterface", nil, nil, in).SetArg(2, set)
	ctrl.Call(subject, "SetArgMethodInterface", nil, nil, in)

	if !reflect.DeepEqual(in, set) {
		t.Error("Expected SetArg() to modify input map argument as any")
	}
}

func TestSetArgPtr(t *testing.T) {
	_, ctrl := createFixtures(t)
	subject := new(Subject)

	var in int = 43
	const set = 42
	ctrl.RecordCall(subject, "SetArgMethod", nil, &in, nil).SetArg(1, set)
	ctrl.Call(subject, "SetArgMethod", nil, &in, nil)

	if in != set {
		t.Error("Expected SetArg() to modify value pointed to by argument")
	}

	ctrl.RecordCall(subject, "SetArgMethodInterface", nil, &in, nil).SetArg(1, set)
	ctrl.Call(subject, "SetArgMethodInterface", nil, &in, nil)

	if in != set {
		t.Error("Expected SetArg() to modify value pointed to by argument as any")
	}
}

func TestReturn(t *testing.T) {
	_, ctrl := createFixtures(t)
	subject := new(Subject)

	// Unspecified return should produce "zero" result.
	ctrl.RecordCall(subject, "FooMethod", "zero")
	ctrl.RecordCall(subject, "FooMethod", "five").Return(5)

	assertEqual(
		t,
		[]any{0},
		ctrl.Call(subject, "FooMethod", "zero"))

	assertEqual(
		t,
		[]any{5},
		ctrl.Call(subject, "FooMethod", "five"))
}

func TestUnorderedCalls(t *testing.T) {
	reporter, ctrl := createFixtures(t)
	defer reporter.recoverUnexpectedFatal()
	subjectTwo := new(Subject)
	subjectOne := new(Subject)

	ctrl.RecordCall(subjectOne, "FooMethod", "1")
	ctrl.RecordCall(subjectOne, "BarMethod", "2")
	ctrl.RecordCall(subjectTwo, "FooMethod", "3")
	ctrl.RecordCall(subjectTwo, "BarMethod", "4")

	// Make the calls in a different order, which should be fine.
	ctrl.Call(subjectOne, "BarMethod", "2")
	ctrl.Call(subjectTwo, "FooMethod", "3")
	ctrl.Call(subjectTwo, "BarMethod", "4")
	ctrl.Call(subjectOne, "FooMethod", "1")

	reporter.assertPass("After making all calls in different order")

	ctrl.Finish()

	reporter.assertPass("After finish")
}

func commonTestOrderedCalls(t *testing.T) (reporter *ErrorReporter, ctrl *gomock.Controller, subjectOne, subjectTwo *Subject) {
	reporter, ctrl = createFixtures(t)

	subjectOne = new(Subject)
	subjectTwo = new(Subject)

	gomock.InOrder(
		ctrl.RecordCall(subjectOne, "FooMethod", "1").AnyTimes(),
		ctrl.RecordCall(subjectTwo, "FooMethod", "2"),
		ctrl.RecordCall(subjectTwo, "BarMethod", "3"),
	)

	return
}

func TestOrderedCallsCorrect(t *testing.T) {
	reporter, ctrl, subjectOne, subjectTwo := commonTestOrderedCalls(t)

	ctrl.Call(subjectOne, "FooMethod", "1")
	ctrl.Call(subjectTwo, "FooMethod", "2")
	ctrl.Call(subjectTwo, "BarMethod", "3")

	ctrl.Finish()

	reporter.assertPass("After finish")
}

func TestPanicOverridesExpectationChecks(t *testing.T) {
	ctrl := gomock.NewController(t)
	reporter := NewErrorReporter(t)

	reporter.assertFatal(func() {
		ctrl.RecordCall(new(Subject), "FooMethod", "1")
		defer ctrl.Finish()
		reporter.Fatalf("Intentional panic")
	})
}

func TestSetArgWithBadType(t *testing.T) {
	rep, ctrl := createFixtures(t)

	s := new(Subject)
	// This should catch a type error:
	rep.assertFatal(func() {
		ctrl.RecordCall(s, "FooMethod", "1").SetArg(0, "blah")
	})
	ctrl.Call(s, "FooMethod", "1")
}

func TestTimes0(t *testing.T) {
	rep, ctrl := createFixtures(t)

	s := new(Subject)
	ctrl.RecordCall(s, "FooMethod", "arg").Times(0)
	rep.assertFatal(func() {
		ctrl.Call(s, "FooMethod", "arg")
	})
}

func TestVariadicMatching(t *testing.T) {
	rep, ctrl := createFixtures(t)
	defer rep.recoverUnexpectedFatal()

	s := new(Subject)
	ctrl.RecordCall(s, "VariadicMethod", 0, "1", "2")
	ctrl.Call(s, "VariadicMethod", 0, "1", "2")
	rep.assertPass("variadic matching works")
}

func TestVariadicNoMatch(t *testing.T) {
	rep, ctrl := createFixtures(t)
	defer rep.recoverUnexpectedFatal()

	s := new(Subject)
	ctrl.RecordCall(s, "VariadicMethod", 0)
	rep.assertFatal(func() {
		ctrl.Call(s, "VariadicMethod", 1)
	}, "expected call at", "doesn't match the argument at index 0",
		"Got: 1 (int)\nWant: is equal to 0 (int)")
	ctrl.Call(s, "VariadicMethod", 0)
}

func TestVariadicMatchingWithSlice(t *testing.T) {
	testCases := [][]string{
		{"1"},
		{"1", "2"},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d arguments", len(tc)), func(t *testing.T) {
			rep, ctrl := createFixtures(t)
			defer rep.recoverUnexpectedFatal()

			s := new(Subject)
			ctrl.RecordCall(s, "VariadicMethod", 1, tc)
			args := make([]any, len(tc)+1)
			args[0] = 1
			for i, arg := range tc {
				args[i+1] = arg
			}
			ctrl.Call(s, "VariadicMethod", args...)
			rep.assertPass("slices can be used as matchers for variadic arguments")
		})
	}
}

func TestVariadicArgumentsGotFormatter(t *testing.T) {
	rep, ctrl := createFixtures(t)
	defer rep.recoverUnexpectedFatal()

	s := new(Subject)
	ctrl.RecordCall(
		s,
		"VariadicMethod",
		gomock.GotFormatterAdapter(
			gomock.GotFormatterFunc(func(i any) string {
				return fmt.Sprintf("test{%v}", i)
			}),
			gomock.Eq(0),
		),
	)

	rep.assertFatal(func() {
		ctrl.Call(s, "VariadicMethod", 1)
	}, "expected call to", "doesn't match the argument at index 0",
		"Got: test{1}\nWant: is equal to 0")
	ctrl.Call(s, "VariadicMethod", 0)
}

func TestVariadicArgumentsGotFormatterTooManyArgsFailure(t *testing.T) {
	rep, ctrl := createFixtures(t)
	defer rep.recoverUnexpectedFatal()

	s := new(Subject)
	ctrl.RecordCall(
		s,
		"VariadicMethod",
		0,
		gomock.GotFormatterAdapter(
			gomock.GotFormatterFunc(func(i any) string {
				return fmt.Sprintf("test{%v}", i)
			}),
			gomock.Eq("1"),
		),
	)

	rep.assertFatal(func() {
		ctrl.Call(s, "VariadicMethod", 0, "2", "3")
	}, "expected call to", "doesn't match the argument at index 1",
		"Got: test{[2 3]}\nWant: is equal to 1")
	ctrl.Call(s, "VariadicMethod", 0, "1")
}

func TestNoHelper(t *testing.T) {
	ctrlNoHelper := gomock.NewController(NewErrorReporter(t))

	// doesn't panic
	ctrlNoHelper.T.Helper()
}

func TestWithHelper(t *testing.T) {
	withHelper := &HelperReporter{TestReporter: NewErrorReporter(t)}
	ctrlWithHelper := gomock.NewController(withHelper)

	ctrlWithHelper.T.Helper()

	if withHelper.helper == 0 {
		t.Fatal("expected Helper to be invoked")
	}
}

func (e *ErrorReporter) Cleanup(f func()) {
	e.t.Helper()
	e.t.Cleanup(f)
}

func TestMultipleDefers(t *testing.T) {
	reporter := NewErrorReporter(t)
	reporter.Cleanup(func() {
		reporter.assertPass("No errors for multiple calls to Finish")
	})
	_ = gomock.NewController(reporter)
}

func TestDeferNotNeededPass(t *testing.T) {
	reporter := NewErrorReporter(t)
	subject := new(Subject)
	var ctrl *gomock.Controller
	reporter.Cleanup(func() {
		reporter.assertPass("Expected method call made.")
	})
	ctrl = gomock.NewController(reporter)
	ctrl.RecordCall(subject, "FooMethod", "argument")
	ctrl.Call(subject, "FooMethod", "argument")
}

func TestOrderedCallsInCorrect(t *testing.T) {
	reporter := NewErrorReporter(t)
	subjectOne := new(Subject)
	subjectTwo := new(Subject)
	var ctrl *gomock.Controller
	reporter.Cleanup(func() {
		reporter.assertFatal(func() {
			gomock.InOrder(
				ctrl.RecordCall(subjectOne, "FooMethod", "1").AnyTimes(),
				ctrl.RecordCall(subjectTwo, "FooMethod", "2"),
				ctrl.RecordCall(subjectTwo, "BarMethod", "3"),
			)
			ctrl.Call(subjectOne, "FooMethod", "1")
			// FooMethod(2) should be called before BarMethod(3)
			ctrl.Call(subjectTwo, "BarMethod", "3")
		}, "Unexpected call to", "Subject.BarMethod([3])", "doesn't have a prerequisite call satisfied")
	})
	ctrl = gomock.NewController(reporter)
}

// Test that calls that are prerequisites to other calls but have maxCalls >
// minCalls are removed from the expected call set.
func TestOrderedCallsWithPreReqMaxUnbounded(t *testing.T) {
	reporter := NewErrorReporter(t)
	subjectOne := new(Subject)
	subjectTwo := new(Subject)
	var ctrl *gomock.Controller
	reporter.Cleanup(func() {
		reporter.assertFatal(func() {
			// Initially we should be able to call FooMethod("1") as many times as we
			// want.
			ctrl.Call(subjectOne, "FooMethod", "1")
			ctrl.Call(subjectOne, "FooMethod", "1")

			// But calling something that has it as a prerequisite should remove it from
			// the expected call set. This allows tests to ensure that FooMethod("1") is
			// *not* called after FooMethod("2").
			ctrl.Call(subjectTwo, "FooMethod", "2")

			ctrl.Call(subjectOne, "FooMethod", "1")
		})
	})
	ctrl = gomock.NewController(reporter)
}

func TestCallAfterLoopPanic(t *testing.T) {
	reporter := NewErrorReporter(t)
	subject := new(Subject)
	var ctrl *gomock.Controller
	reporter.Cleanup(func() {
		firstCall := ctrl.RecordCall(subject, "FooMethod", "1")
		secondCall := ctrl.RecordCall(subject, "FooMethod", "2")
		thirdCall := ctrl.RecordCall(subject, "FooMethod", "3")

		gomock.InOrder(firstCall, secondCall, thirdCall)

		defer func() {
			err := recover()
			if err == nil {
				t.Error("Call.After creation of dependency loop did not panic.")
			}
		}()

		// This should panic due to dependency loop.
		firstCall.After(thirdCall)
	})
	ctrl = gomock.NewController(reporter)
}
