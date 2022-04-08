/*
Copyright 2022 The KodeRover Authors.

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

package open

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/koderover/zadig/pkg/microservice/aslan/core/code/client"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/code/client/codehub"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/code/client/gerrit"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/code/client/gitee"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/code/client/github"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/code/client/gitlab"
	"github.com/koderover/zadig/pkg/setting"
	"github.com/koderover/zadig/pkg/shared/client/systemconfig"
)

type ClientConfig interface {
	Open(id int, logger *zap.SugaredLogger) (client.CodeHostClient, error)
}

var ClientsConfig = map[string]func() ClientConfig{
	setting.SourceFromGitlab:  func() ClientConfig { return new(gitlab.Config) },
	setting.SourceFromGithub:  func() ClientConfig { return new(github.Config) },
	setting.SourceFromGerrit:  func() ClientConfig { return new(gerrit.Config) },
	setting.SourceFromCodeHub: func() ClientConfig { return new(codehub.Config) },
	setting.SourceFromGitee:   func() ClientConfig { return new(gitee.Config) },
}

func OpenClient(codehostID int, log *zap.SugaredLogger) (client.CodeHostClient, error) {
	ch, err := systemconfig.New().GetCodeHost(codehostID)
	if err != nil {
		return nil, err
	}

	var c client.CodeHostClient
	f, ok := ClientsConfig[ch.Type]
	if !ok {
		return c, fmt.Errorf("unknow codehost type")
	}
	clientConfig := f()
	bs, err := json.Marshal(ch)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(bs, clientConfig); err != nil {
		return nil, err
	}
	return clientConfig.Open(codehostID, log)
}
