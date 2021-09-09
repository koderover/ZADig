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

package service

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"strings"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"

	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models/template"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	commonservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service"
	fsservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/fs"
	"github.com/koderover/zadig/pkg/setting"
	e "github.com/koderover/zadig/pkg/tool/errors"
	yaml2 "github.com/koderover/zadig/pkg/util/yaml"
)

type RepoConfig struct {
	CodehostID  int      `json:"codehostID,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	Branch      string   `json:"branch,omitempty"`
	ValuesPaths []string `json:"valuesPaths,omitempty"`
}

type KVPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type RendersetCreateArgs struct {
	EnvName        string      `json:"envName,omitempty,omitempty"`
	ServiceName    string      `json:"serviceName,omitempty"`
	YamlSource     string      `json:"yamlSource,omitempty"`
	GitRepoConfig  *RepoConfig `json:"gitRepoConfig,omitempty"`
	OverrideValues []*KVPair   `json:"overrideValues,omitempty"`
	ValuesYAML     string      `json:"valuesYAML,omitempty"`
}

func (args *RendersetCreateArgs) toOverrideValueString() string {
	if len(args.OverrideValues) == 0 {
		return ""
	}
	kvPairSlice := make([]string, 0, len(args.OverrideValues))
	for _, pair := range args.OverrideValues {
		kvPairSlice = append(kvPairSlice, fmt.Sprintf("%s=%s", pair.Key, pair.Value))
	}
	return strings.Join(kvPairSlice, ",")
}

func (args *RendersetCreateArgs) toGitRepoConfig() *template.GitRepoConfig {
	if args.GitRepoConfig == nil {
		return nil
	}
	return &template.GitRepoConfig{
		CodehostID:  args.GitRepoConfig.CodehostID,
		Owner:       args.GitRepoConfig.Owner,
		Repo:        args.GitRepoConfig.Repo,
		Branch:      args.GitRepoConfig.Branch,
		ValuesPaths: args.GitRepoConfig.ValuesPaths,
	}
}

func (args *RendersetCreateArgs) fromRenderChart(chart *template.RenderChart) *RendersetCreateArgs {
	args.ServiceName = chart.ServiceName
	args.YamlSource = chart.YamlSource
	if args.YamlSource == setting.ValuesYamlSourceFreeEdit {
		args.ValuesYAML = chart.ValuesYaml
	}
	if chart.GitRepoConfig != nil {
		args.GitRepoConfig = &RepoConfig{
			CodehostID:  chart.GitRepoConfig.CodehostID,
			Owner:       chart.GitRepoConfig.Owner,
			Repo:        chart.GitRepoConfig.Repo,
			Branch:      chart.GitRepoConfig.Branch,
			ValuesPaths: chart.GitRepoConfig.ValuesPaths,
		}
	}
	if chart.OverrideValues != "" {
		kvPairs := strings.Split(chart.OverrideValues, ",")
		for _, kv := range kvPairs {
			kvSlice := strings.Split(kv, "=")
			if len(kvSlice) == 2 {
				args.OverrideValues = append(args.OverrideValues, &KVPair{
					Key:   kvSlice[0],
					Value: kvSlice[1],
				})
			}
		}
	}
	return args
}

func ListChartRenders(productName, envName, serviceName string, log *zap.SugaredLogger) ([]*RendersetCreateArgs, error) {

	renderSetName := commonservice.GetProductEnvNamespace(envName, productName)

	opt := &commonrepo.RenderSetFindOption{
		Name: renderSetName,
	}
	rendersetObj, err := commonrepo.NewRenderSetColl().Find(opt)
	if err != nil || rendersetObj == nil {
		return nil, err
	}

	ret := make([]*RendersetCreateArgs, 0)

	for _, singleChart := range rendersetObj.ChartInfos {
		if serviceName == "" {
			ret = append(ret, new(RendersetCreateArgs).fromRenderChart(singleChart))
		} else {
			if singleChart.ServiceName == serviceName {
				ret = append(ret, new(RendersetCreateArgs).fromRenderChart(singleChart))
				break
			}
		}
	}
	return ret, nil
}

func generateValuesYaml(service *commonmodels.Service, args *RendersetCreateArgs, log *zap.SugaredLogger) (string, error) {
	if args.YamlSource == setting.ValuesYamlSourceFreeEdit {
		return args.ValuesYAML, nil
	} else if args.YamlSource == setting.ValuesYamlSourceDefault {
		return service.HelmChart.ValuesYaml, nil
	} else if args.YamlSource == setting.ValuesYamlSourceGitRepo {
		if args.GitRepoConfig == nil {
			return "", nil
		}
		// TODO need optimize, use parallel execution
		var allValues []byte
		for _, filePath := range args.GitRepoConfig.ValuesPaths {
			fileContent, err := fsservice.DownloadFileFromSource(
				&fsservice.DownloadFromSourceArgs{CodehostID: args.GitRepoConfig.CodehostID, Owner: args.GitRepoConfig.Owner, Repo: args.GitRepoConfig.Repo, Path: filePath, Branch: args.GitRepoConfig.Branch})
			if err != nil {
				return "", errors.Errorf("fail to download file from git, path: %s, repo: %v", filePath, *args.GitRepoConfig)
			}
			allValues, err = yaml2.Merge([][]byte{allValues, fileContent})
			if err != nil {
				return "", errors.Errorf("fail to merge file, path: %s, repo: %v", filePath, *args.GitRepoConfig)
			}
		}

		return string(allValues), nil
	}
	return "", nil
}

func CreateOrUpdateChartValues(productName, envName string, args *RendersetCreateArgs, userName, requestID string, log *zap.SugaredLogger) error {

	serviceName := args.ServiceName

	serviceOpt := &commonrepo.ServiceFindOption{
		ProductName: productName,
		ServiceName: serviceName,
		Revision:    0,
	}
	serviceObj, err := commonrepo.NewServiceColl().Find(serviceOpt)
	if err != nil {
		return e.ErrCreateRenderSet.AddDesc(err.Error())
	}
	if serviceObj == nil {
		return e.ErrCreateRenderSet.AddDesc("service not found")
	}
	if serviceObj.Type != setting.HelmDeployType {
		return e.ErrCreateRenderSet.AddDesc("invalid service type")
	}
	if serviceObj.HelmChart == nil {
		return e.ErrCreateRenderSet.AddDesc("missing helm chart info")
	}

	yamlContent, err := generateValuesYaml(serviceObj, args, log)
	if err != nil {
		return e.ErrCreateRenderSet.AddDesc(err.Error())
	}

	if yamlContent == "" {
		return e.ErrCreateRenderSet.AddDesc("empty yaml content")
	}
	// validate yaml content
	tMap := map[string]interface{}{}
	if err = yaml.Unmarshal([]byte(yamlContent), &tMap); err != nil {
		if err != nil {
			return e.ErrCreateRenderSet.AddDesc("yaml content illegal")
		}
	}

	renderSetName := commonservice.GetProductEnvNamespace(envName, productName)

	opt := &commonrepo.RenderSetFindOption{Name: renderSetName}
	curRenderset, err := commonrepo.NewRenderSetColl().Find(opt)
	if err != nil && err != mongo.ErrNoDocuments {
		return e.ErrCreateRenderSet.AddDesc(err.Error())
	}

	if curRenderset == nil {
		curRenderset = &commonmodels.RenderSet{
			Name:        renderSetName,
			EnvName:     envName,
			ProductTmpl: productName,
			UpdateBy:    userName,
			IsDefault:   false,
		}
	}

	//update or insert service values.yaml
	var targetChartInfo *template.RenderChart
	for _, singleChart := range curRenderset.ChartInfos {
		if singleChart.ServiceName != serviceName {
			continue
		}
		singleChart.ValuesYaml = yamlContent
		singleChart.YamlSource = args.YamlSource
		singleChart.OverrideValues = args.toOverrideValueString()
		if args.YamlSource == setting.ValuesYamlSourceGitRepo {
			singleChart.GitRepoConfig = args.toGitRepoConfig()
		} else {
			singleChart.GitRepoConfig = nil
		}
		targetChartInfo = singleChart
		break
	}
	if targetChartInfo == nil {
		targetChartInfo = &template.RenderChart{
			ServiceName:    serviceName,
			ChartVersion:   serviceObj.HelmChart.Version,
			ValuesYaml:     yamlContent,
			YamlSource:     args.YamlSource,
			OverrideValues: args.toOverrideValueString(),
		}
		if args.YamlSource == setting.ValuesYamlSourceGitRepo {
			targetChartInfo.GitRepoConfig = args.toGitRepoConfig()
		} else {
			targetChartInfo.GitRepoConfig = nil
		}
		curRenderset.ChartInfos = append(curRenderset.ChartInfos, targetChartInfo)
	}

	//create new renderset with increased revision
	err = commonservice.CreateHelmRenderSet(
		curRenderset,
		log,
	)

	if err != nil {
		return e.ErrCreateRenderSet.AddDesc(err.Error())
	}
	return err
}

func ensureHelmRenderSetArgs(args *commonmodels.RenderSet) error {
	if args == nil {
		return errors.New("nil RenderSet")
	}

	if len(args.Name) == 0 {
		return errors.New("empty render set name")
	}
	// 设置新的版本号
	rev, err := commonrepo.NewCounterColl().GetNextSeq("renderset:" + args.Name)
	if err != nil {
		return fmt.Errorf("get next render set revision error: %v", err)
	}

	args.Revision = rev
	return nil
}
