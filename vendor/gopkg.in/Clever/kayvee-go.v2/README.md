# kayvee
--
    import "gopkg.in/Clever/kayvee-go.v2"

Package kayvee provides methods to output human and machine parseable strings,
with a "json" format.

## [Logger API Documentation](./logger)

## Example

```go
    package main

    import(
        "fmt"
        "time"

        "gopkg.in/Clever/kayvee-go.v2/logger"
    )

    func main() {
        myLogger := logger.New("myApp")

        // Simple debugging
        myLogger.Debug("Service has started")

        // Make a query and log its length
        query_start := time.Now()
        myLogger.GaugeFloat("QueryTime", time.Since(query_start).Seconds())

        // Output structured data
        myLogger.InfoD("DataResults", map[string]interface{}{"key": "value"})
    }
```


## Testing

Run `make test` to execute the tests

## Change log

v2.1 - Add kayvee-go/logger with log level, counters, and gauge support
v0.1 - Initial release.

## Backward Compatibility

The kayvee 1.x interface still exist but is considered deprecated. You can find documentation on using it in the [compatibility guide](./compatibility.md)

