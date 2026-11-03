# Catchup

Catchup starts out as a local MCP server for Google Classroom but more integrations will be added later on top of Classroom.

## Status

The project is in early development and is not a high priority. Thus, it might be scrapped.

## Planned tools

| Tool | Question it answers |
|---|---|
| `list_classes` | Which classes am I in? |
| `list_topics` | Which topics does a class have? |
| `what_did_i_miss` | What was posted or due while I was away? |
| `pending_work` | Which coursework is due soon or overdue? |
| `search_resources` | Where is the file or link about a subject? |
| `read_attachment` | What does an attached file say? |
| `missing_submissions` | Which students have outstanding work? |

Each install runs in student mode or teacher mode. The mode decides which tools are available and which Google permissions the server requests.

## Privacy

- The server runs on your machine and calls only Google APIs.
- The server does not log class content.

## Development

The project needs Go 1.27.3 or later. It uses the official MCP Go SDK and follows MCP specification `2026-07-28`.

Run the tests:

```bash
go test ./...
```

Open the server in the MCP Inspector:

```bash
npx @modelcontextprotocol/inspector go run .
```

## License

Apache-2.0
