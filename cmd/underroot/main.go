package main

import (
	"fmt"

	"github.com/Adhithya-J/underroot.git/internal/executor"
	"github.com/Adhithya-J/underroot.git/internal/ui"
)

const (
	maxRetries = 3
)

func main() {

	app := NewApp()

	ui.PrintBanner()

	for {
		// print cwd and model-name
		UpdateWorkingDir(app.Session)
		ui.PrintPromptBar(app.Session.ParentDir, app.Session.FolderName, app.Session.Model, app.Session.TotalTokens())

		txt, err := ui.ReadInput(app.Scanner)
		app.Session.ErrorHistory = nil

		if err != nil {
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
				ui.PrintError("Error parsing json", err)
			}
			ui.PrintGray("Generating response...")
			response, runErr := app.Agent.Run(input)
			if runErr != nil {
				ui.PrintError("Agent run failed", runErr)
				app.Session.AddErrorHistory(AgentError{
					ErrorType: "",
					ErrorMsg:  runErr.Error(),
					Script:    "",
					Output:    "",
				})
				continue
			}
			ui.PrintLine()
			app.Session.SetLastResponse(response.Script, response.Explanation)

			ui.PrintScript(app.Session.LastScript)
			ui.PrintExplanation(app.Session.LastExplanation)
			ui.PrintLine()

			permitted, err := ui.AskForApproval(app.Scanner)
			if err != nil {
				ui.PrintError("Skipping execution....\nEncountering error", err)
				break
			}
			if permitted {
				ui.PrintGray("Executing Script....")
				psOut, err := executor.ExecuteScript(response.Script)
				if err != nil {
					ui.PrintError("Execution Error", err)
					app.Session.AddErrorHistory(AgentError{
						ErrorType: "",
						ErrorMsg:  err.Error(),
						Script:    response.Script,
						Output:    psOut,
					})
					continue
				}
				app.Session.SetOutput(psOut)

				interaction := Interaction{
					UserInput:   app.Session.CurrentInput,
					Explanation: response.Explanation,
					Script:      response.Script,
					Output:      psOut,
				}
				app.Session.AddInteraction(interaction)

				ui.PrintOutput(app.Session.LastOutput)
				fmt.Println("Success!")
				fmt.Printf("History size: %d\n", len(app.Session.History))
				ui.PrintLine()

				break
			} else {
				ui.PrintGray("Skipping execution")
				ui.PrintLine()
				break

			}
		}
	}

	ui.PrintLine()

}
