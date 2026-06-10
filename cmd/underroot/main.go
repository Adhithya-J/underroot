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

	app := NewApp() //  session starts with a fresh session state

	ui.PrintBanner()

	for { // outer loop of ui for a session
		UpdateWorkingDir(app.Session) // adds current dir path, current folder name, parent dir path, current dir contents to session state, to help llm better navigate the filesystem
		ui.PrintPromptBar(app.Session.ParentDir, app.Session.FolderName, app.Session.Model, app.Session.TotalTokens())

		// each query in a session starts with a fresh error history (empty)
		// this was done to keep session ctx window shorter (since we are dealing with SLMs), however, keeping the error history across the session would be useful to avoid recurring errors, essentially we are not allowing the model to do good ICL of the environment
		app.Session.ErrorHistory = nil

		txt, err := ui.ReadInput(app.Scanner)

		if err != nil {
			ui.PrintError("User input read failed", err)
			continue
		}

		app.Session.CurrentInput = txt // should current input be added to session state as a separate variable?

		if app.Session.CurrentInput == "" {
			continue
		}

		if ui.ShouldExit(app.Session.CurrentInput) {
			break
		}
		var attemptStatus bool = false // this tracks whether user question is successfully answered after all retries
		// depending on the value, what gets added to session state ctx varies
		// previously, we were was only storing successful attempts, then realized, unsuccessful attempts will also be useful
		// but what to store in unsuccessful attempts? - the error message, but how would you structure it, i haven't explore on that,
		// TODO: how to structure errors to add them to context for failed attempts?

		app.Session.ResetRequestState()
		for i := 0; i < maxRetries; i++ {
			app.Session.RetryCount = i
			ui.PrintRetry(app.Session.RetryCount, maxRetries)

			input, err := BuildPrompt(app.Session) // what is fed into llm prompt? a structured version of conversation history with suggested fix for errors
			if err != nil {
				ui.PrintError("Error parsing json", err)
			}
			ui.PrintGray("Generating response...")
			response, errType, runErr := app.Agent.Run(input) // what does this do?
			if runErr != nil {
				ui.PrintError("Agent run failed", runErr)
				app.Session.AddErrorHistory(AgentError{
					ErrorType: errType,
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
					ui.PrintOutput(psOut)
					app.Session.AddErrorHistory(AgentError{
						ErrorType: "ExectionFailed",
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
				attemptStatus = true
				fmt.Printf("History size: %d\n", len(app.Session.History))
				ui.PrintLine()

				break
			} else {
				ui.PrintGray("Skipping execution")
				ui.PrintLine()
				break

			}
		}
		// adding failed attempt flags to llm knows what the previous requests were
		if !attemptStatus {
			app.Session.AddInteraction(Interaction{
				UserInput:   app.Session.CurrentInput,
				Explanation: "",
				Script:      "",
				Output:      "",
			})

		}
	}

	ui.PrintLine()

}
