# ProtoVoy Viva + Go

Extract this archive to a directory you can write to. Run the commands below
from that directory. On Windows use PowerShell and add `.exe` to the executable
names.

## Desktop

Run `./workbench` on Linux or `./workbench.exe` on Windows. On macOS, open
`ProtoVoy.app`. The macOS bundle currently has no Developer ID signature or
notarization.

Choose **HTTP**, enter the URL of an HTTP service you control, and select
**Send**.

## Browser

Run `./server -web ./web` and open `http://127.0.0.1:39300/`. For execution
through the server, add `-target` and an HTTP URL configured for this instance.

To use the Local Agent, set `PROTOVOY_AGENT_TOKEN` to a random token and run
`./agent` in another terminal. In the workbench, open **Local Agent**, enter
`127.0.0.1:39217` and the same token, and select **Pair with Local Agent**.

For remote hosting, see the HTTPS and authentication settings in the
[Viva + Go guide](https://github.com/aiLi0617/ProtoVoy/tree/main/apps/protovoy-go).

## CLI

Run `./protovoy -h` for request options or
`./protovoy run PROJECT.json` to execute an exported collection.

Use `-version` with `protovoy`, `agent` or `server` to check the build.

## Mock server

Run `./mock -rules mock-rules.json` to serve a JSON array of scenarios on
`http://127.0.0.1:39218`. Use `./mock -version` to check the build.
For example, save the following array as `mock-rules.json`:

```json
[{"id":"hello","method":"GET","path":"/hello","status":200,"body":"Hello"}]
```

Open `http://127.0.0.1:39218/m1/default-project/default/hello`.
The server reloads valid edits to the scenario file as requests arrive.
Run `./mock -rules mock-rules.json -check` to validate scenarios before starting.
