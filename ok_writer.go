package oklog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

// Writer implements io.Writer and is used to send both discrete messages to a OK Log server, or data from a
// stream-oriented interface (like the functions in log).
type Writer struct {
	mu   sync.Mutex
	conn net.Conn
}

// Message represents the contents of the OK Log message.
type Message struct {
	Version  string
	Host     string
	Short    string
	Full     string
	TimeUnix float64
	Level    string
	Extra    map[string]interface{}
}

const delimiter = '\n'

func init() {
	rand.Seed(time.Now().UnixNano())
}

// NewWriter returns a new OK Log Writer.  This writer can be used to send the output of the standard Go log functions
// to a randomly choosed ingest OK Log server by passing it to log.SetOutput()
func NewWriter(addr []string) (*Writer, error) {
	var err error
	w := new(Writer)

	addrIdx := rand.Intn(len(addr))
	if w.conn, err = net.Dial("tcp", addr[addrIdx]); err != nil {
		return nil, err
	}

	return w, nil
}

// WriteMessage sends the specified message to the OK Log server specified and chosen in the call to New().  It assumes
// all the fields are filled out appropriately.  In general, clients will want to use Write, rather than WriteMessage.
func (w *Writer) WriteMessage(m *Message) (err error) {
	mBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}

	buff := bytes.NewBuffer(mBytes)
	buff.Grow(1)
	buff.WriteByte(delimiter)
	record := buff.String()

	w.mu.Lock()
	n, err := w.conn.Write([]byte(record))
	if err != nil {
		log.Printf("error: %v", err)
		_ = w.conn.Close()
	}
	w.mu.Unlock()

	if n != len(record) {
		return fmt.Errorf("bad write (%d/%d)", n, len(mBytes))
	}

	return nil
}
