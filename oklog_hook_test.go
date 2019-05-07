package oklog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	pkgerrors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWriting(t *testing.T) {
	msgChan := make(chan Message, 1)
	l := startTestOklogServer(t, msgChan)
	//noinspection GoUnhandledErrorResult
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	hook := NewOklogHook([]string{fmt.Sprintf(":%d", port)}, map[string]interface{}{"foo": "bar"})
	hook.Host = "testing.local"
	hook.Blacklist([]string{"filterMe"})
	msgData := "test message\nsecond line"

	log := logrus.New()
	log.Out = ioutil.Discard
	log.Hooks.Add(hook)
	log.WithFields(logrus.Fields{"withField": "1", "filterMe": "1"}).Info(msgData)

	select {
	case msg := <-msgChan:
		if msg.Short != "test message" {
			t.Errorf("msg.Short: expected %s, got %s", msgData, msg.Full)
		}

		if msg.Full != msgData {
			t.Errorf("msg.Full: expected %s, got %s", msgData, msg.Full)
		}

		if msg.Level != logrus.InfoLevel.String() {
			t.Errorf("msg.Level: expected: %s, got %s)", logrus.InfoLevel.String(), msg.Level)
		}

		if msg.Host != "testing.local" {
			t.Errorf("Host should match (exp: testing.local, got: %s)", msg.Host)
		}

		if len(msg.Extra) != 2 {
			t.Errorf("wrong number of extra fields (exp: %d, got %d) in %v", 5, len(msg.Extra), msg.Extra)
		}

		if len(msg.Extra) != 2 {
			t.Errorf("wrong number of extra fields (exp: %d, got %d) in %v", 2, len(msg.Extra), msg.Extra)
		}

		extra := map[string]interface{}{"foo": "bar", "withField": "1"}

		for k, v := range extra {
			// Remember extra fields are prefixed with "_"
			if msg.Extra["_"+k].(string) != extra[k].(string) {
				t.Errorf("Expected extra '%s' to be %#v, got %#v", k, v, msg.Extra["_"+k])
			}
		}
	case <-time.After(time.Millisecond * 10):
		t.Error("Timed out; no notice received by OK Log")
	}
}

func TestJSONErrorMarshalling(t *testing.T) {
	msgChan := make(chan Message, 1)
	l := startTestOklogServer(t, msgChan)
	//noinspection GoUnhandledErrorResult
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	hook := NewOklogHook([]string{fmt.Sprintf(":%d", port)}, map[string]interface{}{})

	log := logrus.New()
	log.Out = ioutil.Discard
	log.Hooks.Add(hook)

	log.WithError(errors.New("sample error")).Info("Testing sample error")

	select {
	case msg := <-msgChan:
		encoded, err := json.Marshal(msg)
		if err != nil {
			t.Errorf("Marshaling json: %s", err)
		}

		errSection := regexp.MustCompile(`"_error":"sample error"`)
		if !errSection.MatchString(string(encoded)) {
			t.Errorf("Expected error message to be encoded into message")
		}
	case <-time.After(time.Millisecond * 10):
		t.Error("Timed out; no notice received by OK Log")
	}
}

func TestParallelLogging(t *testing.T) {
	msgChan := make(chan Message, 20)
	l := startTestOklogServer(t, msgChan)
	//noinspection GoUnhandledErrorResult
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	hook := NewOklogHook([]string{fmt.Sprintf(":%d", port)}, nil)
	asyncHook := NewAsyncOklogHook([]string{fmt.Sprintf(":%d", port)}, nil)
	defer asyncHook.Flush()

	log := logrus.New()
	log.Out = ioutil.Discard
	log.Hooks.Add(hook)
	log.Hooks.Add(asyncHook)

	quit := make(chan struct{})
	defer close(quit)

	panicked := false

	recordPanic := func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}

	go func() {
		// Start draining messages from OK Log
		go func() {
			defer recordPanic()
			for {
				select {
				case <-msgChan:
					continue
				case <-quit:
					return
				default:

					continue
				}
			}
		}()

		// Log into our hook in parallel
		for i := 0; i < 10; i++ {
			go func() {
				defer recordPanic()
				for {
					select {
					case <-quit:
						return
					default:
						log.Info("Logging")
					}
				}
			}()
		}
	}()

	// Let them all do their thing for a while
	time.Sleep(100 * time.Millisecond)
	if panicked {
		t.Fatalf("Logging in parallel caused a panic")
	}
}

func TestWithInvalidOklogAddr(t *testing.T) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}
	logrus.SetOutput(ioutil.Discard)
	hook := NewOklogHook([]string{addr.String()}, nil)

	log := logrus.New()
	log.Out = ioutil.Discard
	log.Hooks.Add(hook)

	// Should not panic
	log.WithError(errors.New("sample error")).Info("Testing sample error")
}

func TestStackTracer(t *testing.T) {
	msgChan := make(chan Message, 1)
	l := startTestOklogServer(t, msgChan)
	//noinspection GoUnhandledErrorResult
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	hook := NewOklogHook([]string{fmt.Sprintf(":%d", port)}, map[string]interface{}{})

	log := logrus.New()
	log.SetReportCaller(true)
	log.Out = ioutil.Discard
	log.Hooks.Add(hook)

	stackErr := pkgerrors.New("sample error")

	log.WithError(stackErr).Info("Testing sample error")

	select {
	case msg := <-msgChan:
		stacktraceI, ok := msg.Extra[stackTraceKey]
		if !ok {
			t.Error("Stack Trace not found in result")
		}
		stacktrace, ok := stacktraceI.(string)
		if !ok {
			t.Error("Stack Trace is not a string")
		}

		// Run the test for stack trace only in stable versions
		if !strings.Contains(runtime.Version(), "devel") {
			stacktraceRE := regexp.MustCompile(`^
(.+)?logrus-oklog-hook.TestStackTracer
	(/|[A-Z]:/).+/logrus-oklog-hook/oklog_hook_test.go:\d+
testing.tRunner
	(/|[A-Z]:/).*/testing.go:\d+
runtime.*
	(/|[A-Z]:/).*/runtime/.*:\d+$`)

			if !stacktraceRE.MatchString(stacktrace) {
				t.Errorf("Stack Trace not as expected. Got:\n%s\n", stacktrace)
			}
		}
	case <-time.After(time.Millisecond * 10):
		t.Error("Timed out; no notice received by OK Log")
	}
}

func TestReportCallerEnabled(t *testing.T) {
	msgChan := make(chan Message, 1)
	l := startTestOklogServer(t, msgChan)
	//noinspection GoUnhandledErrorResult
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	hook := NewOklogHook([]string{fmt.Sprintf(":%d", port)}, map[string]interface{}{})
	hook.Host = "testing.local"
	msgData := "test message\nsecond line"

	log := logrus.New()
	log.SetReportCaller(true)
	log.Out = ioutil.Discard
	log.Hooks.Add(hook)
	log.Info(msgData)

	select {
	case msg := <-msgChan:
		fileField, ok := msg.Extra["_file"]
		if !ok {
			t.Error("_file field not present in extra fields")
		}

		fileGot, ok := fileField.(string)
		if !ok {
			t.Error("_file field is not a string")
		}

		fileExpected := "oklog_hook_test.go"
		if !strings.HasSuffix(fileGot, fileExpected) {
			t.Errorf("msg.Extra[\"_file\"]: expected %s, got %s", fileExpected, fileGot)
		}

		lineField, ok := msg.Extra["_line"]
		if !ok {
			t.Error("_line field not present in extra fields")
		}

		lineGot, ok := lineField.(float64)
		if !ok {
			t.Error("_line does not have the correct type")
		}

		lineExpected := 253 // Update this if code is updated above
		if int(lineField.(float64)) != lineExpected {
			t.Errorf("msg.Extra[\"_line\"]: expected %d, got %d", lineExpected, int(lineGot))
		}

		functionField, ok := msg.Extra["_function"]
		if !ok {
			t.Error("_function field not present in extra fields")
		}

		functionGot, ok := functionField.(string)
		if !ok {
			t.Error("_function field is not a string")
		}

		functionExpected := "TestReportCallerEnabled"
		if !strings.HasSuffix(functionGot, functionExpected) {
			t.Errorf("msg.Extra[\"_function\"]: expected %s, got %s", functionExpected, functionGot)
		}
	case <-time.After(time.Millisecond * 25):
		t.Error("Timed out; no notice received by OK Log")
	}
}

func TestReportCallerDisabled(t *testing.T) {
	msgChan := make(chan Message, 1)
	l := startTestOklogServer(t, msgChan)
	//noinspection GoUnhandledErrorResult
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	hook := NewOklogHook([]string{fmt.Sprintf(":%d", port)}, map[string]interface{}{})
	hook.Host = "testing.local"
	msgData := "test message\nsecond line"

	log := logrus.New()
	log.SetReportCaller(false)
	log.Out = ioutil.Discard
	log.Hooks.Add(hook)
	log.Info(msgData)

	select {
	case msg := <-msgChan:
		if _, ok := msg.Extra["_file"]; ok {
			t.Error("_file field should not be present in extra fields")
		}

		if _, ok := msg.Extra["_line"]; ok {
			t.Error("_line field should not be present in extra fields")
		}

		if _, ok := msg.Extra["_function"]; ok {
			t.Error("_function field should not be present in extra fields")
		}
	case <-time.After(time.Millisecond * 10):
		t.Error("Timed out; no notice received by OK Log")
	}
}

func startTestOklogServer(t *testing.T, msgChan chan<- Message) net.Listener {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		//noinspection GoUnhandledErrorResult
		defer conn.Close()

		s := bufio.NewScanner(conn)
		for s.Scan() {
			var message Message
			if err := json.NewDecoder(bytes.NewReader(s.Bytes())).Decode(&message); err != nil {
				t.Error(err)
			}
			msgChan <- message
		}
	}()

	return l
}
