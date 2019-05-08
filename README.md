# OK Log Hook for [Logrus](https://github.com/sirupsen/logrus)  <img src="http://i.imgur.com/hTeVwmJ.png" width="40" height="40" alt=":walrus:" class="emoji" title=":walrus:" /> [![Build Status](https://travis-ci.org/Otetz/logrus-oklog-hook.svg?branch=master)](https://travis-ci.org/Otetz/logrus-oklog-hook) [![godoc reference](https://godoc.org/github.com/Otetz/logrus-oklog-hook?status.svg)](https://godoc.org/github.com/Otetz/logrus-oklog-hook) [![Go Report Card](https://goreportcard.com/badge/github.com/Otetz/logrus-oklog-hook)](https://goreportcard.com/report/github.com/Otetz/logrus-oklog-hook) [![License](https://img.shields.io/badge/license-MIT-green.svg)](https://github.com/Otetz/logrus-oklog-hook/blob/master/LICENSE.md) [![GolangCI](https://golangci.com/badges/github.com/Otetz/logrus-oklog-hook.svg)](https://golangci.com/r/github.com/Otetz/logrus-oklog-hook) [![codecov](https://codecov.io/gh/Otetz/logrus-oklog-hook/branch/master/graph/badge.svg)](https://codecov.io/gh/Otetz/logrus-oklog-hook)



Use this hook to send your logs to [OK Log](https://github.com/oklog/oklog) ingest server(s) over TCP.

All logrus fields will be sent as additional fields on OK Log.

## Usage

The hook must be configured with:

* A OK Log address (a "ip:port" string).
* an optional map with extra global fields. These fields will be included in all messages sent to OK Log

```go
package main

import (
	"github.com/otetz/logrus-oklog-hook"
	log "github.com/sirupsen/logrus"
)

func main() {
    hook := oklog.NewOklogHook([]string{"<oklog_ip>:<oklog_port>"}, map[string]interface{}{"this": "is logged every time"})
    log.AddHook(hook)
    log.Info("some logging message")
}
```

### Asynchronous logger

```go
package main

import (
    "github.com/otetz/logrus-oklog-hook"
    log "github.com/sirupsen/logrus"
)

func main() {
    hook := oklog.NewAsyncOklogHook([]string{"<oklog_ip>:<oklog_port>"}, map[string]interface{}{"this": "is logged every time"})
    defer hook.Flush()
    log.AddHook(hook)
    log.Info("some logging message")
}
```
