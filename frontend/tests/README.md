# Settings recovery checks

`npm --prefix frontend run test:settings` runs the policy boundary tests with the
existing TypeScript compiler and Node's built-in test runner. CI runs this command.

For browser verification of the real built page against synthetic API responses:

```sh
npm --prefix frontend run build
python3 frontend/tests/settings-recovery-fixture.py --mode pending
```

Open `http://127.0.0.1:18766/#/settings`. The fixture binds loopback only, serves
the built assets, uses a synthetic administrator and configuration, and never
calls a model, repository, mail, or ticket service. It is a UI fixture, not the
production backend or a production authentication example.

Switch scenarios and reset counters without changing production settings:

```sh
curl -s -X POST http://127.0.0.1:18766/_fixture \
  -H 'Content-Type: application/json' -d '{"mode":"reload_lost"}'
```

Reload the browser after switching. Modes: `pending`, `active`, `unbound`,
`unavailable`, `malformed`, `unsafe`, `get_failure`, `save_pending`,
`reload_lost`, `save_lost`, `success_read_failure`, `timeout`, `forbidden`,
`unauthorized`, `invalid_proxy`. `timeout` accepts one write then delays its
response for 25 seconds; the page should stop waiting after 20 seconds and lock
writes. Lost-response modes accept a write and close the connection.

`GET /_proof` exposes the fixture's GET count and captured synthetic write
requests. Verify that reload has exactly `expected_config_revision` as a string,
PUT excludes `repository_activation`, double clicks/timeouts never produce a
second write, and manual rereads only increase GET count. Use only synthetic
input in this test fixture. `POST /_read_ok` makes reads succeed again without
resetting already accepted writes. Stop the fixture with Ctrl-C after testing.

Browser evidence must distinguish this synthetic UI exercise from the real
Go/SQLite reload HTTP tests; it does not prove production startup or native
repository execution is enabled.
