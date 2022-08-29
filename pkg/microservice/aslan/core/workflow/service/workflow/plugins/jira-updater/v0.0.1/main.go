package main

import (
	"fmt"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/viper"

	"github.com/koderover/zadig/pkg/tool/log"
)

// envs
const (
	JiraAddress  = "JIRA_ADDRESS"
	Username     = "USERNAME"
	Password     = "PASSWORD"
	IssueID      = "ISSUE_ID"
	TargetStatus = "TARGET_STATUS"
)

func main() {
	log.Init(&log.Config{
		Level:       "info",
		Development: false,
		MaxSize:     5,
	})
	viper.AutomaticEnv()

	addr := viper.GetString(JiraAddress)
	issueID := viper.GetString(IssueID)
	username := viper.GetString(Username)
	password := viper.GetString(Password)
	status := viper.GetString(TargetStatus)

	log.Infof("executing jira status update to %s for issue: %s on server %s", status, issueID, addr)

	tp := jira.BasicAuthTransport{
		Username: username,
		Password: password,
	}

	jiraclient, err := jira.NewClient(tp.Client(), addr)
	if err != nil {
		fmt.Printf("failed to create JIRA client, error: %s\n", err)
		return
	}

	var transitionID string

	possibleTransitions, _, err := jiraclient.Issue.GetTransitions(issueID)
	if err != nil {
		fmt.Printf("failed to get possible transitions, err: %s\n", err)
		return
	}

	for _, possibleTransition := range possibleTransitions {
		if possibleTransition.Name == status {
			transitionID = possibleTransition.ID
			break
		}
	}

	if transitionID == "" {
		fmt.Printf("no transition of name %s found, check if the target status exist\n", status)
		return
	}

	_, err = jiraclient.Issue.DoTransition(issueID, transitionID)
	if err != nil {
		fmt.Printf("failed to do change status, err: %s\n", err)
		return
	}

	log.Infof("Jira status update complete")
}
