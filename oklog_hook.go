package oklog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"sync"
	"time"
)

const stackTraceKey = "_stacktrace"

// BufSize -- Set oklog.BufSize = <value> _before_ calling NewOklogHook.
// Once the buffer is full, logging will start blocking, waiting for slots to be available in the queue.
var BufSize uint = 8192

// Hook to send logs to a logging service compatible with the OK Log in JSON format.
type Hook struct {
	Extra       map[string]interface{}
	Host        string
	Level       logrus.Level
	okLogger    *Writer
	buf         chan *logrus.Entry
	wg          sync.WaitGroup
	mu          sync.RWMutex
	synchronous bool
	blacklist   map[string]bool
}

// NewOklogHook creates a hook to be added to an instance of logger.
func NewOklogHook(addr []string, extra map[string]interface{}) *Hook {
	o, err := NewWriter(addr)
	if err != nil {
		logrus.WithError(err).Error("Can't create OK Log logger")
	}

	host, err := os.Hostname()
	if err != nil {
		host = "localhost"
	}

	hook := &Hook{
		Host:        host,
		Extra:       extra,
		Level:       logrus.DebugLevel,
		okLogger:    o,
		synchronous: true,
	}
	return hook
}

// NewAsyncOklogHook creates a hook to be added to an instance of logger.
// The hook created will be asynchronous, and it's the responsibility of the user to call the Flush method before
// exiting to empty the log queue.
func NewAsyncOklogHook(addr []string, extra map[string]interface{}) *Hook {
	o, err := NewWriter(addr)
	if err != nil {
		logrus.WithError(err).Error("Can't create OK Log logger")
	}

	host, err := os.Hostname()
	if err != nil {
		host = "localhost"
	}

	hook := &Hook{
		Host:     host,
		Extra:    extra,
		Level:    logrus.DebugLevel,
		okLogger: o,
		buf:      make(chan *logrus.Entry, BufSize),
	}
	go hook.fire() // Log in background
	return hook
}

// Levels returns the available logging levels.
func (hook *Hook) Levels() []logrus.Level {
	//noinspection GoPreferNilSlice
	levels := []logrus.Level{}
	for _, level := range logrus.AllLevels {
		if level <= hook.Level {
			levels = append(levels, level)
		}
	}
	return levels
}

// Fire is called when a log event is fired.
// We assume the entry will be altered by another hook, otherwise we might logging something wrong to OK log
func (hook *Hook) Fire(entry *logrus.Entry) error {
	hook.mu.RLock() // Claim the mutex as a RLock - allowing multiple go routines to log simultaneously
	defer hook.mu.RUnlock()

	newData := make(map[string]interface{})
	for k, v := range entry.Data {
		newData[k] = v
	}

	newEntry := &logrus.Entry{
		Logger:  entry.Logger,
		Data:    newData,
		Time:    entry.Time,
		Level:   entry.Level,
		Caller:  entry.Caller,
		Message: entry.Message,
	}

	if hook.synchronous {
		hook.sendEntry(newEntry)
	} else {
		hook.wg.Add(1)
		hook.buf <- newEntry
	}

	return nil
}

// Flush waits for the log queue to be empty.
// This func is meant to be used when the hook was created with NewAsyncOklogHook.
func (hook *Hook) Flush() {
	hook.mu.Lock() // claim the mutex as a Lock - we want exclusive access to it
	defer hook.mu.Unlock()

	hook.wg.Wait()
}

// Blacklist create a blacklist map to filter some message keys.
// This useful when you want your application to log extra fields locally but don't want OK Log to store them.
func (hook *Hook) Blacklist(b []string) {
	hook.blacklist = make(map[string]bool)
	for _, elem := range b {
		hook.blacklist[elem] = true
	}
}

// fire will loop on the 'buf' channel, and write entries to OK Log
func (hook *Hook) fire() {
	for {
		entry := <-hook.buf // receive new entry on channel
		hook.sendEntry(entry)
		hook.wg.Done()
	}
}

// sendEntry sends an entry to OK Log synchronously
func (hook *Hook) sendEntry(entry *logrus.Entry) {
	if hook.okLogger == nil {
		fmt.Println("Can't connect to OK Log")
		return
	}

	// remove trailing and leading whitespace
	p := bytes.TrimSpace([]byte(entry.Message))

	// If there are newlines in the message, use the first line for the short message and set the full message to the
	// original input.  If the input has no newlines, stick the whole thing in Short.
	short := p
	full := []byte("")
	if i := bytes.IndexRune(p, '\n'); i > 0 {
		short = p[:i]
		full = p
	}

	// Don't modify entry.Data directly, as the entry will used after this hook was fired
	extra := map[string]interface{}{}
	// Merge extra fields
	for k, v := range hook.Extra {
		// "[...] every field you send and prefix with a _ (underscore) will be treated as an additional field."
		k = fmt.Sprintf("_%s", k)
		extra[k] = v
	}

	if entry.Caller != nil {
		extra["_file"] = entry.Caller.File
		extra["_line"] = entry.Caller.Line
		extra["_function"] = entry.Caller.Function
	}

	for k, v := range entry.Data {
		if !hook.blacklist[k] {
			// "[...] every field you send and prefix with a _ (underscore) will be treated as an additional field."
			extraK := fmt.Sprintf("_%s", k)
			if k == logrus.ErrorKey {
				asError, isError := v.(error)
				_, isMarshaller := v.(json.Marshaler)
				if isError && !isMarshaller {
					extra[extraK] = newMarshallableError(asError)
				} else {
					extra[extraK] = v
				}
				if stackTrace := extractStackTrace(asError); stackTrace != nil {
					extra[stackTraceKey] = fmt.Sprintf("%+v", stackTrace)
				}
			} else {
				extra[extraK] = v
			}
		}
	}

	m := Message{
		Version:  "0.1",
		Host:     hook.Host,
		Short:    string(short),
		Full:     string(full),
		TimeUnix: float64(time.Now().UnixNano()/1000000) / 1000.,
		Level:    entry.Level.String(),
		Extra:    extra,
	}

	if err := hook.okLogger.WriteMessage(&m); err != nil {
		fmt.Println(err)
	}
}
