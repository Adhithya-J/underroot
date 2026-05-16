# underroot

`underroot` is an AI-powered assistant for managing files and folders on Windows. It translates natural language requests into PowerShell scripts and executes them with your approval.

The project features a self-healing loop, meaning if a script encounters an error, the agent analyzes the output and attempts to correct itself automatically.

## Key Features
- **Autonomous Execution:** Generates and runs PowerShell scripts based on your prompts.
- **Self-Healing:** Automatically diagnoses and fixes script errors.
- **Safety First:** Includes both AI-based intent checks and static command validation.
- **Human Approval:** No script is executed without explicit user confirmation.

## Usage

1. **Setup:** Create a `.env` file with your `API_KEY`, `BASE_URL`, and `MODEL`.
2. **Run:**
   ```bash
   go run ./cmd/underroot
   ```
3. **Interact:** Describe the task you want to perform (e.g., "Archive all .log files in the current directory").

## Requirements
- Go 1.26.2+
- Windows (PowerShell)
- OpenAI-compatible API key
