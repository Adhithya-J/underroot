package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/Adhithya-J/underroot.git/internal/agent"
	"github.com/Adhithya-J/underroot.git/internal/executor"
	"github.com/Adhithya-J/underroot.git/internal/intent"
	"github.com/Adhithya-J/underroot.git/internal/ui"
)

const (
	maxRetries = 3
)

func handleExecutionFailure(session *Session, errType string, err error, output, script string) bool {
	ui.PrintError(errType, err)
	ui.PrintOutput(output)

	errMsg := err.Error()
	if trimmed := strings.TrimSpace(output); trimmed != "" {
		errMsg += ": " + trimmed
	}
	session.SetActiveError(errType, errMsg, script)

	if executor.IsPermissionDenied(err) {
		ui.PrintGray("Permission Denied: This operation requires elevated permissions.")
		ui.PrintGray("Please run Underroot in an elevated (Administrator) PowerShell prompt or adjust folder permissions.")
		return true // Terminate retry loop on elevation requirement
	}
	return false // Continue retry loop
}

func handleShellCommand(app *App, input string) (handled, exit bool) {
	if !strings.HasPrefix(input, "/") {
		return false, false
	}

	fields := strings.Fields(input)
	switch strings.ToLower(fields[0]) {
	case "/exit", "/quit", "/q":
		return true, true
	case "/init":
		app.LoadKnownFolders(true)
	case "/permissions", "/perms":
		handlePermissionsCommand(app.Session, fields[1:])
	case "/todo", "/objectives":
		ui.PrintTodo(app.Session.ObjectiveRecitation())
	case "/failures":
		ui.PrintFailures(GetFailureHistory(app.Session))
	case "/help":
		ui.PrintHelp()
	default:
		ui.PrintGray("Unknown command: " + fields[0] + ". Type /help for available commands.")
	}
	return true, false
}

func handlePermissionsCommand(session *Session, args []string) {
	if len(args) == 0 {
		allowed, denied := session.Permissions.Counts()
		allowedRules, disallowedRules := session.Permissions.ToolRules()
		ui.PrintPermissions(session.Permissions.Mode().String(), allowed, denied, allowedRules, disallowedRules)
		return
	}

	switch strings.ToLower(args[0]) {
	case "reset":
		session.Permissions.Reset()
		ui.PrintGray("Session permissions reset to ask mode.")
		return
	case "rules":
		allowedRules, disallowedRules := session.Permissions.ToolRules()
		ui.PrintPermissions(session.Permissions.Mode().String(), 0, 0, allowedRules, disallowedRules)
		return
	case "allow":
		if len(args) < 2 {
			ui.PrintGray("Usage: /permissions allow <command-pattern>")
			return
		}
		if err := session.Permissions.AllowTool(strings.Join(args[1:], " ")); err != nil {
			ui.PrintGray("Invalid permission rule: " + err.Error())
			return
		}
		ui.PrintGray("Added allowed_tools rule: " + strings.Join(args[1:], " "))
		return
	case "disallow":
		if len(args) < 2 {
			ui.PrintGray("Usage: /permissions disallow <command-pattern>")
			return
		}
		if err := session.Permissions.DisallowTool(strings.Join(args[1:], " ")); err != nil {
			ui.PrintGray("Invalid permission rule: " + err.Error())
			return
		}
		ui.PrintGray("Added disallowed_tools rule: " + strings.Join(args[1:], " "))
		return
	}

	mode, ok := ParsePermissionMode(args[0])
	if !ok {
		ui.PrintGray("Usage: /permissions [ask|auto|deny|allow <rule>|disallow <rule>|rules|reset]")
		return
	}

	session.Permissions.SetMode(mode)
	ui.PrintGray("Session permission mode set to " + mode.String() + ".")
	if mode == PermissionAuto {
		ui.PrintGray("Safety checks and dry runs still run before auto-approved execution.")
	}
}

func main() {
	app := NewApp() // session starts with a fresh session state

	ui.PrintBanner()
	app.LoadKnownFolders(false)

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

		if handled, exit := handleShellCommand(app, app.Session.CurrentInput); handled {
			if exit {
				break
			}
			continue
		}

		if err := intent.Validate(app.Session.CurrentInput); err != nil {
			ui.PrintError("Invalid request", err)
			continue
		}

		if app.Session.Objectives != nil {
			if err := app.Session.Objectives.Begin(app.Session.CurrentInput); err != nil {
				ui.PrintGray("Objective ledger unavailable: " + err.Error())
			}
		}

		app.Session.ResetRequestState()
		succeeded := false

	retryLoop:
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
			if response.NextStep.Type == agent.NextStepInterruption {
				ui.PrintGray("Tool approval interrupted the run: " + response.NextStep.Reason)
				app.Session.SetActiveError("ToolApprovalRequired", response.NextStep.Reason, "")
				continue retryLoop
			}
			ui.PrintLine()

			ui.PrintScript(response.Script)
			ui.PrintExplanation(response.Explanation)
			ui.PrintLine()

			ui.PrintGray("Dry-running script with PowerShell WhatIf...")
			dryRunOut, dryRunErr := executor.DryRunScript(response.Script)
			if dryRunErr != nil {
				if handleExecutionFailure(app.Session, "DryRunFailed", dryRunErr, dryRunOut, response.Script) {
					break retryLoop
				}
				continue
			}
			ui.PrintOutput(dryRunOut)

			decision, reason := app.Session.Permissions.Evaluate(response.Script)
			switch decision {
			case DecisionDeny:
				ui.PrintGray("Execution blocked by session permissions: " + reason)
				// A permission denial is a tool result, not a dead end: keep the
				// evidence in context so the model can try a compliant alternative.
				app.Session.SetActiveError("PermissionDenied", reason, response.Script)
				continue retryLoop
			case DecisionAllow:
				ui.PrintGray("Session permissions auto-approved this script.")
			default:
				decision, approvalErr := ui.AskForApproval(app.Scanner)
				if approvalErr != nil {
					ui.PrintError("Skipping execution....\nEncountering error", approvalErr)
					app.Session.ClearActiveError()
					break retryLoop
				}

				switch decision {
				case ui.ApprovalSkip:
					ui.PrintGray("Skipping execution")
					app.Session.ClearActiveError()
					ui.PrintLine()
					break retryLoop
				case ui.ApprovalAlways:
					app.Session.Permissions.Allow(response.Script)
					ui.PrintGray("This script is approved for the rest of the session.")
				case ui.ApprovalNever:
					app.Session.Permissions.Deny(response.Script)
					ui.PrintGray("This script is blocked for the rest of the session.")
					app.Session.SetActiveError("PermissionDenied", "the user blocked this exact script for the session", response.Script)
					continue retryLoop
				}
			}

			ui.PrintGray("Executing Script....")
			psOut, err := executor.ExecuteScript(response.Script)
			if err != nil {
				if handleExecutionFailure(app.Session, "ExecutionFailed", err, psOut, response.Script) {
					break retryLoop
				}
				continue
			}

			// On success: store lean interaction and clear active error
			app.Session.AddInteraction(Interaction{
				UserInput:   app.Session.CurrentInput,
				FinalScript: response.Script,
			})
			app.Session.ClearActiveError()
			succeeded = true
			if app.Session.Objectives != nil {
				_ = app.Session.Objectives.Complete(app.Session.CurrentInput, "executed successfully")
			}

			ui.PrintOutput(psOut)
			fmt.Println("Success!")
			fmt.Printf("History size: %d\n", len(app.Session.History))
			ui.PrintLine()
			break retryLoop
		}

		// Preserve evidence when every attempt failed; the model sees this summary
		// in later requests instead of silently losing the failure.
		if !succeeded && app.Session.ActiveError != nil {
			active := *app.Session.ActiveError
			app.Session.AddFailure(Failure{
				UserInput: app.Session.CurrentInput,
				Attempts:  app.Session.RetryCount + 1,
				ErrorType: active.ErrorType,
				ErrorMsg:  active.ErrorMsg,
				Script:    active.Script,
			})
			if app.Session.Objectives != nil {
				_ = app.Session.Objectives.Fail(
					app.Session.CurrentInput,
					fmt.Sprintf("%s: %s", active.ErrorType, truncateText(active.ErrorMsg, 160)),
				)
			}
			ui.PrintGray("All retries failed. A failure summary was added to session context.")
		}

		// Ensure transient error is not carried to the next query
		app.Session.ClearActiveError()
	}
}
