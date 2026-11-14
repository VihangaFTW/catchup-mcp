# Catchup

Catchup starts out as a local MCP server for Google Classroom but more integrations will be added later on top of Classroom.

## Status

The project is in early development and is not a high priority. Thus, it might be scrapped.

The first release supports teacher mode only. Student mode follows in a later release.


## Setup

### Create a Google OAuth client

The server signs in to Google with an OAuth client that you create once.

1. Create a project in the [Google Cloud Console](https://console.cloud.google.com) and enable the Google Classroom API.
2. Configure the OAuth consent screen. If the screen stays in testing mode, add your Google account as a test user.
3. Create an OAuth client ID with the application type "Desktop app".
4. Save the client ID and the client secret in a JSON file outside the repository.

The file is named the center file and has this format:

```json
{
  "client_id": "your-client-id.apps.googleusercontent.com",
  "client_secret": "your-client-secret"
}
```

### Start the server

The server reads its settings from environment variables.

| Variable | Purpose |
|---|---|
| `CATCHUP_CENTER_FILE` | The path to the center file. This variable is required. |
| `CATCHUP_MODE` | The mode of the install. The default value is `teacher` and it is the only supported mode. |
| `CATCHUP_TIMEZONE` | An IANA timezone name such as `Asia/Colombo`. The default value is the local timezone. |

Start the server and keep the terminal open:

```bash
CATCHUP_CENTER_FILE=~/.config/catchup/center.json go run ./cmd/catchup-mcp
```

The server listens on `http://127.0.0.1:8080/mcp`.

### Connect Claude Code

Add the server to Claude Code from a second terminal:

```bash
claude mcp add --scope user --transport http catchup http://127.0.0.1:8080/mcp
```

The `--scope user` option makes the server available in every folder. Without this option, Claude Code loads the server only in the folder where you ran the command.

Start a new Claude Code session after this command. Then confirm that `catchup` shows as connected:

```bash
claude mcp list
```

## Planned tools

| Tool | Question it answers |
|---|---|
| `list_classes` | Which classes am I in? |
| `list_topics` | Which topics does a class have? |
| `what_did_i_miss` | What was posted or due while I was away? |
| `pending_work` | Which coursework is due soon or overdue? |
| `search_resources` | Where is the file or link about a subject? |
| `missing_submissions` | Which students have outstanding work? |

Catchup does not read the contents of attached files. It returns each file's Google Drive ID and link. To read a file, connect a Google Drive tool to your MCP client as well, such as the Google Drive connector in Claude.

Each install runs in student mode or teacher mode. The mode decides which tools are available and which Google permissions the server requests.

## Development

The project needs Go 1.27.3 or later. It uses the official MCP Go SDK and follows MCP specification `2026-07-28`.

Run the tests:

```bash
go test ./...
```

## License

[Apache-2.0](LICENCE.md)
