package docker

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

var clickhouseHost string

func StartClickHouseServer(c context.Context) (string, error) {
	if err := Installed(); err != nil {
		return "", err
	}
	if clickhouseHost != "" {
		return clickhouseHost, nil
	}
	value, err, _ := flight.Do("clickhouse", func() (interface{}, error) {
		host, err := startClickHouseServer(c)
		if err != nil {
			return "", err
		}
		clickhouseHost = host
		return host, err
	})
	if err != nil {
		return "", err
	}
	data, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("returned value was not a string")
	}
	return data, nil
}

func startClickHouseServer(c context.Context) (string, error) {
	// Try docker-compose ClickHouse first (standard port 9000)
	// Try with and without credentials
	dsns := []string{
		"clickhouse://default:@localhost:9000/default",  // default user with empty password
		"clickhouse://localhost:9000/default",            // no credentials
	}

	for _, dsn := range dsns {
		if tryConnectClickHouse(c, dsn, 5*time.Second) {
			slog.Info("found running docker-compose clickhouse", "dsn", dsn)
			return dsn, nil
		}
	}

	// If docker-compose ClickHouse is not available, return an error
	// Don't try to start our own container - ports may be in use
	return "", fmt.Errorf("clickhouse is not available on port 9000. Make sure docker-compose is running or start ClickHouse manually")
}

func tryConnectClickHouse(c context.Context, dsn string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(c, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false

		case <-ticker.C:
			db, err := sql.Open("clickhouse", dsn)
			if err != nil {
				slog.Debug("sqltest clickhouse open failed", "err", err)
				continue
			}

			if err := db.PingContext(ctx); err != nil {
				slog.Debug("sqltest clickhouse ping failed", "err", err)
				db.Close()
				continue
			}

			db.Close()
			return true
		}
	}
}


