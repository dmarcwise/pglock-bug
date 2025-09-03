package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cirello.io/pglock"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	var dsn = "postgres://postgres:@localhost:5433/caddy"
	config, err := pgxpool.ParseConfig(dsn)
	must(err)

	// Create a custom context for the heartbeat, to be able to cancel it from outside pglock
	heartbeatContext, cancelHeartbeatSqlCommand := context.WithCancel(context.Background())

	// Set up the pgx query tracer, which allows us to:
	// - Log queries as they happen
	// - Intercept queries and trigger heartbeat query cancel at the right time
	config.ConnConfig.Tracer = tracer{cancel: cancelHeartbeatSqlCommand}

	// This disables statement cache, it makes it easier to see the commands in Wireshark
	// In our case this means: one TCP packet for the request + one TCP packet for the response (delayed by toxiproxy)
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	db, err := pgxpool.NewWithConfig(context.Background(), config)
	must(err)
	defer db.Close()

	client, err := pglock.UnsafeNew(
		stdlib.OpenDBFromPool(db),
	)

	must(err)
	_ = client.DropTable()
	must(client.CreateTable())

	// Acquire the lock
	lock, err := client.AcquireContext(
		context.Background(),
		"test",
		pglock.WithCustomHeartbeatContext(heartbeatContext),
	)
	must(err)

	// Wait a bit before unlocking, to ensure the first heartbeat runs
	time.Sleep(500 * time.Millisecond)

	// Alternative way to reproduce the issue:
	// - Remove the `t.cancel()` in `TraceQueryStart`
	// - Play with the sleep above until you find a value that causes the Release to happen after
	//   the heartbeat has started but before the SQL command response is travelling.
	//   On my machine, a 200ms value works

	// Release the lock and expect it to fail
	err = lock.Close()

	fmt.Println()

	switch {
	case err == nil:
		fmt.Println("OK: unlocked ok")
	case errors.Is(err, pglock.ErrLockAlreadyReleased):
		fmt.Println("!!! unlocked already released")
	default:
		fmt.Printf("!!! unlocked other error: %v\n", err)
	}
}

type tracer struct {
	cancel context.CancelFunc
}

func (t tracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	fmt.Println("query start:", data.SQL, data.Args)

	// If the query is the heartbeat UPDATE, cancel the context to stop the query
	if strings.HasPrefix(strings.TrimSpace(data.SQL), "UPDATE") {
		// We wait 10 milliseconds before cancelling, to ensure the command is already sent to
		// the server and executed (this happens without delay very quickly if postgres is local),
		// but the response is still travelling (toxiproxy adds 100ms delay)
		time.AfterFunc(10*time.Millisecond, func() {
			fmt.Println("cancelling heartbeat context")
			t.cancel()
		})
	}

	return ctx
}

func (t tracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	fmt.Printf("query end: %s %v\n", data.CommandTag, data.Err)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
