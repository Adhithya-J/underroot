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

func handleExecutionFailure(session *Session, errType string, err error, output, script string) bool {
	ui.PrintError(errType, err)
	ui.PrintOutput(output)
	if executor.IsPermissionDenied(err) {
		ui.PrintGray("Permission Denied: This operation requires elevated permissions.")
		ui.PrintGray("Please run Underroot in an elevated (Administrator) PowerShell prompt or adjust folder permissions.")
		session.ClearActiveError()
		return true // Terminate retry loop on elevation requirement
	}
	errMsg := err.Error()
	if trimmed := strings.TrimSpace(output); trimmed != "" {
		errMsg += ": " + trimmed
	}
	session.SetActiveError(errType, errMsg, script)
	return false // Continue retry loop
}

func main() {
	app := NewApp() // session starts with a fresh session state

	ui.PrintBanner()

	for { // outer loop of ui for a session
		UpdateWorkingDir(app.Session)
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

			agentCtx := BuildContext(app.Session)
			ui.PrintGray(fmt.Sprintf("Request context: %d/4096 estimated tokens", EstimatePromptTokens(agentCtx)))
			ui.PrintGray("Generating response...")
			response, errType, runErr := app.Agent.Run(agentCtx)
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
				if handleExecutionFailure(app.Session, "DryRunFailed", dryRunErr, dryRunOut, response.Script) {
					break
				}
				continue
			}
			ui.PrintOutput(dryRunOut)

			permitted, err := ui.AskForApproval(app.Scanner)
			if err != nil {
				ui.PrintError("Skipping execution....\nEncountering error", err)
				app.Session.ClearActiveError()
				break
			}
			if !permitted {
				ui.PrintGray("Skipping execution")
				app.Session.ClearActiveError()
				ui.PrintLine()
				break
			}

			ui.PrintGray("Executing Script....")
			psOut, err := executor.ExecuteScript(response.Script)
			if err != nil {
				if handleExecutionFailure(app.Session, "ExecutionFailed", err, psOut, response.Script) {
					break
				}
				continue
			}

			// On success: store lean interaction and clear active error
			app.Session.AddInteraction(Interaction{
				UserInput:   app.Session.CurrentInput,
				FinalScript: response.Script,
			})
			app.Session.ClearActiveError()

			ui.PrintOutput(psOut)
			fmt.Println("Success!")
			fmt.Printf("History size: %d\n", len(app.Session.History))
			ui.PrintLine()
			break
		}

		// Ensure transient error is not carried to the next query
		app.Session.ClearActiveError()
	}
}
