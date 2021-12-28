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

package workflow

import (
	"fmt"
	"sort"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/koderover/zadig/pkg/microservice/aslan/config"
	commonmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models/task"
	taskmodels "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/models/task"
	commonrepo "github.com/koderover/zadig/pkg/microservice/aslan/core/common/repository/mongodb"
	commonservice "github.com/koderover/zadig/pkg/microservice/aslan/core/common/service"
	"github.com/koderover/zadig/pkg/microservice/aslan/core/common/service/s3"
	e "github.com/koderover/zadig/pkg/tool/errors"
	"github.com/koderover/zadig/pkg/util"
)

// get global config payload
func CreateArtifactPackageTask(args *commonmodels.ArtifactPackageTaskArgs, taskCreator string, log *zap.SugaredLogger) (*task.Task, error) {

	configPayload := commonservice.GetConfigPayload(0)
	repos, err := commonrepo.NewRegistryNamespaceColl().FindAll(&commonrepo.FindRegOps{})

	if err != nil {
		log.Errorf("CreateArtifactPackageTask query registries failed, err: %s", err)
		return nil, fmt.Errorf("failed to query registries")
	}

	registriesInvolved := sets.NewString()
	registriesInvolved.Insert(args.SourceRegistries...)
	registriesInvolved.Insert(args.TargetRegistries...)

	configPayload.RepoConfigs = make(map[string]*commonmodels.RegistryNamespace)
	for _, repo := range repos {
		if !registriesInvolved.Has(repo.ID.Hex()) {
			continue
		}
		// if the registry is SWR, we need to modify ak/sk according to the rule
		if repo.RegProvider == config.SWRProvider {
			ak := fmt.Sprintf("%s@%s", repo.Region, repo.AccessKey)
			sk := util.ComputeHmacSha256(repo.AccessKey, repo.SecretKey)
			repo.AccessKey = ak
			repo.SecretKey = sk
		}
		configPayload.RepoConfigs[repo.ID.Hex()] = repo
	}

	defaultS3, err := s3.FindDefaultS3()
	if err != nil {
		err = e.ErrFindDefaultS3Storage.AddDesc("default storage is required by distribute task")
		return nil, err
	}

	defaultURL, err := defaultS3.GetEncryptedURL()
	if err != nil {
		err = e.ErrS3Storage.AddErr(err)
		return nil, err
	}

	task := &task.Task{
		Type:                    config.ArtifactType,
		ProductName:             args.ProjectName,
		Status:                  config.StatusCreated,
		ArtifactPackageTaskArgs: args,
		TaskCreator:             taskCreator,
		ConfigPayload:           configPayload,
		StorageURI:              defaultURL,
	}
	endpoint := fmt.Sprintf("%s-%s:9000", config.Namespace(), ClusterStorageEP)
	task.StorageEndpoint = endpoint

	subTasks, err := (&taskmodels.ArtifactPackage{
		TaskType:         config.TaskArtifactPackage,
		Enabled:          true,
		TaskStatus:       "",
		Timeout:          0,
		StartTime:        0,
		EndTime:          0,
		LogFile:          "",
		Images:           args.Images,
		SourceRegistries: args.SourceRegistries,
		TargetRegistries: args.TargetRegistries,
	}).ToSubTask()

	if err != nil {
		return nil, err
	}

	if err := ensurePipelineTask(task, "", log); err != nil {
		log.Errorf("CreateServiceTask ensurePipelineTask err : %v", err)
		return nil, err
	}

	stages := make([]*commonmodels.Stage, 0)
	AddSubtaskToStage(&stages, subTasks, args.EnvName)
	sort.Sort(ByStageKind(stages))
	task.Stages = stages
	task.SubTasks = []map[string]interface{}{}
	if len(task.Stages) == 0 {
		return nil, e.ErrCreateTask.AddDesc(e.PipelineSubTaskNotFoundErrMsg)
	}

	return task, nil
}
