/*
Copyright 2021 The KodeRover Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gitlab

import (
	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	gitservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/git"
	"github.com/koderover/zadig/pkg/tool/git"
	"github.com/koderover/zadig/pkg/tool/log"
)

func (c *Client) CreateWebHook(owner, repo string) (string, error) {
	_, err := c.AddProjectHook(owner, repo, &git.Hook{
		URL:    config.WebHookURL(),
		Secret: gitservice.GetHookSecret(),
		Events: []string{git.PushEvent, git.PullRequestEvent, git.BranchOrTagCreateEvent},
	})

	return "", err
}

func (c *Client) DeleteWebHook(owner, repo, hookID string) error {
	whs, err := c.ListProjectHooks(owner, repo, nil)
	if err != nil {
		log.Errorf("Failed to list hooks from %s/%s, err: %s", owner, repo, err)
		return err
	}

	for _, wh := range whs {
		// we assume that there is only one webhook matching this url
		if wh.URL == config.WebHookURL() {
			return c.DeleteProjectHook(owner, repo, wh.ID)
		}
	}

	return nil
}
