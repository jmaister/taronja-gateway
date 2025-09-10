# Logging HTTP Requests in Go

## Is it useful?
Logging requests is very useful for debugging, monitoring, auditing, and performance analysis. It helps track errors, usage patterns, and security incidents.

## Better ways to log requests
- Use structured logging (e.g., with logrus, zap, zerolog) instead of plain text logs. This makes logs easier to parse and query.
- Log relevant fields: method, path, status, latency, user agent, IP, request ID, etc.
- Avoid logging sensitive data (e.g., passwords, tokens).

## Storing logs in a file
You can store logs in a file. Use a logging library that supports file output and log rotation.

## Log rotation and querying
- Use libraries like [lumberjack](https://github.com/natefinch/lumberjack) for rolling log files.
- For querying, consider shipping logs to a log management system (e.g., ELK stack, Loki, or even simple grep/awk for local files).

### Example: Using logrus with lumberjack for rolling logs
```go
import (
    "github.com/sirupsen/logrus"
    "gopkg.in/natefinch/lumberjack.v2"
)

func setupLogger() {
    logrus.SetOutput(&lumberjack.Logger{
        Filename:   "/var/log/taronja-gateway/access.log",
        MaxSize:    10, // megabytes
        MaxBackups: 3,
        MaxAge:     28, // days
        Compress:   true,
    })
    logrus.SetFormatter(&logrus.JSONFormatter{})
}
```
You can then log requests in your middleware using `logrus.WithFields`.

## Summary
- Logging requests is useful.
- Use structured logging and a rolling log file.
- For querying, use log management tools or simple command-line utilities.
