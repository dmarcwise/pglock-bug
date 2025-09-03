Ref: https://github.com/cirello-io/pglock/issues/175

Steps:

- Start the toxiproxy to simulate network latency to the database instance:

```shell
docker compose up
```

(add `-d` to run in detached mode, and then stop with `docker compose down`)

- Set up the proxy to introduce 100ms latency on response traffic from PostgreSQL:

```shell
docker compose exec toxiproxy /toxiproxy-cli create --listen localhost:5433 --upstream localhost:5432 postgres
docker compose exec toxiproxy /toxiproxy-cli toxic add --type latency -a latency=100 postgres
# Verify with:
docker compose exec toxiproxy /toxiproxy-cli inspect postgres
```

- Run the Go application and make sure the `dsn` variable in `main.go` is set to use the proxy port (5433).

```shell
go run main.go
```

- Observe the output.

## Example output

```
query start: DROP TABLE locks []
query end: DROP TABLE <nil>
query start: CREATE TABLE  locks (
	name CHARACTER VARYING(255) PRIMARY KEY,
	record_version_number BIGINT,
	data BYTEA,
	owner CHARACTER VARYING(255)
) []
query end: CREATE TABLE <nil>
query start: CREATE SEQUENCE  locks_rvn CYCLE OWNED BY locks.record_version_number []
query end: CREATE SEQUENCE <nil>
query start: SELECT nextval('locks_rvn') [map[16:1 17:1 20:1 21:1 23:1 26:1 28:1 29:1 700:1 701:1 1082:1 1114:1 1184:1]]
query end: SELECT 1 <nil>
query start:
		INSERT INTO locks
			("name", "record_version_number", "data", "owner")
		VALUES
			($1, $2, $3, $6)
		ON CONFLICT ("name") DO UPDATE
		SET
			"record_version_number" = CASE
				WHEN COALESCE(locks."record_version_number" = $4, TRUE) THEN $2
				ELSE locks."record_version_number"
			END,
			"data" = CASE
				WHEN COALESCE(locks."record_version_number" = $4, TRUE) THEN
					CASE
						WHEN $5 THEN $3
						ELSE locks."data"
					END
				ELSE locks."data"
			END,
			"owner" = CASE
				WHEN COALESCE(locks."record_version_number" = $4, TRUE) THEN $6
				ELSE locks."owner"
			END
		RETURNING
			"record_version_number", "data", "owner"
	 [map[16:1 17:1 20:1 21:1 23:1 26:1 28:1 29:1 700:1 701:1 1082:1 1114:1 1184:1] test 1 [] 0 false pglock-2354866237003592922]
query end: INSERT 0 1 <nil>
query start: SELECT nextval('locks_rvn') [map[16:1 17:1 20:1 21:1 23:1 26:1 28:1 29:1 700:1 701:1 1082:1 1114:1 1184:1]]
query end: SELECT 1 <nil>
query start:
		UPDATE
			locks
		SET
			"record_version_number" = $3
		WHERE
			"name" = $1
			AND "record_version_number" = $2
	 [test 1 2]
cancelling heartbeat context
query end:  context canceled
query start:
			DELETE FROM
				locks
			WHERE
				"name" = $1
				AND "record_version_number" = $2
		 [test 1]
query end: DELETE 0 <nil>

!!! unlocked already released
```
