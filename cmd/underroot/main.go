package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/Adhithya-J/underroot.git/internal/executor"
	"github.com/Adhithya-J/underroot.git/internal/ui"
)

const (
	maxRetries = 3
)

func main() {
	app := NewApp() // session starts with a fresh session state

	ui.PrintBanner()

	for { // outer loop of ui for a session
		UpdateWorkingDir(app.Session) // adds current dir path, current folder name, parent dir path, current dir contents to session state
		ui.PrintPromptBar(app.Session.ParentDir, app.Session.FolderName, app.Session.Model, app.Session.TotalTokens())

		// Each query starts with no active transient error
		app.Session.ClearActiveError()

		txt, err := ui.ReadInput(app.Scanner)
		if err != nil {
			if err == io.EOF {
				break
			}
			ui.PrintError("User input read failed", err)
			continue
		}

		app.Session.CurrentInput = txt

		if app.Session.CurrentInput == "" {
			continue
		}

		if ui.ShouldExit(app.Session.CurrentInput) {
			break
		}

		app.Session.ResetRequestState()
		for i := 0; i < maxRetries; i++ {
			app.Session.RetryCount = i
			ui.PrintRetry(app.Session.RetryCount, maxRetries)

			input, err := BuildPrompt(app.Session)
			if err != nil {
				ui.PrintError("Error building prompt", err)
				break
			}
			ui.PrintGray(fmt.Sprintf("Request context: %d/4096 estimated tokens", EstimatePromptTokens(input)))
			ui.PrintGray("Generating response...")
			response, errType, runErr := app.Agent.Run(input)
			if runErr != nil {
				ui.PrintError("Agent run failed", runErr)
				app.Session.SetActiveError(errType, runErr.Error(), "")
				continue
			}
			ui.PrintLine()

			ui.PrintScript(response.Script)
			ui.PrintExplanation(response.Explanation)
			ui.PrintLine()

			ui.PrintGray("Dry-running script with PowerShell WhatIf...")
			dryRunOut, dryRunErr := executor.DryRunScript(response.Script)
			if dryRunErr != nil {
				ui.PrintError("Dry run failed", dryRunErr)
				ui.PrintOutput(dryRunOut)
				errMsg := dryRunErr.Error()
				if trimmed := strings.TrimSpace(dryRunOut); trimmed != "" {
					errMsg += ": " + trimmed
				}
				app.Session.SetActiveError("DryRunFailed", errMsg, response.Script)
				continue
			}
			ui.PrintOutput(dryRunOut)

			permitted, err := ui.AskForApproval(app.Scanner)
			if err != nil {
				ui.PrintError("Skipping execution....\nEncountering error", err)
				app.Session.ClearActiveError()
				break
			}
			if permitted {
				ui.PrintGray("Executing Script....")
				psOut, err := executor.ExecuteScript(response.Script)
				if err != nil {
					ui.PrintError("Execution Error", err)
					ui.PrintOutput(psOut)
					errMsg := err.Error()
					if trimmed := strings.TrimSpace(psOut); trimmed != "" {
						errMsg += ": " + trimmed
					}
					app.Session.SetActiveError("ExecutionFailed", errMsg, response.Script)
					continue
				}

				// On success: store lean interaction and clear active error
				interaction := Interaction{
					UserInput:   app.Session.CurrentInput,
					FinalScript: response.Script,
				}
				app.Session.AddInteraction(interaction)
				app.Session.ClearActiveError()

				ui.PrintOutput(psOut)
				fmt.Println("Success!")
				fmt.Printf("History size: %d\n", len(app.Session.History))
				ui.PrintLine()

				break
			} else {
				ui.PrintGray("Skipping execution")
				app.Session.ClearActiveError()
				ui.PrintLine()
				break
			}
		}

		// Ensure transient error is not carried to the next query
		app.Session.ClearActiveError()
	}
}
