# underroot

`underroot` is an interactive Windows assistant that converts natural-language requests into PowerShell scripts. It shows the generated script and explanation, validates the request, and asks for confirmation before execution.

## Features

- Natural-language file and directory management
- AI-generated PowerShell scripts
- Tool-assisted directory listing, file reading, and path checks
- Verified known folders for Desktop, Downloads, Documents, Pictures, Videos, and Music
- AI, rule-based, and PowerShell AST safety checks
- Session permissions with `allowed_tools`, `disallowed_tools`, and per-command scoping
- API-level structured output with native tool calling and a JSON-prompt fallback
- Request validation for empty, malformed, overlong, and instruction-override input
- Automatic retries for generation and execution errors
- Failure summaries and a persistent `todo.md` objective ledger
- Explicit approval before a script is executed

## Requirements

- Windows with PowerShell available on `PATH`
- Go 1.23 or later
- An OpenAI-compatible API endpoint and API key

## Configuration

Create a `.env` file in the repository root:

```dotenv
OpenAIAPIKey=your-api-key
OpenAIBaseURL=https://api.example.com/v1
OpenAIModel=your-model-name
```

Do not commit `.env` or other credentials to the repository.

Configuration is loaded by `internal/config`. The standard OpenAI variable names
are also accepted. To run fully offline, see [Offline mock](#offline-mock-no-llm-api).

### Permission rules

`allowed_tools` auto-approves matching commands, while `disallowed_tools` blocks
matching commands and overrides allow rules. The session mode (`ask`, `auto`, or
`deny`) is the fallback for everything else. Rules accept a command or a
`PowerShell(Command:argument-glob)` form:

```text
/permissions allow PowerShell(Get-ChildItem:*)
/permissions disallow PowerShell(Remove-Item:C:\Windows\*)
/permissions rules
```

A denied script is fed back into context as evidence so the model can try a
compliant alternative instead of stopping immediately.

## Usage

Start the assistant from the repository root:

```powershell
go run ./cmd/underroot
```

Enter a request at the prompt, such as:

```text
Archive all .log files in the current directory
```

Review the generated PowerShell script and explanation. Confirm with `Y` or press Enter to execute it. Type `exit`, `quit`, or `q` to close the application.

At the approval prompt you can also type `a` to approve the same script for the rest of the session, or `d` to block it for the rest of the session.

## Offline mock (no LLM API)

Two offline modes are available. Both use the same deterministic scenario engine,
so they produce the same discovery calls, scripts, explanations, and safety
verdicts.

### Option 1: in-process mock

```powershell
$env:UNDERROOT_USE_MOCK = "true"
go run ./cmd/underroot
```

No API key, base URL, or model is needed. The status bar shows `mock`.

### Option 2: OpenAI-compatible mock server

This exercises the real HTTP client, structured-output parsing, and native tool
calling.

```powershell
go run ./cmd/mockllm -addr 127.0.0.1:8080 -verbose
```

In another terminal, point the app at it. The repository `.env` already uses
`http://localhost:8080/`:

```dotenv
OpenAIBaseURL=http://127.0.0.1:8080/
OpenAIAPIKey=unused
OpenAIModel=underroot-mock
```

Then run `go run ./cmd/underroot`.

### Good scenarios to try

| Prompt | Exercises |
| --- | --- |
| `list the files in this folder` | Direct final generation |
| `list the files in the projects folder` | Discovery (`search_entries`) and a confirmed path |
| `read README.md` | File discovery, then `Get-Content` |
| `open my Downloads folder` | Verified known folder, no discovery |
| `archive all .log files` | Extension discovery, `Compress-Archive` |
| `delete temporary .tmp files` | Scoped destructive script with a dry-run guard |
| `create a folder called demo` | `New-Item` |
| `count the files here` | Grouping by extension |
| `show the largest files` | Sorting by size |
| `find the budget report` | Recursive search |
| `move all .pdf files to Documents` | Verified destination folder |
| `organize files by extension` | Multi-step mutation |
| `rename notes.txt to journal.txt` | Single-file rename |
| `rename all .txt files to .md` | Extension-wide rename |
| `show disk space` | Drive inventory |
| `list the top processes` | Process inventory |
| `simulate failure` | Self-healing retry loop |
| `delete C:\Windows` | `is_safe: false` AI safety block |

Run mutating scenarios (`delete`, `move`, `organize`, `rename`, `create`,
`archive`) in a scratch directory, because the scripts target the current
working directory after your approval.

The mock server also supports `GET /health`, `GET /v1/models`, and an optional
`-delay 250ms` flag for latency testing.

### Interactive commands

- `/init` — resolve your standard Windows folders and write a verified manifest.
- `/permissions` — show the session permission mode and rules.
- `/permissions ask|auto|deny` — change how unmatched scripts are approved.
- `/permissions allow <rule>` — auto-approve commands matching a rule.
- `/permissions disallow <rule>` — block commands matching a rule (overrides allow).
- `/permissions rules` — show `allowed_tools` and `disallowed_tools`.
- `/permissions reset` — forget remembered decisions and rules.
- `/todo` — show the active objective ledger.
- `/failures` — show durable failure evidence.
- `/help` — list commands.

### Known folders

On first run Underroot resolves the current user's Desktop, Downloads, Documents, Pictures, Videos, and Music folders through Windows' Shell API, verifies each path, and writes `.underroot/known-folders.md` in your user configuration directory. Later requests use those verified paths without filesystem discovery. If a folder moves or disappears, the next startup re-resolves it. The manifest stores only local paths and verification metadata.

## Safety

Generated scripts can modify files and run commands with the permissions of the current user. Review every script and its effects before approving execution. The safety checks are safeguards, not a guarantee that every request or script is safe.

## Objectives and failure evidence

Each request also updates `.underroot/todo.md` in your user configuration directory. The active objective is recited at the end of the model context so long sessions stay on task. Completed, skipped, and failed objectives remain in the ledger; failures include the error type and a short note so the model can avoid repeating an approach that already failed.

## Development

Run the test suite with:

```powershell
go test ./...
```

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
