# underroot

`underroot` is an interactive Windows assistant that converts natural-language requests into PowerShell scripts. It shows the generated script and explanation, validates the request, and asks for confirmation before execution.

## Features

- Natural-language file and directory management
- AI-generated PowerShell scripts
- Tool-assisted directory listing, file reading, and path checks
- Verified known folders for Desktop, Downloads, Documents, Pictures, Videos, and Music
- AI, rule-based, and PowerShell AST safety checks
- Session-level execution permissions
- Automatic retries for generation and execution errors
- Failure summaries that stay in the session context
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

### Interactive commands

- `/init` — resolve your standard Windows folders and write a verified manifest.
- `/permissions` — show the session permission mode.
- `/permissions ask|auto|deny` — change how scripts are approved.
- `/permissions reset` — forget remembered decisions and return to ask mode.
- `/help` — list commands.

### Known folders

On first run Underroot resolves the current user's Desktop, Downloads, Documents, Pictures, Videos, and Music folders through Windows' Shell API, verifies each path, and writes `.underroot/known-folders.md` in your user configuration directory. Later requests use those verified paths without filesystem discovery. If a folder moves or disappears, the next startup re-resolves it. The manifest stores only local paths and verification metadata.

## Safety

Generated scripts can modify files and run commands with the permissions of the current user. Review every script and its effects before approving execution. The safety checks are safeguards, not a guarantee that every request or script is safe.

## Development

Run the test suite with:

```powershell
go test ./...
```

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
